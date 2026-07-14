package summarize

import (
	"strings"
	"testing"
)

func TestParseSummaryStrictJSON(t *testing.T) {
	s := ParseSummary(`{"goal":"do x","done":["a"],"next":["b"],"decisions":["c"]}`)
	if s == nil {
		t.Fatal("expected a summary")
	}
	if s.Goal != "do x" || len(s.Done) != 1 || s.Done[0] != "a" {
		t.Fatalf("bad parse: %+v", s)
	}
}

func TestParseSummaryToleratesFences(t *testing.T) {
	s := ParseSummary("Here you go:\n```json\n{\"goal\":\"g\"}\n```\nthanks")
	if s == nil || s.Goal != "g" {
		t.Fatalf("expected goal g, got %+v", s)
	}
}

func TestParseSummaryRejectsEmptyGoal(t *testing.T) {
	if s := ParseSummary(`{"goal":"   ","done":["a"]}`); s != nil {
		t.Fatalf("expected nil for empty goal, got %+v", s)
	}
	if s := ParseSummary("no json here"); s != nil {
		t.Fatalf("expected nil for non-JSON, got %+v", s)
	}
}

func TestParseSummaryClampsListLength(t *testing.T) {
	// build a JSON array of 20 strings
	arr := "[" + strings.Repeat(`"x",`, 19) + `"x"]`
	s := ParseSummary(`{"goal":"g","done":` + arr + `}`)
	if s == nil {
		t.Fatal("expected summary")
	}
	if len(s.Done) != maxListItems {
		t.Fatalf("expected clamp to %d, got %d", maxListItems, len(s.Done))
	}
}

func TestParseSummaryClampsItemLength(t *testing.T) {
	long := strings.Repeat("a", 400)
	s := ParseSummary(`{"goal":"` + long + `"}`)
	if s == nil {
		t.Fatal("expected summary")
	}
	if len([]rune(s.Goal)) != maxItemLength {
		t.Fatalf("expected goal clamped to %d runes, got %d", maxItemLength, len([]rune(s.Goal)))
	}
	if !strings.HasSuffix(s.Goal, "…") {
		t.Fatal("expected ellipsis suffix")
	}
}
