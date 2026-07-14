package ticket

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// GitHub issue refs look like "456" or "#456". Anything else (JIRA-123,
// LIN-42, ...) is not ours — return nil so resolution falls through.
var issueID = regexp.MustCompile(`^#?(\d+)$`)

type githubAdapter struct{}

func (githubAdapter) Source() string { return "github" }

func (githubAdapter) Fetch(cwd, id string) *packet.Ticket {
	m := issueID.FindStringSubmatch(strings.TrimSpace(id))
	if m == nil {
		return nil
	}
	issueNumber := m[1]

	cmd := exec.Command("gh", "issue", "view", issueNumber, "--json", "title")
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.Output()
	if err != nil {
		// gh not installed, not authenticated, no GitHub remote, or issue
		// not found — Phase 2 requires falling back to manual entry.
		return nil
	}
	var parsed struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || parsed.Title == "" {
		return nil
	}
	return &packet.Ticket{ID: "#" + issueNumber, Title: parsed.Title, Source: "github"}
}
