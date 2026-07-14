package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/testutil"
)

func TestHookTrackRecordsFile(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("x.go", "l1\n", "init")

	payload, _ := json.Marshal(map[string]any{
		"cwd":        r.Dir,
		"tool_input": map[string]string{"file_path": filepath.Join(r.Dir, "x.go")},
	})
	res := r.RunStdin(string(payload), "hook", "track")
	if res.ExitCode != 0 {
		t.Fatalf("hook track exit %d: %s", res.ExitCode, res.Stderr)
	}

	logFile := filepath.Join(r.Dir, ".git", "entrypoint", "agent-files")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("agent-files log not written: %v", err)
	}
	if string(data) != "x.go\n" {
		t.Fatalf("expected x.go recorded, got %q", data)
	}
}

// fakeClaude prints a fixed JSON summary regardless of args, standing in for
// the agent self-summarize the SessionEnd hook triggers.
const fakeClaude = `#!/bin/sh
echo '{"goal":"auto goal","done":["did work"],"next":["next step"],"decisions":["chose X"]}'
`

func TestHookAutoCapture(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("claude", fakeClaude)
	r.CommitFile("a.txt", "hi\n", "init")

	payload, _ := json.Marshal(map[string]any{
		"session_id": "sess-123",
		"cwd":        r.Dir,
		"reason":     "clear",
	})
	res := r.RunStdin(string(payload), "hook")
	if res.ExitCode != 0 {
		t.Fatalf("hook exit %d: %s", res.ExitCode, res.Stderr)
	}
	mustContain(t, res.Stdout, "Captured pk_")

	resume := r.Run("resume")
	mustContain(t, resume.Stdout, "Goal: auto goal")
	mustContain(t, resume.Stdout, "did work")

	st := r.Run("status")
	mustContain(t, st.Stdout, "Last hook: captured")
}

func TestHookAutoSkipsWhenNoNewCommit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("claude", fakeClaude)
	r.CommitFile("a.txt", "hi\n", "init")

	payload, _ := json.Marshal(map[string]any{"session_id": "s", "cwd": r.Dir})
	// First run captures.
	r.RunStdin(string(payload), "hook")
	// Second run: HEAD already has a packet, no new commit → skipped.
	res := r.RunStdin(string(payload), "hook")
	if res.ExitCode != 0 {
		t.Fatalf("hook must exit 0 even on skip, got %d", res.ExitCode)
	}
	mustContain(t, res.Stderr, "no new commit since the last packet")

	st := r.Run("status")
	mustContain(t, st.Stdout, "Last hook: skipped")
}
