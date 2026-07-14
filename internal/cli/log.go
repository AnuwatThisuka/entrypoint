package cli

import (
	"fmt"
	"os"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/spf13/cobra"
)

func newLog() *cobra.Command {
	return &cobra.Command{
		Use:   "log",
		Short: "List all entrypoint packets on the current branch, newest first",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}
			if !git.HasCommits(cwd) {
				fmt.Println("No entrypoint packets on this branch.")
				return nil
			}

			packets, err := packet.ListBranch(cwd)
			if err != nil {
				return err
			}
			if len(packets) == 0 {
				fmt.Println("No entrypoint packets on this branch.")
				return nil
			}

			for _, bp := range packets {
				p := bp.Packet
				ticket := ""
				if p.Ticket != nil {
					ticket = " [" + p.Ticket.ID + "]"
				}
				fmt.Printf("v%d  %s  %s  %s%s  %s\n",
					p.Version, p.ID, short(bp.CommitSha), p.CreatedAt, ticket, p.Goal)
			}
			return nil
		},
	}
}
