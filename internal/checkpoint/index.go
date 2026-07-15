package checkpoint

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Index.Get when no session has the given id.
var ErrNotFound = errors.New("checkpoint: session not found")

// Query filters a listing of sessions. Zero-value fields are "no filter".
// The index is a derived projection: every field here maps to a column that
// was itself derived from a Session that came from git (Invariant I1). No
// business logic lives in the query — filters are mechanical.
type Query struct {
	Branch   string
	TicketID string
	Agent    string
	Model    string
	// TextMatch is a keyword search over the summary (goal/decisions/done/next).
	TextMatch string
	// Limit caps the result count; a non-positive value means the index's
	// default limit applies.
	Limit int
}

// Index is the queryable projection of sessions. Implementations (SQLite in
// v1, Postgres later) must be fully rebuildable from git — nothing may live
// only here. Results order newest-first by CreatedAt unless noted.
type Index interface {
	// Upsert writes a session, keyed by Session.ID, idempotently. Re-applying
	// the same session is a no-op change; this is what makes at-least-once
	// ingest safe.
	Upsert(ctx context.Context, s Session) error
	// Get returns the session with id, or ErrNotFound.
	Get(ctx context.Context, id string) (Session, error)
	// Query returns sessions matching q, newest first, honoring Limit.
	Query(ctx context.Context, q Query) ([]Session, error)
}
