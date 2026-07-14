package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/spf13/cobra"
)

// formatPacket renders a packet for pasting into an agent's context window.
func formatPacket(p *packet.Packet, commitSha string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Entrypoint %s (v%d) — branch %s\n", p.ID, p.Version, p.Branch)
	fmt.Fprintf(&b, "Captured: %s  Commit: %s\n", p.CreatedAt, short(commitSha))
	if p.Visibility == "redacted" {
		b.WriteString("(redacted: state detail was stripped at capture time — public repo)\n")
	}
	if p.Ticket != nil {
		title := ""
		if p.Ticket.Title != "" {
			title = " — " + p.Ticket.Title
		}
		fmt.Fprintf(&b, "Ticket: %s%s (%s)\n", p.Ticket.ID, title, p.Ticket.Source)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Goal: %s\n", p.Goal)

	if len(p.State.Done) > 0 {
		b.WriteString("\nDone:\n")
		for _, item := range p.State.Done {
			fmt.Fprintf(&b, "  - %s\n", item)
		}
	}
	if p.State.InProgress != "" {
		fmt.Fprintf(&b, "\nIn progress: %s\n", p.State.InProgress)
	}
	if len(p.State.Next) > 0 {
		b.WriteString("\nNext:\n")
		for _, item := range p.State.Next {
			fmt.Fprintf(&b, "  - %s\n", item)
		}
	}
	if len(p.Decisions) > 0 {
		b.WriteString("\nDecisions:\n")
		for _, item := range p.Decisions {
			fmt.Fprintf(&b, "  - %s\n", item)
		}
	}
	if len(p.OpenQuestions) > 0 {
		b.WriteString("\nOpen questions:\n")
		for _, item := range p.OpenQuestions {
			fmt.Fprintf(&b, "  - %s\n", item)
		}
	}
	if len(p.FilesTouched) > 0 {
		fmt.Fprintf(&b, "\nFiles touched: %s\n", strings.Join(p.FilesTouched, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

func newResume() *cobra.Command {
	var at string
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Print the latest entrypoint packet for the current branch",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}
			if !git.HasCommits(cwd) {
				return fmt.Errorf("No commits on this branch yet — nothing to resume.")
			}

			packets, err := packet.ListBranch(cwd)
			if err != nil {
				return err
			}
			if len(packets) == 0 {
				return fmt.Errorf("No entrypoint packets on this branch. Run `entrypoint capture` first.")
			}

			selected := &packets[0]
			if at != "" {
				v, err := strconv.Atoi(at)
				if err != nil {
					return fmt.Errorf("--at expects a version number, got %q.", at)
				}
				selected = nil
				for i := range packets {
					if packets[i].Packet.Version == v {
						selected = &packets[i]
						break
					}
				}
				if selected == nil {
					return fmt.Errorf("No packet with version %d on this branch.", v)
				}
			}

			fmt.Println(formatPacket(selected.Packet, selected.CommitSha))
			return nil
		},
	}
	cmd.Flags().StringVar(&at, "at", "", "print a specific packet version instead of the latest")
	return cmd
}
