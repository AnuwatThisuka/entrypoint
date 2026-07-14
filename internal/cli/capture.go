package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AnuwatThisuka/entrypoint/internal/blocks"
	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/privacy"
	"github.com/AnuwatThisuka/entrypoint/internal/sessionfiles"
	"github.com/AnuwatThisuka/entrypoint/internal/signing"
	"github.com/AnuwatThisuka/entrypoint/internal/summarize"
	"github.com/AnuwatThisuka/entrypoint/internal/ticket"
	"github.com/spf13/cobra"
)

func newCapture() *cobra.Command {
	var (
		ticketID      string
		goal          string
		done          []string
		next          []string
		decision      []string
		inProgress    string
		question      []string
		force         bool
		forceFull     bool
		forceRedacted bool
		auto          bool
		sessionID     string
		sign          bool
	)

	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Capture an entrypoint packet on the current HEAD commit",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()

			if forceFull && forceRedacted {
				return fmt.Errorf("--force-full and --force-redacted are mutually exclusive.")
			}
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}
			if !git.HasCommits(cwd) {
				return fmt.Errorf("No commits yet — commit your work before capturing a packet.")
			}

			if auto {
				sid := sessionID
				if sid == "" {
					sid = os.Getenv("CLAUDE_SESSION_ID")
				}
				if sid == "" {
					return fmt.Errorf("--auto needs a session: pass --session-id or set CLAUDE_SESSION_ID.")
				}
				summary := summarize.RequestAgent(cwd, sid)
				if summary == nil {
					return fmt.Errorf(
						"Could not get a session summary from the agent — packet not written. " +
							"Run `entrypoint capture` manually instead.")
				}
				if goal == "" {
					goal = summary.Goal
				}
				if len(done) == 0 {
					done = summary.Done
				}
				if len(next) == 0 {
					next = summary.Next
				}
				if len(decision) == 0 {
					decision = summary.Decisions
				}
				if inProgress == "" {
					inProgress = summary.InProgress
				}
				if len(question) == 0 && len(summary.OpenQuestions) > 0 {
					question = summary.OpenQuestions
				}
			}

			interactive := !auto && isTTY(os.Stdin) && isTTY(os.Stdout)
			if interactive && goal == "" {
				r := bufio.NewReader(os.Stdin)
				goal = ask(r, "Goal (why is this work happening?): ")
				if len(done) == 0 {
					done = askList(r, "Done")
				}
				if inProgress == "" {
					inProgress = ask(r, "In progress (empty to skip): ")
				}
				if len(next) == 0 {
					next = askList(r, "Next")
				}
				if len(decision) == 0 {
					decision = askList(r, "Key decisions")
				}
			}

			if goal == "" {
				return fmt.Errorf(`A goal is required — pass --goal "<text>" or run interactively.`)
			}

			branch, err := git.CurrentBranch(cwd)
			if err != nil {
				return err
			}

			var tk *packet.Ticket
			if ticketID != "" {
				resolved := ticket.Resolve(cwd, ticketID)
				tk = &resolved
				if tk.Title != "" {
					fmt.Printf("Linked ticket %s — %s\n", tk.ID, tk.Title)
				}
			}

			agentFiles, _ := sessionfiles.Read(cwd)
			blks, err := blocks.ComputeHead(cwd, agentFiles)
			if err != nil {
				return err
			}
			files, err := git.HeadChangedFiles(cwd)
			if err != nil {
				return err
			}
			version, err := packet.NextVersion(cwd)
			if err != nil {
				return err
			}

			p := &packet.Packet{
				ID:            packet.NewID(),
				Version:       version,
				CreatedAt:     time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
				Branch:        branch,
				Ticket:        tk,
				Goal:          goal,
				State:         packet.State{Done: nonNil(done), InProgress: inProgress, Next: nonNil(next)},
				Decisions:     nonNil(decision),
				OpenQuestions: emptyToNil(question),
				FilesTouched:  nonNil(files),
				Blocks:        blks,
				Visibility:    "full",
			}

			fc := privacy.ForceNone
			if forceFull {
				fc = privacy.ForceFull
			} else if forceRedacted {
				fc = privacy.ForceRedacted
			}
			res := privacy.Apply(cwd, p, fc)
			safe := res.Packet

			if safe.Visibility == "redacted" && !res.Forced {
				reason := "Repo visibility could not be determined (remote present)"
				if res.RepoVisibility == privacy.Public {
					reason = "Public repo detected"
				}
				fmt.Printf("%s — packet redacted: state detail and open questions "+
					"stripped. Use --force-full to override.\n", reason)
			}

			// Sign after privacy so the signature covers what is stored.
			if sign {
				sig, err := signing.Sign(cwd, safe)
				if err != nil {
					return err
				}
				safe.Signature = sig
			}

			// Capture amends HEAD (new sha). Warn if the pre-amend commit was
			// already on the tracked remote — the next push then needs --force.
			preAmendSha, err := git.HeadSha(cwd)
			if err != nil {
				return err
			}
			upstream, _ := git.UpstreamRef(cwd)
			wasPushed := false
			if upstream != nil {
				wasPushed, _ = git.IsAncestor(cwd, preAmendSha, upstream.Ref)
			}

			commitSha, err := packet.AttachToHead(cwd, safe, force)
			if err != nil {
				return err
			}
			// The capture closes out the session — start the next one clean.
			_ = sessionfiles.Clear(cwd)

			signedNote := ""
			if sign {
				signedNote = " (signed)"
			}
			fmt.Printf("Captured %s (v%d) on %s at %s%s\n",
				safe.ID, safe.Version, branch, short(commitSha), signedNote)

			if wasPushed {
				fmt.Printf("Warning: %s was already pushed to %s; capture rewrote it. "+
					"Update the remote with: git push --force-with-lease %s %s\n",
					short(preAmendSha), upstream.Ref, upstream.Remote, branch)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&ticketID, "ticket", "", "ticket id to link, e.g. 456 or JIRA-123")
	f.StringVar(&goal, "goal", "", "one-line reason this work is happening")
	f.StringArrayVar(&done, "done", nil, "completed item (repeatable)")
	f.StringArrayVar(&next, "next", nil, "next step (repeatable)")
	f.StringArrayVar(&decision, "decision", nil, "key decision and why (repeatable)")
	f.StringVar(&inProgress, "in-progress", "", "what is currently in progress")
	f.StringArrayVar(&question, "question", nil, "open question (repeatable)")
	f.BoolVar(&force, "force", false, "replace an existing packet on HEAD")
	f.BoolVar(&forceFull, "force-full", false, "write full detail even on a public repo")
	f.BoolVar(&forceRedacted, "force-redacted", false, "redact even on a private repo")
	f.BoolVar(&auto, "auto", false, "ask the agent that just finished to summarize the session (Claude Code hook)")
	f.StringVar(&sessionID, "session-id", "", "session to resume for --auto (defaults to $CLAUDE_SESSION_ID)")
	f.BoolVar(&sign, "sign", false, "sign the packet with your git-configured GPG key (user.signingkey)")
	return cmd
}

// ask prompts for a single trimmed line; empty answer means "skip".
func ask(r *bufio.Reader, label string) string {
	fmt.Print(label)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}

// askList prompts for a list, one item per line, empty line to finish.
func askList(r *bufio.Reader, label string) []string {
	fmt.Printf("%s (one per line, empty line to finish):\n", label)
	var items []string
	for {
		fmt.Print("  - ")
		line, _ := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return items
		}
		items = append(items, line)
	}
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// nonNil returns an empty (non-nil) slice for nil input, so JSON renders
// [] like the TS version rather than null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func emptyToNil(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
