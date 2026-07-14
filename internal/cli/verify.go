package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/signing"
	"github.com/spf13/cobra"
)

func newVerify() *cobra.Command {
	var branch string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check packet signatures on a branch — signed, unsigned, or tampered",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}
			if !git.HasCommits(cwd) {
				fmt.Println("No entrypoint packets on this branch.")
				return nil
			}

			rev := "HEAD"
			if branch != "" {
				rev = branch
			}
			branchLabel := branch
			if branchLabel == "" {
				branchLabel, _ = git.CurrentBranch(cwd)
			}

			packets, err := packet.ListBranch(cwd, rev)
			if err != nil {
				return fmt.Errorf("Unknown branch %q.", rev)
			}
			if len(packets) == 0 {
				fmt.Printf("No entrypoint packets on %s.\n", branchLabel)
				return nil
			}

			var signed, unsigned, unverifiable, tampered int
			for _, bp := range packets {
				result, err := signing.Verify(cwd, bp.Packet)
				if err != nil {
					return err
				}
				head := fmt.Sprintf("%s (v%d)  %s", bp.Packet.ID, bp.Packet.Version, short(bp.CommitSha))
				switch result.Status {
				case signing.Signed:
					signed++
					fmt.Printf("%s  signed\n", head)
				case signing.Unsigned:
					unsigned++
					fmt.Printf("%s  unsigned\n", head)
				case signing.Unverifiable:
					unverifiable++
					detail := result.Detail
					if detail == "" {
						detail = "signer's key unknown"
					}
					fmt.Printf("%s  signed, can't verify — %s\n", head, detail)
				default:
					tampered++
					detail := result.Detail
					if detail == "" {
						detail = "signature mismatch"
					}
					fmt.Printf("%s  TAMPERED — %s\n", head, detail)
				}
			}

			parts := []string{fmt.Sprintf("%d signed", signed), fmt.Sprintf("%d unsigned", unsigned)}
			if unverifiable > 0 {
				parts = append(parts, fmt.Sprintf("%d unverifiable", unverifiable))
			}
			parts = append(parts, fmt.Sprintf("%d tampered", tampered))
			fmt.Printf("\n%s: %s\n", branchLabel, strings.Join(parts, ", "))

			if unverifiable > 0 {
				fmt.Println("Unverifiable packets are signed by keys not in your keyring — " +
					"import the signer's public key to check them.")
			}

			// Only proven mismatches are an error; unverifiable is a warning.
			if tampered > 0 {
				return &exitError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "", "branch to walk (default: current)")
	return cmd
}
