// Package entire maps Entire's on-disk checkpoint format into the normalized
// checkpoint.Session. It is the ONLY package permitted to know Entire's file
// names (metadata.json, prompt.txt, full.jsonl) and field names — this quarantines
// a fast-moving competitor's format churn behind the adapter boundary (I4).
//
// Provenance: this adapter is a clean-room mapping written against the observed
// shape of Entire's export. No Entire source code is copied here (see NOTICE).
package entire

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/AnuwatThisuka/entrypoint/internal/checkpoint"
)

// Name is the importer id stamped onto Source.Importer.
const Name = "entire"

// File names inside an Entire checkpoint session subtree. Only metadata.json is
// ever read: prompt.txt and full.jsonl are raw content kept strictly
// by-reference in the git ref, never pulled into the index (Invariant I3).
const (
	fileMetadata   = "metadata.json"
	filePrompt     = "prompt.txt"
	fileTranscript = "full.jsonl"
)

// ByReferenceFiles are the raw-content blobs Entire stores alongside
// metadata.json that entrypoint deliberately never reads into the index — they
// stay by-reference in the git ref (Invariant I3). Exposed so by-reference
// linking (Phase B) does not re-hardcode these names outside this package.
var ByReferenceFiles = []string{filePrompt, fileTranscript}

// Importer implements checkpoint.Importer for the Entire format.
type Importer struct{}

// New returns an Entire importer.
func New() Importer { return Importer{} }

// Name reports the importer id.
func (Importer) Name() string { return Name }

// metadata is the subset of Entire's metadata.json we map. Unknown keys are
// captured separately into Extra rather than dropped, so competitor format
// additions survive a round-trip.
type metadata struct {
	ID           string       `json:"id"`
	CreatedAt    string       `json:"created_at"`
	Branch       string       `json:"branch"`
	CommitSHA    string       `json:"commit_sha"`
	Agent        string       `json:"agent"`
	Model        string       `json:"model"`
	Visibility   string       `json:"visibility"`
	FilesTouched []string     `json:"files_touched"`
	Ticket       *ticketMeta  `json:"ticket"`
	Summary      *summaryMeta `json:"summary"`
}

type ticketMeta struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Source string `json:"source"`
}

type summaryMeta struct {
	Goal          string   `json:"goal"`
	Decisions     []string `json:"decisions"`
	Done          []string `json:"done"`
	Next          []string `json:"next"`
	OpenQuestions []string `json:"open_questions"`
}

// knownKeys are the top-level metadata.json fields the struct above consumes;
// everything else is preserved into Session.Extra.
var knownKeys = map[string]struct{}{
	"id": {}, "created_at": {}, "branch": {}, "commit_sha": {}, "agent": {},
	"model": {}, "visibility": {}, "files_touched": {}, "ticket": {}, "summary": {},
}

// Import reads metadata.json from raw and maps it into a normalized Session.
// It never reads prompt.txt or full.jsonl (I3). A malformed metadata.json is a
// typed error; a missing summary is tolerated (empty summary, no error).
func (Importer) Import(raw checkpoint.RawSession) (checkpoint.Session, error) {
	if raw.File == nil {
		return checkpoint.Session{}, fmt.Errorf("entire: raw session %q has no file reader: %w", raw.Path, checkpoint.ErrIncomplete)
	}
	blob, err := raw.File(fileMetadata)
	if err != nil {
		return checkpoint.Session{}, fmt.Errorf("entire: read %s for %q: %w", fileMetadata, raw.Path, err)
	}

	var meta metadata
	if err := json.Unmarshal(blob, &meta); err != nil {
		return checkpoint.Session{}, fmt.Errorf("entire: parse %s for %q: %w", fileMetadata, raw.Path, checkpoint.ErrIncomplete)
	}

	// Second pass for unknown fields → Extra. A parse failure here cannot
	// happen if the first Unmarshal succeeded, but tolerate it defensively.
	extra := map[string]any{}
	var all map[string]any
	if err := json.Unmarshal(blob, &all); err == nil {
		for k, v := range all {
			if _, known := knownKeys[k]; !known {
				extra[k] = v
			}
		}
	}
	if len(extra) == 0 {
		extra = nil
	}

	s := checkpoint.Session{
		Source: checkpoint.Source{
			Importer: Name,
			NativeID: meta.ID,
			Ref:      raw.Ref,
		},
		Commit: checkpoint.Commit{
			SHA:    meta.CommitSHA,
			Branch: meta.Branch,
		},
		Agent:        meta.Agent,
		Model:        meta.Model,
		Visibility:   checkpoint.Visibility(meta.Visibility),
		FilesTouched: meta.FilesTouched,
		Extra:        extra,
	}

	if meta.CreatedAt != "" {
		t, perr := parseTime(meta.CreatedAt)
		if perr != nil {
			return checkpoint.Session{}, fmt.Errorf("entire: created_at %q for %q: %w", meta.CreatedAt, raw.Path, checkpoint.ErrIncomplete)
		}
		s.CreatedAt = t
	}

	if meta.Ticket != nil {
		s.Ticket = &checkpoint.Ticket{
			ID:     meta.Ticket.ID,
			Title:  meta.Ticket.Title,
			Source: meta.Ticket.Source,
		}
	}

	if meta.Summary != nil {
		s.Summary = checkpoint.Summary{
			Goal:          meta.Summary.Goal,
			Decisions:     meta.Summary.Decisions,
			Done:          meta.Summary.Done,
			Next:          meta.Summary.Next,
			OpenQuestions: meta.Summary.OpenQuestions,
		}
	}

	if err := checkpoint.Normalize(&s); err != nil {
		return checkpoint.Session{}, fmt.Errorf("entire: normalize %q: %w", raw.Path, err)
	}
	return s, nil
}

// parseTime accepts RFC3339 (with or without a zone) and normalizes to UTC.
// A value carrying an offset is converted; a value with no zone is read as
// UTC rather than guessed as local, so imports are deterministic.
func parseTime(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	// Zone-less fallback, interpreted as UTC.
	if t, err := time.Parse("2006-01-02T15:04:05", v); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q", v)
}
