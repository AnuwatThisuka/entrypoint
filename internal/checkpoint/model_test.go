package checkpoint

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeriveIDDeterministicAndScoped(t *testing.T) {
	a := DeriveID(Source{Importer: "entire", NativeID: "chk_1"})
	again := DeriveID(Source{Importer: "entire", NativeID: "chk_1"})
	if a == "" || a != again {
		t.Fatalf("DeriveID not deterministic: %q vs %q", a, again)
	}
	// Same native id under a different importer must not collide.
	if other := DeriveID(Source{Importer: "entrypoint", NativeID: "chk_1"}); other == a {
		t.Fatalf("importer not part of identity: both derived %q", a)
	}
	if got := DeriveID(Source{Importer: "entire", NativeID: ""}); got != "" {
		t.Fatalf("empty NativeID must derive empty id, got %q", got)
	}
}

func TestNormalizeVisibilityFailsSafe(t *testing.T) {
	cases := map[string]Visibility{
		"full":     VisibilityFull,
		"redacted": VisibilityRedacted,
		"":         VisibilityRedacted,
		"public":   VisibilityRedacted, // unknown → private
	}
	for in, want := range cases {
		s := Session{Source: Source{Importer: "x", NativeID: "1"}, Visibility: Visibility(in)}
		if err := Normalize(&s); err != nil {
			t.Fatalf("Normalize(%q): %v", in, err)
		}
		if s.Visibility != want {
			t.Errorf("visibility %q → %q, want %q", in, s.Visibility, want)
		}
	}
}

func TestNormalizeDerivesIDAndUTC(t *testing.T) {
	loc := time.FixedZone("UTC+7", 7*3600)
	s := Session{
		Source:    Source{Importer: "entire", NativeID: "chk_1"},
		CreatedAt: time.Date(2026, 1, 2, 15, 4, 5, 0, loc),
	}
	if err := Normalize(&s); err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("ID not derived")
	}
	if s.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt not UTC: %v", s.CreatedAt.Location())
	}
	if h := s.CreatedAt.Hour(); h != 8 { // 15:04 +07:00 == 08:04 UTC
		t.Errorf("UTC hour = %d, want 8", h)
	}
}

func TestNormalizeIncompleteWithoutNativeID(t *testing.T) {
	s := Session{Source: Source{Importer: "entire"}}
	if err := Normalize(&s); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("want ErrIncomplete, got %v", err)
	}
}

func TestNormalizeCleansLists(t *testing.T) {
	s := Session{
		Source:       Source{Importer: "x", NativeID: "1"},
		FilesTouched: []string{"a.go", " a.go ", "", "b.go"},
		Summary:      Summary{Goal: "  ship it  ", Decisions: []string{"d1", "d1", ""}},
		Ticket:       &Ticket{ID: ""}, // empty ticket dropped
	}
	if err := Normalize(&s); err != nil {
		t.Fatal(err)
	}
	if got := s.FilesTouched; len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("FilesTouched not deduped/trimmed: %v", got)
	}
	if s.Summary.Goal != "ship it" {
		t.Errorf("goal not trimmed: %q", s.Summary.Goal)
	}
	if len(s.Summary.Decisions) != 1 {
		t.Errorf("decisions not deduped: %v", s.Summary.Decisions)
	}
	if s.Ticket != nil {
		t.Errorf("empty ticket should be dropped, got %+v", s.Ticket)
	}
}

// stubImporter is a minimal Importer for registry tests.
type stubImporter struct {
	name string
	out  Session
}

func (s stubImporter) Name() string { return s.name }
func (s stubImporter) Import(RawSession) (Session, error) {
	return s.out, nil
}

func TestRegistryDispatch(t *testing.T) {
	r := NewRegistry()
	r.Register(stubImporter{name: "a", out: Session{ID: "from-a"}})

	got, err := r.Import(RawSession{Importer: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "from-a" {
		t.Errorf("dispatched to wrong importer: %+v", got)
	}

	if _, err := r.Import(RawSession{Importer: "nope"}); !errors.Is(err, ErrIncomplete) {
		t.Errorf("unknown importer should be ErrIncomplete, got %v", err)
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("registering duplicate importer should panic")
		}
	}()
	r := NewRegistry()
	r.Register(stubImporter{name: "dup"})
	r.Register(stubImporter{name: "dup"})
}

// Index is only an interface in Phase A; assert its sentinel is distinct so
// callers can rely on errors.Is discrimination.
func TestIndexSentinelsDistinct(t *testing.T) {
	if errors.Is(ErrNotFound, ErrIncomplete) {
		t.Fatal("ErrNotFound and ErrIncomplete must be distinct")
	}
	_ = context.Background()
}
