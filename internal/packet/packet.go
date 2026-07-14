// Package packet defines the core Packet unit and its read/write path
// over git notes. Notes stay attached to their original commit sha, so
// after a rebase/squash the trailer id (which travels with the message)
// is the reliable join key — not the commit sha.
package packet

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/trailer"
)

// Ticket links a packet to the issue that motivated the work.
type Ticket struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Source string `json:"source"` // "manual" | "github" | "jira" | "linear"
}

// Block is block-level blame data derived from a diff hunk.
type Block struct {
	File        string `json:"file"`
	Range       string `json:"range"` // e.g. "L45-L60"
	Type        string `json:"type"`  // "agent" | "human"
	DecisionRef string `json:"decisionRef,omitempty"`
}

// State is the work-in-progress snapshot.
type State struct {
	Done       []string `json:"done"`
	InProgress string   `json:"inProgress,omitempty"`
	Next       []string `json:"next"`
}

// Packet is the core unit: a short, structured checkpoint of context.
type Packet struct {
	ID            string   `json:"id"`      // pk_XXXXXXXX
	Version       int      `json:"version"` // monotonic per branch
	CreatedAt     string   `json:"createdAt"`
	Branch        string   `json:"branch"`
	Ticket        *Ticket  `json:"ticket,omitempty"`
	Goal          string   `json:"goal"`
	State         State    `json:"state"`
	Decisions     []string `json:"decisions"`
	OpenQuestions []string `json:"openQuestions,omitempty"`
	FilesTouched  []string `json:"filesTouched"`
	Blocks        []Block  `json:"blocks,omitempty"`
	Visibility    string   `json:"visibility"` // "full" | "redacted"
	// Signature is Phase 8 opt-in — ASCII-armored GPG signature over body.
	Signature string `json:"signature,omitempty"`
}

// NewID mints a fresh packet id (pk_ + 4 random bytes hex).
func NewID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "pk_" + hex.EncodeToString(b)
}

// Marshal renders a packet as the 2-space-indented JSON stored in a note.
func Marshal(p *Packet) (string, error) {
	out, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

// ReadAll returns every packet in the notes ref, indexed by id. Non-JSON
// notes on our ref are skipped — not ours to interpret.
func ReadAll(cwd string) (map[string]*Packet, error) {
	packets := make(map[string]*Packet)
	for _, note := range git.NotesList(cwd) {
		raw, ok := git.NotesShow(cwd, note.CommitSha)
		if !ok {
			continue
		}
		var p Packet
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		if p.ID != "" {
			cp := p
			packets[p.ID] = &cp
		}
	}
	return packets, nil
}

// BranchPacket pairs a packet with the commit it is attached to.
type BranchPacket struct {
	Packet    *Packet
	CommitSha string
}

// ListBranch returns packets reachable from revs (default HEAD), newest
// commit first, resolved trailer-first then joined to note bodies by id.
func ListBranch(cwd string, revs ...string) ([]BranchPacket, error) {
	byID, err := ReadAll(cwd)
	if err != nil {
		return nil, err
	}
	commits, err := git.LogCommits(cwd, revs...)
	if err != nil {
		return nil, err
	}
	var result []BranchPacket
	for _, c := range commits {
		for _, id := range trailer.ParsePacketIDs(c.Message) {
			if p, ok := byID[id]; ok {
				result = append(result, BranchPacket{Packet: p, CommitSha: c.Sha})
			}
		}
	}
	return result, nil
}

// ListReachable returns packets reachable from any branch/tag/remote (plus
// HEAD for detached checkouts), newest first, each id once. Used by report
// so it never shows packets that log/why would not.
func ListReachable(cwd string) ([]BranchPacket, error) {
	byID, err := ReadAll(cwd)
	if err != nil {
		return nil, err
	}
	if len(byID) == 0 {
		return nil, nil
	}
	commits, err := git.LogCommits(cwd, "HEAD", "--branches", "--tags", "--remotes")
	if err != nil {
		return nil, err
	}
	var result []BranchPacket
	seen := make(map[string]bool)
	for _, c := range commits {
		for _, id := range trailer.ParsePacketIDs(c.Message) {
			if seen[id] {
				continue
			}
			if p, ok := byID[id]; ok {
				seen[id] = true
				result = append(result, BranchPacket{Packet: p, CommitSha: c.Sha})
			}
		}
	}
	return result, nil
}

// NextVersion is the next monotonic version for the current branch.
func NextVersion(cwd string) (int, error) {
	existing, err := ListBranch(cwd)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, bp := range existing {
		if bp.Packet.Version > max {
			max = bp.Packet.Version
		}
	}
	return max + 1, nil
}

// AttachToHead appends the trailer to HEAD's message (amend) then stores
// the packet JSON as a note on the new HEAD sha. With force, an existing
// trailer is replaced; without it, a HEAD that already carries a packet
// is an error ("commit your new work first").
func AttachToHead(cwd string, p *Packet, force bool) (string, error) {
	message, err := git.HeadMessage(cwd)
	if err != nil {
		return "", err
	}
	if existing := trailer.ParsePacketIDs(message); len(existing) > 0 {
		if !force {
			return "", fmt.Errorf(
				"HEAD already has an entrypoint packet (%s). "+
					"Make a new commit first, or pass --force to replace it.",
				strings.Join(existing, ", "))
		}
		message = trailer.StripPacketTrailers(message)
	}

	message, err = trailer.AppendPacketTrailer(cwd, message, p.ID)
	if err != nil {
		return "", err
	}
	commitSha, err := git.AmendHeadMessage(cwd, message)
	if err != nil {
		return "", err
	}
	body, err := Marshal(p)
	if err != nil {
		return "", err
	}
	if err := git.NotesAdd(cwd, commitSha, body); err != nil {
		return "", err
	}
	return commitSha, nil
}
