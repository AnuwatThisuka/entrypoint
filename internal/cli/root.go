// Package cli wires the entrypoint CLI. Command files here stay focused on
// argument parsing and output formatting; real logic lives in internal/*.
package cli

import (
	"fmt"
	"os"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/spf13/cobra"
)

// version is the CLI version, overridable at build time via -ldflags.
var version = "0.1.0"

// exitError carries an explicit process exit code for commands that report
// a non-zero status (verify tampered, doctor failures) after already
// printing their own output. Execute treats it as "exit, don't reprint".
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// newRoot builds the root command with all subcommands attached.
func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "entrypoint",
		Short:         "Lightweight, ticket-linked context checkpoints for AI coding agents",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(
		newInit(),
		newCapture(),
		newResume(),
		newLog(),
		newBlame(),
		newTrack(),
		newWhy(),
		newSync(),
		newVerify(),
		newReport(),
		newStatus(),
		newDoctor(),
		newHook(),
	)
	return root
}

// Execute runs the CLI and translates errors into plain-language output,
// mirroring the TS cli.ts catch block.
func Execute() {
	err := newRoot().Execute()
	if err == nil {
		return
	}
	var ee *exitError
	if ok := asExit(err, &ee); ok {
		os.Exit(ee.code)
	}
	if ge, ok := git.AsError(err); ok {
		fmt.Fprintf(os.Stderr, "entrypoint: git command failed — %s\n", ge.Stderr)
	} else {
		fmt.Fprintf(os.Stderr, "entrypoint: %s\n", err.Error())
	}
	os.Exit(1)
}

func asExit(err error, target **exitError) bool {
	if e, ok := err.(*exitError); ok {
		*target = e
		return true
	}
	return false
}
