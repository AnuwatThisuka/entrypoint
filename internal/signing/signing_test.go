package signing

import (
	"strings"
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

func sample() *packet.Packet {
	return &packet.Packet{
		ID: "pk_abcd1234", Version: 1, CreatedAt: "2026-07-14T00:00:00.000Z",
		Branch: "main", Goal: "g",
		State:     packet.State{Done: []string{"a"}, Next: []string{}},
		Decisions: []string{"d"}, FilesTouched: []string{"x.go"},
		Visibility: "full",
	}
}

func TestCanonicalBodyExcludesSignature(t *testing.T) {
	p := sample()
	p.Signature = "SHOULD-NOT-APPEAR"
	body, err := CanonicalBody(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "SHOULD-NOT-APPEAR") {
		t.Fatalf("signature leaked into canonical body: %s", body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatal("canonical body should end with newline")
	}
}

func TestCanonicalBodyDeterministic(t *testing.T) {
	a, _ := CanonicalBody(sample())
	b, _ := CanonicalBody(sample())
	if a != b {
		t.Fatalf("canonical body not deterministic:\n%s\n%s", a, b)
	}
}

func TestCanonicalBodyNoHTMLEscape(t *testing.T) {
	p := sample()
	p.Goal = "fix <div> & stuff"
	body, _ := CanonicalBody(p)
	// JS JSON.stringify leaves <, >, & literal — we must match to keep
	// signatures portable.
	if !strings.Contains(body, "<div> & stuff") {
		t.Fatalf("expected literal HTML chars, got %s", body)
	}
}

func TestVerifyUnsignedIsNotAnError(t *testing.T) {
	res, err := Verify("", "", sample())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != Unsigned {
		t.Fatalf("expected unsigned, got %s", res.Status)
	}
}

func TestSignerIdentity(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		wantUID   string
		wantEmail string
	}{
		{
			name:      "name and email",
			status:    "[GNUPG:] GOODSIG A1B2C3D4E5F6 Ada Lovelace <ada@example.com>",
			wantUID:   "Ada Lovelace <ada@example.com>",
			wantEmail: "ada@example.com",
		},
		{
			name:      "email is lowercased",
			status:    "[GNUPG:] GOODSIG DEADBEEF Grace <Grace@Example.COM>",
			wantUID:   "Grace <Grace@Example.COM>",
			wantEmail: "grace@example.com",
		},
		{
			name:      "uid without email",
			status:    "[GNUPG:] GOODSIG DEADBEEF anonymous",
			wantUID:   "anonymous",
			wantEmail: "",
		},
		{
			name:      "no goodsig line",
			status:    "[GNUPG:] NO_PUBKEY DEADBEEF",
			wantUID:   "",
			wantEmail: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uid, email := signerIdentity(tc.status)
			if uid != tc.wantUID {
				t.Errorf("uid: got %q want %q", uid, tc.wantUID)
			}
			if email != tc.wantEmail {
				t.Errorf("email: got %q want %q", email, tc.wantEmail)
			}
		})
	}
}
