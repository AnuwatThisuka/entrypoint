// Package summarize is auto-capture (Phase 4): ask the agent that just
// finished a session to summarize it as packet fields. The session is
// resumed in headless mode for a strict-JSON reply — the transcript itself
// is never read or stored.
package summarize

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// Keep auto-captured packets small.
const (
	maxListItems  = 8
	maxItemLength = 300
)

// Summary is the agent's structured session recap.
type Summary struct {
	Goal          string
	Done          []string
	InProgress    string
	Next          []string
	Decisions     []string
	OpenQuestions []string
}

// Prompt is the strict-JSON request sent to the resumed agent. Its exact
// shape is an open decision — it affects packet quality more than any code
// choice here.
const Prompt = `The coding session is ending. Summarize it as an Entrypoint packet so the next session can resume cheaply.

Reply with ONLY a JSON object — no markdown fences, no prose before or after:
{"goal": "...", "done": ["..."], "inProgress": "...", "next": ["..."], "decisions": ["..."], "openQuestions": ["..."]}

Field meanings:
- goal: one line — why this work is happening
- done: items completed this session
- inProgress: what was mid-flight when the session ended (omit if nothing)
- next: concrete next steps
- decisions: key choices made, each with a short reason
- openQuestions: unresolved questions (omit if none)

Keep it short: at most 8 items per list, one line each. No code snippets, no file contents.`

// RequestAgent resumes the session headless and asks for a summary.
// Returns nil when the claude CLI is unavailable, the session can't be
// resumed, or the reply isn't usable — callers decide if that's fatal.
func RequestAgent(cwd, sessionID string) *Summary {
	cmd := exec.Command("claude", "--resume", sessionID, "-p", Prompt)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return ParseSummary(string(out))
}

// ParseSummary parses the agent's reply into a Summary. It tolerates stray
// text or code fences around the JSON object but requires a non-empty goal.
func ParseSummary(text string) *Summary {
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end <= start {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return nil
	}
	goal, ok := raw["goal"].(string)
	if !ok || strings.TrimSpace(goal) == "" {
		return nil
	}

	s := &Summary{
		Goal:      clampItem(goal),
		Done:      clampList(raw["done"]),
		Next:      clampList(raw["next"]),
		Decisions: clampList(raw["decisions"]),
	}
	if ip, ok := raw["inProgress"].(string); ok && strings.TrimSpace(ip) != "" {
		s.InProgress = clampItem(ip)
	}
	if q := clampList(raw["openQuestions"]); len(q) > 0 {
		s.OpenQuestions = q
	}
	return s
}

func clampItem(value string) string {
	trimmed := strings.TrimSpace(value)
	r := []rune(trimmed)
	if len(r) > maxItemLength {
		return string(r[:maxItemLength-1]) + "…"
	}
	return trimmed
}

func clampList(value any) []string {
	arr, ok := value.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		c := clampItem(s)
		if c == "" {
			continue
		}
		out = append(out, c)
		if len(out) == maxListItems {
			break
		}
	}
	return out
}
