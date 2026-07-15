package entrypoint

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/checkpoint"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

func samplePacket() *packet.Packet {
	return &packet.Packet{
		ID:            "pk_abcd1234",
		Version:       3,
		CreatedAt:     "2026-01-02T15:04:05Z",
		Branch:        "feature/x",
		Ticket:        &packet.Ticket{ID: "GH-1", Title: "t", Source: "github"},
		Goal:          "ship",
		State:         packet.State{Done: []string{"a"}, InProgress: "b", Next: []string{"c"}},
		Decisions:     []string{"d1"},
		OpenQuestions: []string{"q1"},
		FilesTouched:  []string{"a.go"},
		Blocks:        []packet.Block{{File: "a.go", Range: "L1-L2", Type: "agent"}},
		Visibility:    "full",
	}
}

func TestFromPacketMapping(t *testing.T) {
	s, err := FromPacket(samplePacket(), "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if s.Source.Importer != "entrypoint" || s.Source.NativeID != "pk_abcd1234" {
		t.Errorf("bad source: %+v", s.Source)
	}
	if s.Commit.SHA != "deadbeef" || s.Commit.Branch != "feature/x" {
		t.Errorf("bad commit: %+v", s.Commit)
	}
	if s.Summary.Goal != "ship" || s.Summary.Done[0] != "a" || s.Summary.Next[0] != "c" {
		t.Errorf("bad summary: %+v", s.Summary)
	}
	if s.Ticket == nil || s.Ticket.Source != "github" {
		t.Errorf("bad ticket: %+v", s.Ticket)
	}
	if s.CreatedAt.Hour() != 15 {
		t.Errorf("created_at not parsed: %v", s.CreatedAt)
	}
	// Native fields with no normalized home are preserved.
	if s.Extra["inProgress"] != "b" {
		t.Errorf("inProgress not preserved: %#v", s.Extra["inProgress"])
	}
	if s.Extra["version"] != 3 {
		t.Errorf("version not preserved: %#v", s.Extra["version"])
	}
	if _, ok := s.Extra["blocks"]; !ok {
		t.Errorf("blocks not preserved: %#v", s.Extra)
	}
}

func TestImportFromBlob(t *testing.T) {
	blob, _ := json.Marshal(samplePacket())
	raw := checkpoint.RawSession{
		Importer: Name,
		Ref:      "refs/entrypoint/packets/v1",
		Path:     "sessions/pk_abcd1234",
		File: func(name string) ([]byte, error) {
			if name != fileName {
				t.Fatalf("unexpected blob read %q", name)
			}
			return blob, nil
		},
	}
	s, err := New().Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.Source.Ref != "refs/entrypoint/packets/v1" || s.Source.NativeID != "pk_abcd1234" {
		t.Errorf("bad source: %+v", s.Source)
	}
	// Commit SHA is unknown from the packet body alone in Phase A.
	if s.Commit.SHA != "" {
		t.Errorf("commit sha should be empty from blob import, got %q", s.Commit.SHA)
	}
}

func TestFromPacketIncompleteWithoutID(t *testing.T) {
	p := samplePacket()
	p.ID = ""
	if _, err := FromPacket(p, ""); !errors.Is(err, checkpoint.ErrIncomplete) {
		t.Fatalf("want ErrIncomplete, got %v", err)
	}
}

func TestImportBadTime(t *testing.T) {
	p := samplePacket()
	p.CreatedAt = "not-a-time"
	blob, _ := json.Marshal(p)
	raw := checkpoint.RawSession{
		Importer: Name,
		File:     func(string) ([]byte, error) { return blob, nil },
	}
	if _, err := New().Import(raw); !errors.Is(err, checkpoint.ErrIncomplete) {
		t.Fatalf("want ErrIncomplete for bad time, got %v", err)
	}
}
