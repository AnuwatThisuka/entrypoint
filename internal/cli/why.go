package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/search"
	"github.com/spf13/cobra"
)

func newWhy() *cobra.Command {
	return &cobra.Command{
		Use:   "why <keywords...>",
		Short: "Search past goals, decisions, and ticket titles across all branches",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}

			// All packets in the repo — notes are branch-independent, so this
			// spans every branch without walking each one.
			byID, err := packet.ReadAll(cwd)
			if err != nil {
				return err
			}
			packets := make([]*packet.Packet, 0, len(byID))
			for _, p := range byID {
				packets = append(packets, p)
			}

			query := strings.Join(args, " ")
			hits := search.Packets(query, packets)
			if len(hits) == 0 {
				fmt.Printf("No packets match %q.\n", query)
				return nil
			}

			for _, h := range hits {
				p := h.Packet
				ticket := ""
				if p.Ticket != nil {
					t := ""
					if p.Ticket.Title != "" {
						t = " — " + p.Ticket.Title
					}
					ticket = fmt.Sprintf("  [%s%s]", p.Ticket.ID, t)
				}
				fmt.Printf("%s (v%d, %s)%s\n", p.ID, p.Version, p.Branch, ticket)
				fmt.Printf("  Goal: %s\n", p.Goal)
				for _, d := range h.MatchedDecisions {
					fmt.Printf("  Decision: %s\n", d)
				}
			}
			return nil
		},
	}
}
