// Package hooklog is the hook last-run log (Phase 9): one line per
// SessionEnd hook run, so `entrypoint status` can surface why the last
// auto-capture did or didn't write a packet. Lives under .git/.
package hooklog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
)

// Outcome of a hook run.
type Outcome string

const (
	Captured Outcome = "captured"
	Skipped  Outcome = "skipped"
	Failed   Outcome = "failed"
)

// Run is one recorded SessionEnd hook run.
type Run struct {
	At      string  `json:"at"` // ISO 8601
	Outcome Outcome `json:"outcome"`
	// Reason is a short machine tag, e.g. "no-new-commit".
	Reason string `json:"reason"`
	// Message is a one-line human-readable explanation.
	Message string `json:"message"`
}

func logPath(cwd string) (string, error) {
	gitDir, err := git.Run(cwd, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, "entrypoint", "last-run"), nil
}

// Record appends a hook run to the log (latest last).
func Record(cwd string, run Run) error {
	target, err := logPath(cwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(run)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(append(data, '\n'))
	return err
}

var (
	reAlreadyPacket = regexp.MustCompile(`(?i)already has an? entrypoint packet`)
	reNoSummary     = regexp.MustCompile(`(?i)could not get a session summary`)
	reNoCommits     = regexp.MustCompile(`(?i)no commits yet`)
)

// Classification names a specific skip reason from a failed capture.
type Classification struct {
	Outcome Outcome
	Reason  string
	Message string
}

// ClassifyCaptureFailure turns a failed auto-capture's stderr into a
// specific skip reason instead of a generic "Hook cancelled".
func ClassifyCaptureFailure(stderr string) Classification {
	switch {
	case reAlreadyPacket.MatchString(stderr):
		return Classification{Skipped, "no-new-commit", "no new commit since the last packet"}
	case reNoSummary.MatchString(stderr):
		return Classification{Skipped, "summary-unusable", "agent could not summarize the session"}
	case reNoCommits.MatchString(stderr):
		return Classification{Skipped, "no-commits", "no commits yet to attach a packet to"}
	}
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = "capture failed"
	}
	return Classification{Failed, "capture-failed", msg}
}

// ReadLast returns the most recent hook run, or nil when none exists.
func ReadLast(cwd string) *Run {
	target, err := logPath(cwd)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil
	}
	var last string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			last = line
		}
	}
	if last == "" {
		return nil
	}
	var run Run
	if err := json.Unmarshal([]byte(last), &run); err != nil {
		return nil
	}
	return &run
}
