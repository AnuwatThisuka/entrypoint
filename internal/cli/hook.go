package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/AnuwatThisuka/entrypoint/internal/hooklog"
	"github.com/spf13/cobra"
)

// The hook subcommand is the Claude Code integration (was the separate
// waypoint-claude-hook binary). A hook must never break the session: every
// failure path logs a one-line notice and exits 0.

type sessionEndPayload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Reason    string `json:"reason"`
}

type postToolUsePayload struct {
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

func newHook() *cobra.Command {
	hook := &cobra.Command{
		Use:    "hook",
		Short:  "Claude Code SessionEnd hook — auto-capture a packet",
		Hidden: true,
		Run: func(c *cobra.Command, args []string) {
			runSafe("session-end", sessionEnd)
		},
	}
	hook.AddCommand(&cobra.Command{
		Use:    "track",
		Short:  "Claude Code PostToolUse hook — record an agent file-write",
		Hidden: true,
		Run: func(c *cobra.Command, args []string) {
			runSafe("track", trackFileWrite)
		},
	})
	return hook
}

// runSafe reads stdin, runs fn, and always exits 0 — a hook must not break
// the session.
func runSafe(mode string, fn func(input string) error) {
	input, _ := io.ReadAll(os.Stdin)
	if err := fn(string(input)); err != nil {
		fmt.Fprintf(os.Stderr, "entrypoint hook (%s) skipped: %s\n", mode, err.Error())
	}
	os.Exit(0)
}

func selfExe() string {
	exe, err := os.Executable()
	if err != nil {
		return "entrypoint"
	}
	return exe
}

func sessionEnd(input string) error {
	var payload sessionEndPayload
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return err
	}
	if payload.SessionID == "" {
		return fmt.Errorf("hook payload has no session_id")
	}
	cwd := payload.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	cmd := exec.Command(selfExe(), "capture", "--auto", "--session-id", payload.SessionID)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		out := stdout.String()
		fmt.Print(out)
		_ = hooklog.Record(cwd, hooklog.Run{
			At: now(), Outcome: hooklog.Captured, Reason: "captured",
			Message: trimSpace(out),
		})
		return nil
	}

	// Name the specific reason instead of a generic "Hook cancelled", and
	// leave the same line behind for `entrypoint status` to surface later.
	cls := hooklog.ClassifyCaptureFailure(stderr.String())
	fmt.Fprintf(os.Stderr, "entrypoint auto-capture %s: %s\n", cls.Outcome, cls.Message)
	_ = hooklog.Record(cwd, hooklog.Run{
		At: now(), Outcome: cls.Outcome, Reason: cls.Reason, Message: cls.Message,
	})
	return nil
}

func trackFileWrite(input string) error {
	var payload postToolUsePayload
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return err
	}
	file := payload.ToolInput.FilePath
	if file == "" {
		file = payload.ToolInput.NotebookPath
	}
	if file == "" {
		return nil // tool didn't write a file — nothing to record
	}
	cwd := payload.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	cmd := exec.Command(selfExe(), "track", file)
	cmd.Dir = cwd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := trimSpace(stderr.String())
		if msg == "" {
			msg = "track failed"
		}
		fmt.Fprintf(os.Stderr, "entrypoint track skipped: %s\n", msg)
	}
	return nil
}

func now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func trimSpace(s string) string {
	return string(bytes.TrimSpace([]byte(s)))
}
