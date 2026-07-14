// Package privacy is the write-time gate every packet must pass before
// hitting git notes. On a public (or undeterminable) repo, packet detail
// is stripped; on a private/no-remote repo it is kept in full.
package privacy

import (
	"encoding/json"
	"os/exec"
	"sync"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// Visibility of the repo a packet would be written to.
type Visibility string

const (
	Public   Visibility = "public"
	Private  Visibility = "private"
	NoRemote Visibility = "no-remote"
	Unknown  Visibility = "unknown"
)

// Force overrides the automatic decision (--force-full / --force-redacted).
type Force string

const (
	ForceNone     Force = ""
	ForceFull     Force = "full"
	ForceRedacted Force = "redacted"
)

// Cached per session so the gh call runs once per capture, not per file.
var (
	cacheMu sync.Mutex
	cache   = map[string]Visibility{}
)

// ResetCache clears the per-session visibility cache (test hook).
func ResetCache() {
	cacheMu.Lock()
	cache = map[string]Visibility{}
	cacheMu.Unlock()
}

// Detect reports whether packets written in cwd could reach a public repo.
func Detect(cwd string) Visibility {
	cacheMu.Lock()
	if v, ok := cache[cwd]; ok {
		cacheMu.Unlock()
		return v
	}
	cacheMu.Unlock()

	v := detectUncached(cwd)

	cacheMu.Lock()
	cache[cwd] = v
	cacheMu.Unlock()
	return v
}

func detectUncached(cwd string) Visibility {
	remotes, err := git.Run(cwd, "remote")
	if err != nil || remotes == "" {
		return NoRemote
	}

	cmd := exec.Command("gh", "repo", "view", "--json", "isPrivate")
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.Output()
	if err != nil {
		return Unknown
	}
	var parsed struct {
		IsPrivate *bool `json:"isPrivate"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || parsed.IsPrivate == nil {
		return Unknown
	}
	if *parsed.IsPrivate {
		return Private
	}
	return Public
}

// Redact keeps goal and decisions; strips state detail and openQuestions
// down to file-level only (filesTouched stays).
func Redact(p *packet.Packet) *packet.Packet {
	out := *p
	out.State = packet.State{Done: []string{}, Next: []string{}}
	out.OpenQuestions = nil
	out.Visibility = "redacted"
	return &out
}

// Result reports what Apply decided.
type Result struct {
	Packet         *packet.Packet
	RepoVisibility Visibility
	Forced         bool
}

// Apply decides and applies packet visibility before write. Fail-safe: a
// remote whose visibility is "unknown" is treated like a public repo —
// better to over-strip than to leak. "no-remote" is treated as private.
func Apply(cwd string, p *packet.Packet, force Force) Result {
	switch force {
	case ForceFull:
		full := *p
		full.Visibility = "full"
		return Result{Packet: &full, RepoVisibility: Detect(cwd), Forced: true}
	case ForceRedacted:
		return Result{Packet: Redact(p), RepoVisibility: Detect(cwd), Forced: true}
	}

	v := Detect(cwd)
	if v == Public || v == Unknown {
		return Result{Packet: Redact(p), RepoVisibility: v, Forced: false}
	}
	full := *p
	full.Visibility = "full"
	return Result{Packet: &full, RepoVisibility: v, Forced: false}
}
