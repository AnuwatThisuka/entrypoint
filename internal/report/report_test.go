package report

import (
	"strings"
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

func TestParseDateBareToIsEndOfDay(t *testing.T) {
	to, err := ParseDate("2026-07-14", "to")
	if err != nil {
		t.Fatal(err)
	}
	if to.Hour() != 23 || to.Minute() != 59 {
		t.Fatalf("expected end-of-day, got %v", to)
	}
	from, err := ParseDate("2026-07-14", "from")
	if err != nil {
		t.Fatal(err)
	}
	if from.Hour() != 0 {
		t.Fatalf("expected start-of-day, got %v", from)
	}
}

func TestParseDateInvalid(t *testing.T) {
	if _, err := ParseDate("not-a-date", "from"); err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestFilterByDate(t *testing.T) {
	from, _ := ParseDate("2026-07-01", "from")
	to, _ := ParseDate("2026-07-31", "to")
	in := packet.BranchPacket{Packet: &packet.Packet{ID: "in", CreatedAt: "2026-07-15T12:00:00.000Z"}, CommitSha: "a"}
	out := packet.BranchPacket{Packet: &packet.Packet{ID: "out", CreatedAt: "2026-08-15T12:00:00.000Z"}, CommitSha: "b"}
	got := FilterByDate([]packet.BranchPacket{in, out}, DateRange{From: from, To: to})
	if len(got) != 1 || got[0].Packet.ID != "in" {
		t.Fatalf("expected only in-range packet, got %+v", got)
	}
}

func TestToCSVEscaping(t *testing.T) {
	rows := []Row{{PacketID: "pk_a", Goal: `has, comma "and quote"`, Decision: "d", Commit: "abc", Date: "2026-07-14T00:00:00.000Z", Signed: "n"}}
	csv := ToCSV(rows)
	if !strings.HasPrefix(csv, "packet_id,ticket,goal,decision,commit,date,signed\n") {
		t.Fatalf("missing header: %q", csv)
	}
	if !strings.Contains(csv, `"has, comma ""and quote"""`) {
		t.Fatalf("bad escaping: %q", csv)
	}
}

func TestToRowsNewestFirst(t *testing.T) {
	older := packet.BranchPacket{Packet: &packet.Packet{ID: "old", CreatedAt: "2026-07-01T00:00:00.000Z"}}
	newer := packet.BranchPacket{Packet: &packet.Packet{ID: "new", CreatedAt: "2026-07-20T00:00:00.000Z"}}
	rows := ToRows([]packet.BranchPacket{older, newer})
	if rows[0].PacketID != "new" {
		t.Fatalf("expected newest first, got %v", rows)
	}
}

func TestToPDFIsValidHeader(t *testing.T) {
	pdf := ToPDF([]Row{{PacketID: "pk_a", Goal: "g", Commit: "abc12345", Date: "2026-07-14T00:00:00.000Z", Signed: "n"}}, "Title")
	if !strings.HasPrefix(string(pdf), "%PDF-1.4") {
		t.Fatalf("not a PDF: %q", pdf[:20])
	}
	if !strings.Contains(string(pdf), "%%EOF") {
		t.Fatal("missing EOF marker")
	}
}
