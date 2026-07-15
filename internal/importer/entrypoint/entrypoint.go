// Package entrypoint maps entrypoint's own native packet into the normalized
// checkpoint.Session. entrypoint's "packet" and a checkpoint "session" are the
// same concept; this adapter makes checkpoint.Session the single normalized
// type so the rest of the platform never sees the native packet struct
// (Invariant I4). internal/packet keeps its capture logic and emits through here.
package entrypoint

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/AnuwatThisuka/entrypoint/internal/checkpoint"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// Name is the importer id stamped onto Source.Importer.
const Name = "entrypoint"

// fileName is the blob a native packet is serialized to inside a ref subtree.
const fileName = "packet.json"

// Importer implements checkpoint.Importer for entrypoint's native packets.
type Importer struct{}

// New returns an entrypoint importer.
func New() Importer { return Importer{} }

// Name reports the importer id.
func (Importer) Name() string { return Name }

// Import reads packet.json from raw and maps it into a normalized Session. The
// code commit SHA is not known from the packet body alone — Phase B trailer
// linkage populates Commit.SHA — so it is left empty here.
func (Importer) Import(raw checkpoint.RawSession) (checkpoint.Session, error) {
	if raw.File == nil {
		return checkpoint.Session{}, fmt.Errorf("entrypoint: raw session %q has no file reader: %w", raw.Path, checkpoint.ErrIncomplete)
	}
	blob, err := raw.File(fileName)
	if err != nil {
		return checkpoint.Session{}, fmt.Errorf("entrypoint: read %s for %q: %w", fileName, raw.Path, err)
	}
	var p packet.Packet
	if err := json.Unmarshal(blob, &p); err != nil {
		return checkpoint.Session{}, fmt.Errorf("entrypoint: parse %s for %q: %w", fileName, raw.Path, checkpoint.ErrIncomplete)
	}
	return fromPacket(&p, "", raw.Ref)
}

// FromPacket maps an in-memory native packet directly into a normalized
// Session, for the capture path (which already holds the packet and the commit
// it was attached to). It is the seam that lets internal/packet emit through
// the importer instead of duplicating the domain.
func FromPacket(p *packet.Packet, commitSHA string) (checkpoint.Session, error) {
	return fromPacket(p, commitSHA, "")
}

func fromPacket(p *packet.Packet, commitSHA, ref string) (checkpoint.Session, error) {
	s := checkpoint.Session{
		Source: checkpoint.Source{
			Importer: Name,
			NativeID: p.ID,
			Ref:      ref,
		},
		Commit: checkpoint.Commit{
			SHA:    commitSHA,
			Branch: p.Branch,
		},
		Visibility:   checkpoint.Visibility(p.Visibility),
		FilesTouched: p.FilesTouched,
		Summary: checkpoint.Summary{
			Goal:          p.Goal,
			Decisions:     p.Decisions,
			Done:          p.State.Done,
			Next:          p.State.Next,
			OpenQuestions: p.OpenQuestions,
		},
	}

	if p.Ticket != nil {
		s.Ticket = &checkpoint.Ticket{
			ID:     p.Ticket.ID,
			Title:  p.Ticket.Title,
			Source: p.Ticket.Source,
		}
	}

	if p.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, p.CreatedAt)
		if err != nil {
			return checkpoint.Session{}, fmt.Errorf("entrypoint: created_at %q for %q: %w", p.CreatedAt, p.ID, checkpoint.ErrIncomplete)
		}
		s.CreatedAt = t.UTC()
	}

	// Preserve native fields the normalized Summary does not model, rather than
	// dropping them. inProgress is a single in-flight item; version and blocks
	// are native bookkeeping. The GPG signature is intentionally NOT indexed.
	extra := map[string]any{}
	if p.State.InProgress != "" {
		extra["inProgress"] = p.State.InProgress
	}
	if p.Version != 0 {
		extra["version"] = p.Version
	}
	if len(p.Blocks) > 0 {
		extra["blocks"] = p.Blocks
	}
	if len(extra) > 0 {
		s.Extra = extra
	}

	if err := checkpoint.Normalize(&s); err != nil {
		return checkpoint.Session{}, fmt.Errorf("entrypoint: normalize %q: %w", p.ID, err)
	}
	return s, nil
}
