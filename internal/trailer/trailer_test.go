package trailer

import (
	"reflect"
	"testing"
)

func TestParsePacketIDs(t *testing.T) {
	msg := "feat: thing\n\nBody text.\n\nEntrypoint-Packet: pk_deadbeef\n"
	got := ParsePacketIDs(msg)
	want := []string{"pk_deadbeef"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParsePacketIDsMultiple(t *testing.T) {
	msg := "x\n\nEntrypoint-Packet: pk_aaaaaaaa\nEntrypoint-Packet: pk_bbbbbbbb\n"
	got := ParsePacketIDs(msg)
	if len(got) != 2 || got[0] != "pk_aaaaaaaa" || got[1] != "pk_bbbbbbbb" {
		t.Fatalf("got %v", got)
	}
}

func TestParsePacketIDsNone(t *testing.T) {
	if ids := ParsePacketIDs("no trailer here\n"); len(ids) != 0 {
		t.Fatalf("expected none, got %v", ids)
	}
}

func TestParseLinkedIDs(t *testing.T) {
	msg := "feat: thing\n\nEntrypoint-Packet: pk_deadbeef\n" +
		"Entire-Checkpoint: chk_123\nEntire-Metadata: meta/abc-1\n"
	got := ParseLinkedIDs(msg)
	want := []string{"pk_deadbeef", "chk_123", "meta/abc-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseLinkedIDsIgnoresUnknownTrailers(t *testing.T) {
	msg := "x\n\nSigned-off-by: someone\nCo-Authored-By: other\n"
	if ids := ParseLinkedIDs(msg); len(ids) != 0 {
		t.Fatalf("expected none, got %v", ids)
	}
}

func TestStripPacketTrailers(t *testing.T) {
	msg := "subject\n\nEntrypoint-Packet: pk_deadbeef\nSigned-off-by: x\n"
	got := StripPacketTrailers(msg)
	if ParsePacketIDs(got) != nil && len(ParsePacketIDs(got)) != 0 {
		t.Fatalf("trailer not stripped: %q", got)
	}
	if want := "subject\n\nSigned-off-by: x\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
