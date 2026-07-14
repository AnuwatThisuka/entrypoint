package cli

import (
	"fmt"
	"os"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/sessionfiles"
	"github.com/spf13/cobra"
)

func newTrack() *cobra.Command {
	return &cobra.Command{
		Use:   "track <files...>",
		Short: "Record files as agent-written for this session (used by the Claude Code PostToolUse hook)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}
			return sessionfiles.Add(cwd, args)
		},
	}
}
