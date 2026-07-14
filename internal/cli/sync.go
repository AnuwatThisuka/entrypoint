package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/spf13/cobra"
)

var (
	reNonFastForward = regexp.MustCompile(`(?i)non-fast-forward|rejected`)
	reNoRefspec      = regexp.MustCompile(`(?i)src refspec .* does not match`)
	reNoRemoteRef    = regexp.MustCompile(`(?i)couldn't find remote ref`)
)

func newSync() *cobra.Command {
	var (
		remote string
		push   bool
	)
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch teammates' entrypoint packets — only the notes ref, no code pull",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}

			out, _ := git.Run(cwd, "remote")
			var remotes []string
			for _, l := range strings.Split(out, "\n") {
				if l != "" {
					remotes = append(remotes, l)
				}
			}
			if !contains(remotes, remote) {
				have := ""
				if len(remotes) > 0 {
					have = fmt.Sprintf(" (configured: %s)", strings.Join(remotes, ", "))
				}
				return fmt.Errorf("No remote named %q%s.", remote, have)
			}

			if push {
				if _, err := git.Run(cwd, "push", remote, git.NotesRef); err != nil {
					if ge, ok := git.AsError(err); ok {
						if reNonFastForward.MatchString(ge.Stderr) {
							return fmt.Errorf(
								"%q has packets you don't have locally. "+
									"Run `entrypoint sync` first, then push again.", remote)
						}
						if reNoRefspec.MatchString(ge.Stderr) {
							return fmt.Errorf("No local packets to push — run `entrypoint capture` first.")
						}
					}
					return err
				}
				count := packetCount(cwd)
				fmt.Printf("Pushed entrypoint packets to %s — %d packet%s shared.\n",
					remote, count, plural(count))
				return nil
			}

			if _, err := git.Run(cwd, "fetch", remote, git.NotesRef+":"+git.NotesRef); err != nil {
				if ge, ok := git.AsError(err); ok {
					if reNoRemoteRef.MatchString(ge.Stderr) {
						return fmt.Errorf(
							"%q has no entrypoint packets yet — nothing to sync. "+
								"A teammate needs to push them first: git push %s %s",
							remote, remote, git.NotesRef)
					}
					if reNonFastForward.MatchString(ge.Stderr) {
						return fmt.Errorf(
							"Local and remote entrypoint notes have diverged. Push your "+
								"packets first (git push %s %s), then sync again.", remote, git.NotesRef)
					}
				}
				return err
			}

			count := packetCount(cwd)
			fmt.Printf("Synced entrypoint packets from %s — %d packet%s available.\n",
				remote, count, plural(count))
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "origin", "remote to fetch from")
	cmd.Flags().BoolVar(&push, "push", false, "push your packets to the remote instead of fetching")
	return cmd
}

func packetCount(cwd string) int {
	byID, _ := packet.ReadAll(cwd)
	return len(byID)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
