package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/trailer"
	"github.com/spf13/cobra"
)

type checkResult struct {
	name    string
	status  string // "pass" | "fail" | "info"
	message string
	fix     string // exact command to run on failure
}

func newDoctor() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common entrypoint problems; each failing check prints the exact fix",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}

			results := []checkResult{
				checkHeadNotes(cwd),
				checkOrphanedNotes(cwd),
				checkGhAuth(cwd),
				checkSigningKey(cwd),
				checkNotesRefSync(cwd),
			}

			failures := 0
			for _, r := range results {
				tag := "FAIL"
				switch r.status {
				case "pass":
					tag = "PASS"
				case "info":
					tag = "INFO"
				}
				fmt.Printf("[%s] %s: %s\n", tag, r.name, r.message)
				if r.status == "fail" {
					failures++
					if r.fix != "" {
						fmt.Printf("       fix: %s\n", r.fix)
					}
				}
			}

			suffix := ""
			if failures == 0 {
				suffix = " All good."
			}
			fmt.Printf("\n%d checks, %d failing.%s\n", len(results), failures, suffix)
			if failures > 0 {
				return &exitError{code: 1}
			}
			return nil
		},
	}
}

func checkHeadNotes(cwd string) checkResult {
	name := "HEAD packet trailer"
	if !git.HasCommits(cwd) {
		return checkResult{name, "info", "no commits yet.", ""}
	}
	msg, _ := git.HeadMessage(cwd)
	ids := trailer.ParsePacketIDs(msg)
	if len(ids) <= 1 {
		m := "no packet on HEAD."
		if len(ids) == 1 {
			m = fmt.Sprintf("one packet (%s).", ids[0])
		}
		return checkResult{name, "pass", m, ""}
	}
	return checkResult{
		name, "fail",
		fmt.Sprintf("HEAD carries %d packet trailers (%s) — only one is expected.",
			len(ids), strings.Join(ids, ", ")),
		"git commit --amend  # remove the duplicate Entrypoint-Packet line, then re-run `entrypoint capture --force`",
	}
}

func checkOrphanedNotes(cwd string) checkResult {
	name := "orphaned notes"
	notes := git.NotesList(cwd)
	if len(notes) == 0 {
		return checkResult{name, "pass", "none — no packets stored yet.", ""}
	}

	reachablePackets, _ := packet.ListReachable(cwd)
	reachable := map[string]bool{}
	for _, bp := range reachablePackets {
		reachable[bp.Packet.ID] = true
	}

	var orphans []string
	for _, note := range notes {
		raw, ok := git.NotesShow(cwd, note.CommitSha)
		if !ok {
			continue
		}
		var parsed struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			continue
		}
		if parsed.ID != "" && !reachable[parsed.ID] {
			orphans = append(orphans, fmt.Sprintf("%s on %s", parsed.ID, short(note.CommitSha)))
		}
	}

	if len(orphans) == 0 {
		return checkResult{name, "pass", "none — every note is reachable.", ""}
	}
	return checkResult{
		name, "fail",
		fmt.Sprintf("%d note(s) reference no reachable commit: %s. "+
			"These are usually left behind when a packet was replaced or the commit was rebased.",
			len(orphans), strings.Join(orphans, "; ")),
		fmt.Sprintf("git notes --ref=%s remove <commit>  # for each orphaned commit above", git.NotesRef),
	}
}

func checkGhAuth(cwd string) checkResult {
	name := "gh ticket linking"
	cmd := exec.Command("gh", "auth", "status")
	cmd.Dir = cwd
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			// gh not installed / not on PATH
			return checkResult{name, "info",
				"gh not installed — ticket linking falls back to manual entry.", ""}
		}
		return checkResult{name, "fail",
			"gh is installed but not authenticated — ticket titles won't auto-fetch.",
			"gh auth login"}
	}
	return checkResult{name, "pass", "gh is authenticated.", ""}
}

func checkSigningKey(cwd string) checkResult {
	name := "signing key"
	if !git.HasCommits(cwd) {
		return checkResult{name, "info", "no commits yet.", ""}
	}
	packets, err := packet.ListBranch(cwd)
	if err != nil {
		return checkResult{name, "info", "could not read branch packets.", ""}
	}
	signedCount := 0
	for _, bp := range packets {
		if bp.Packet.Signature != "" {
			signedCount++
		}
	}
	if signedCount == 0 {
		return checkResult{name, "info", "no signed packets on this branch.", ""}
	}
	key, _ := git.Run(cwd, "config", "user.signingkey")
	if strings.TrimSpace(key) != "" {
		return checkResult{name, "pass",
			fmt.Sprintf("%d signed packet(s); user.signingkey is set.", signedCount), ""}
	}
	return checkResult{
		name, "fail",
		fmt.Sprintf("%d signed packet(s) on this branch but user.signingkey is unset — you can't sign or re-verify.", signedCount),
		"git config user.signingkey <your-key-id>",
	}
}

func checkNotesRefSync(cwd string) checkResult {
	name := "notes ref sync"
	local, _ := git.LocalNotesRefSha(cwd)

	upstream, _ := git.UpstreamRef(cwd)
	out, _ := git.Run(cwd, "remote")
	var remotes []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			remotes = append(remotes, l)
		}
	}
	remote := ""
	if upstream != nil {
		remote = upstream.Remote
	} else if contains(remotes, "origin") {
		remote = "origin"
	} else if len(remotes) > 0 {
		remote = remotes[0]
	}

	if remote == "" {
		if local != "" {
			return checkResult{name, "info",
				"packets exist locally but no remote is configured to sync them to.", ""}
		}
		return checkResult{name, "pass", "no remote and no local packets.", ""}
	}

	remoteSha, reachable := git.RemoteNotesRefSha(cwd, remote)
	if !reachable {
		return checkResult{name, "info", fmt.Sprintf("could not reach remote %q.", remote), ""}
	}

	switch {
	case local == "" && remoteSha == "":
		return checkResult{name, "pass", "no packets stored anywhere yet.", ""}
	case local != "" && remoteSha == "":
		return checkResult{name, "fail",
			fmt.Sprintf("local packets are not on %q.", remote), "entrypoint sync --push"}
	case local == "" && remoteSha != "":
		return checkResult{name, "fail",
			fmt.Sprintf("%q has packets you don't have locally.", remote), "entrypoint sync"}
	case local == remoteSha:
		return checkResult{name, "pass", fmt.Sprintf("in sync with %q.", remote), ""}
	}
	return checkResult{
		name, "fail",
		fmt.Sprintf("local notes ref differs from %q.", remote),
		"entrypoint sync   # then, if you have local packets to share: entrypoint sync --push",
	}
}
