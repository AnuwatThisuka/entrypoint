package entire

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/checkpoint"
)

// rawFrom builds a RawSession whose File serves the given blobs and records
// every requested name, so tests can assert that raw content (prompt.txt,
// full.jsonl) is never read (Invariant I3).
func rawFrom(blobs map[string]string, asked *[]string) checkpoint.RawSession {
	return checkpoint.RawSession{
		Importer: Name,
		Ref:      "refs/entire/checkpoints/v1",
		Path:     "sessions/chk_1",
		File: func(name string) ([]byte, error) {
			if asked != nil {
				*asked = append(*asked, name)
			}
			b, ok := blobs[name]
			if !ok {
				return nil, fmt.Errorf("no such blob %q", name)
			}
			return []byte(b), nil
		},
	}
}

func TestImportValid(t *testing.T) {
	var asked []string
	meta := `{
	  "id": "chk_1",
	  "created_at": "2026-01-02T15:04:05+07:00",
	  "branch": "feature/x",
	  "commit_sha": "abc123",
	  "agent": "claude-code",
	  "model": "claude-opus",
	  "visibility": "full",
	  "files_touched": ["a.go", "b.go"],
	  "ticket": {"id": "JIRA-9", "title": "do thing", "source": "jira"},
	  "summary": {"goal": "ship", "decisions": ["use go-git"], "done": ["x"], "next": ["y"], "open_questions": ["z"]}
	}`
	raw := rawFrom(map[string]string{fileMetadata: meta}, &asked)

	s, err := New().Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" || s.Source.Importer != "entire" || s.Source.NativeID != "chk_1" {
		t.Fatalf("bad source/id: %+v", s.Source)
	}
	if s.Source.Ref != "refs/entire/checkpoints/v1" {
		t.Errorf("ref not propagated: %q", s.Source.Ref)
	}
	if s.Commit.SHA != "abc123" || s.Commit.Branch != "feature/x" {
		t.Errorf("bad commit: %+v", s.Commit)
	}
	if s.Visibility != checkpoint.VisibilityFull {
		t.Errorf("visibility = %q", s.Visibility)
	}
	if s.CreatedAt.Hour() != 8 { // +07:00 normalized to UTC
		t.Errorf("created_at not UTC-normalized: %v", s.CreatedAt)
	}
	if s.Ticket == nil || s.Ticket.ID != "JIRA-9" {
		t.Errorf("ticket not mapped: %+v", s.Ticket)
	}
	if s.Summary.Goal != "ship" || len(s.Summary.Decisions) != 1 {
		t.Errorf("summary not mapped: %+v", s.Summary)
	}

	// I3: only metadata.json may be read; raw content must be untouched.
	for _, name := range asked {
		if name == filePrompt || name == fileTranscript {
			t.Fatalf("importer read raw content %q — violates I3", name)
		}
	}
}

func TestImportUnknownFieldsPreserved(t *testing.T) {
	meta := `{"id":"chk_2","visibility":"redacted","cost_usd":0.42,"future_field":{"k":"v"}}`
	s, err := New().Import(rawFrom(map[string]string{fileMetadata: meta}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if s.Extra["cost_usd"] != 0.42 {
		t.Errorf("unknown scalar not preserved: %#v", s.Extra["cost_usd"])
	}
	if _, ok := s.Extra["future_field"]; !ok {
		t.Errorf("unknown object not preserved: %#v", s.Extra)
	}
	// Known fields must not leak into Extra.
	if _, ok := s.Extra["id"]; ok {
		t.Errorf("known key leaked into Extra: %#v", s.Extra)
	}
}

func TestImportMissingSummaryTolerated(t *testing.T) {
	meta := `{"id":"chk_3","visibility":"full"}`
	s, err := New().Import(rawFrom(map[string]string{fileMetadata: meta}, nil))
	if err != nil {
		t.Fatalf("missing summary should be tolerated, got %v", err)
	}
	if s.Summary.Goal != "" || len(s.Summary.Done) != 0 {
		t.Errorf("expected empty summary, got %+v", s.Summary)
	}
}

func TestImportErrors(t *testing.T) {
	cases := []struct {
		name  string
		blobs map[string]string
	}{
		{"malformed json", map[string]string{fileMetadata: `{not json`}},
		{"empty native id", map[string]string{fileMetadata: `{"visibility":"full"}`}},
		{"bad created_at", map[string]string{fileMetadata: `{"id":"chk_4","created_at":"nope"}`}},
		{"missing metadata blob", map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New().Import(rawFrom(tc.blobs, nil))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestImportNoFileReader(t *testing.T) {
	_, err := New().Import(checkpoint.RawSession{Importer: Name})
	if !errors.Is(err, checkpoint.ErrIncomplete) {
		t.Fatalf("nil File reader should be ErrIncomplete, got %v", err)
	}
}

func TestZonelessTimeIsUTC(t *testing.T) {
	meta := `{"id":"chk_5","created_at":"2026-01-02T15:04:05"}`
	s, err := New().Import(rawFrom(map[string]string{fileMetadata: meta}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if s.CreatedAt.Location() != nil && s.CreatedAt.Hour() != 15 {
		t.Errorf("zoneless time not read as UTC 15:04: %v", s.CreatedAt)
	}
}
