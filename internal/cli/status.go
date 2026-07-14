package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/hooklog"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/trailer"
	"github.com/spf13/cobra"
)

func newStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether HEAD has a packet, uncommitted changes, push state, and the last hook outcome",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}

			branch, _ := git.CurrentBranch(cwd)
			fmt.Printf("Branch: %s\n", branch)

			if !git.HasCommits(cwd) {
				fmt.Println("HEAD: no commits yet — commit before capturing a packet.")
				printLastHook(cwd)
				return nil
			}

			// 1. Does HEAD already carry a packet?
			message, _ := git.HeadMessage(cwd)
			ids := trailer.ParsePacketIDs(message)
			if len(ids) == 0 {
				fmt.Println("Packet: HEAD has no packet — run `entrypoint capture`.")
			} else {
				byID, _ := packet.ReadAll(cwd)
				parts := make([]string, 0, len(ids))
				for _, id := range ids {
					if p, ok := byID[id]; ok {
						parts = append(parts, fmt.Sprintf("%s (v%d)", id, p.Version))
					} else {
						parts = append(parts, fmt.Sprintf("%s (note missing)", id))
					}
				}
				fmt.Printf("Packet: HEAD has %s.\n", strings.Join(parts, ", "))
			}

			// 2. Uncommitted changes make the next capture operate on a stale HEAD.
			if dirty, _ := git.WorkingTreeDirty(cwd); dirty {
				fmt.Println("Changes: uncommitted changes present — commit them first so the " +
					"next capture lands on the right commit.")
			} else {
				fmt.Println("Changes: working tree clean.")
			}

			// 3. Is HEAD already on the tracked remote?
			upstream, _ := git.UpstreamRef(cwd)
			if upstream == nil {
				fmt.Println("Remote: no upstream tracked for this branch.")
			} else {
				head, _ := git.HeadSha(cwd)
				if anc, _ := git.IsAncestor(cwd, head, upstream.Ref); anc {
					fmt.Printf("Remote: HEAD is already on %s — the next capture will "+
						"need `git push --force-with-lease %s %s`.\n", upstream.Ref, upstream.Remote, branch)
				} else {
					fmt.Printf("Remote: HEAD is not yet on %s.\n", upstream.Ref)
				}
			}

			printLastHook(cwd)
			return nil
		},
	}
}

func printLastHook(cwd string) {
	last := hooklog.ReadLast(cwd)
	if last == nil {
		fmt.Println("Last hook: none recorded.")
		return
	}
	fmt.Printf("Last hook: %s — %s (%s).\n", last.Outcome, last.Message, last.At)
}
