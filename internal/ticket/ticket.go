// Package ticket resolves a user-supplied ticket id to a Ticket, enriching
// it with a title when an adapter can. Adapters are pluggable — new sources
// (Jira, Linear) implement Adapter; GitHub assumptions stay in this package.
package ticket

import (
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// Adapter is a ticket source. Adapters are tried in order; the first that
// recognizes the id and fetches successfully wins. A nil ticket means
// "not mine / couldn't fetch" — resolution falls through to the next.
type Adapter interface {
	Source() string
	Fetch(cwd, id string) *packet.Ticket
}

// Jira/Linear adapters slot in here later.
var adapters = []Adapter{githubAdapter{}}

// Resolve maps a ticket id to a Ticket. It never fails — a broken/missing
// adapter degrades to a manual ticket (Phase 2 fallback requirement).
func Resolve(cwd, id string) packet.Ticket {
	for _, a := range adapters {
		if t := a.Fetch(cwd, id); t != nil {
			return *t
		}
	}
	return packet.Ticket{ID: id, Source: "manual"}
}
