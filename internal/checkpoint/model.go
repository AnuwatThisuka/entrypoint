// Package checkpoint is the normalized core domain: the single canonical
// Session type that every external format is mapped into by an Importer.
//
// The core owns no source-native schema. It must never import an
// internal/importer/* package or reference a source's file names
// (metadata.json, prompt.txt, …) — the dependency points inward only
// (Invariant I4). Importers depend on checkpoint; checkpoint depends on
// nothing under importer.
package checkpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ErrIncomplete means a RawSession could not be normalized into a Session —
// e.g. it carries no source-native id, so no stable Session.ID can be
// derived. Importers wrap it with %w and enough context to locate the record.
var ErrIncomplete = errors.New("checkpoint: incomplete session")

// Visibility controls how much of a session may be disclosed. Unknown or
// empty values normalize to Redacted: on ambiguity we fail toward *less*
// disclosure (Invariant I3).
type Visibility string

const (
	// VisibilityFull permits the business-readable summary to be shown.
	VisibilityFull Visibility = "full"
	// VisibilityRedacted is the private default for anything unrecognized.
	VisibilityRedacted Visibility = "redacted"
)

// Source records which importer produced a Session and the record's identity
// in that source's own namespace. It is the basis of the derived Session.ID.
type Source struct {
	// Importer is the registered importer name, e.g. "entrypoint" | "entire".
	Importer string
	// NativeID is the record's id in the source's namespace (a packet id, an
	// Entire checkpoint id, …). Required — an empty NativeID is ErrIncomplete.
	NativeID string
	// Ref is the git ref the record was read from, when known.
	Ref string
}

// Commit links a session to the code commit it produced, when known.
type Commit struct {
	SHA    string
	Branch string
}

// Ticket links a session to the issue that motivated the work.
type Ticket struct {
	ID     string
	Title  string
	Source string // "manual" | "github" | "jira" | "linear"
}

// Summary is the small, business-readable digest stored by default. Raw
// prompts and transcripts are never inlined here — they stay by-reference in
// the git ref (Invariant I3).
type Summary struct {
	Goal          string
	Decisions     []string
	Done          []string
	Next          []string
	OpenQuestions []string
}

// Session is the one normalized checkpoint type. Every importer maps its
// source format into this shape; nothing downstream (index, dashboard,
// report) sees a source-native struct.
type Session struct {
	// ID is the stable, content-derived identity used for dedup and upsert.
	// Populated by Normalize via DeriveID — importers may leave it blank.
	ID string

	Source     Source
	Commit     Commit
	Ticket     *Ticket
	Agent      string
	Model      string
	CreatedAt  time.Time
	Visibility Visibility

	Summary      Summary
	FilesTouched []string

	// Extra preserves source fields we do not model, so an unknown key from a
	// competitor's format churn is retained rather than dropped.
	Extra map[string]any
}

// DeriveID returns the stable Session.ID for a Source: a content hash over
// the importer name and native id. It is deterministic across re-imports of
// the same record, so at-least-once ingest dedups cleanly on ID (Invariant
// I1 / Phase E). Returns "" when NativeID is empty.
func DeriveID(s Source) string {
	if s.NativeID == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s.Importer + "\x00" + s.NativeID))
	return "cs_" + hex.EncodeToString(h[:])[:16]
}

// Normalize finalizes a Session in place: derives the ID, normalizes the
// timestamp to UTC, defaults Visibility to the private value unless the
// source explicitly said "full", and tidies the summary/file lists. It
// returns ErrIncomplete when no stable ID can be derived.
func Normalize(s *Session) error {
	s.ID = DeriveID(s.Source)
	if s.ID == "" {
		return ErrIncomplete
	}

	if !s.CreatedAt.IsZero() {
		s.CreatedAt = s.CreatedAt.UTC()
	}

	// Fail safe: only an explicit "full" keeps full visibility.
	if s.Visibility != VisibilityFull {
		s.Visibility = VisibilityRedacted
	}

	s.FilesTouched = cleanStrings(s.FilesTouched)
	s.Summary.Decisions = cleanStrings(s.Summary.Decisions)
	s.Summary.Done = cleanStrings(s.Summary.Done)
	s.Summary.Next = cleanStrings(s.Summary.Next)
	s.Summary.OpenQuestions = cleanStrings(s.Summary.OpenQuestions)
	s.Summary.Goal = strings.TrimSpace(s.Summary.Goal)

	if s.Ticket != nil && s.Ticket.ID == "" {
		s.Ticket = nil
	}
	return nil
}

// cleanStrings trims, drops empties, and de-duplicates while preserving order.
func cleanStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
