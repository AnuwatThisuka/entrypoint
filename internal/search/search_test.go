package search

import (
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

func TestPacketsRanksByMatchCount(t *testing.T) {
	a := &packet.Packet{ID: "pk_a", Goal: "fix timeout timeout", CreatedAt: "2026-01-01T00:00:00.000Z"}
	b := &packet.Packet{ID: "pk_b", Goal: "fix timeout", Decisions: []string{"bump timeout"}, CreatedAt: "2026-01-02T00:00:00.000Z"}
	c := &packet.Packet{ID: "pk_c", Goal: "unrelated", CreatedAt: "2026-01-03T00:00:00.000Z"}

	hits := Packets("timeout", []*packet.Packet{a, b, c})
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	// b has 2 occurrences (goal + decision), a has 2 (goal twice) → tie broken
	// by newer createdAt, so b first.
	if hits[0].Packet.ID != "pk_b" {
		t.Fatalf("expected pk_b first, got %s (scores %d,%d)", hits[0].Packet.ID, hits[0].Score, hits[1].Score)
	}
}

func TestPacketsMatchedDecisions(t *testing.T) {
	p := &packet.Packet{
		ID: "pk_a", Goal: "x",
		Decisions: []string{"use ret; because retry", "unrelated line"},
	}
	hits := Packets("retry", []*packet.Packet{p})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if len(hits[0].MatchedDecisions) != 1 || hits[0].MatchedDecisions[0] != "use ret; because retry" {
		t.Fatalf("wrong matched decisions: %v", hits[0].MatchedDecisions)
	}
}

func TestPacketsTicketTitleSearched(t *testing.T) {
	p := &packet.Packet{ID: "pk_a", Goal: "x", Ticket: &packet.Ticket{ID: "#1", Title: "flaky login", Source: "github"}}
	if hits := Packets("flaky", []*packet.Packet{p}); len(hits) != 1 {
		t.Fatalf("ticket title not searched: %v", hits)
	}
}

func TestPacketsEmptyQuery(t *testing.T) {
	if hits := Packets("   ", []*packet.Packet{{ID: "pk_a", Goal: "x"}}); hits != nil {
		t.Fatalf("expected nil for empty query, got %v", hits)
	}
}
