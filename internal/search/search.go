// Package search is why-search: plain keyword matching over goal,
// decisions, and ticket title, ranked by total match count. Packet bodies
// are short by design, so grep-style counting is enough — no embeddings.
package search

import (
	"sort"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// Hit is one matched packet with its score and matching decision lines.
type Hit struct {
	Packet           *packet.Packet
	Score            int
	MatchedDecisions []string
}

func countOccurrences(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	count := 0
	for i := 0; ; {
		idx := strings.Index(haystack[i:], needle)
		if idx < 0 {
			break
		}
		count++
		i += idx + len(needle)
	}
	return count
}

// Packets ranks packets by keyword match count, highest first, breaking
// ties by newest createdAt.
func Packets(query string, packets []*packet.Packet) []Hit {
	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return nil
	}

	var hits []Hit
	for _, p := range packets {
		fields := make([]string, 0, len(p.Decisions)+2)
		fields = append(fields, strings.ToLower(p.Goal))
		for _, d := range p.Decisions {
			fields = append(fields, strings.ToLower(d))
		}
		if p.Ticket != nil {
			fields = append(fields, strings.ToLower(p.Ticket.Title))
		} else {
			fields = append(fields, "")
		}

		score := 0
		for _, kw := range keywords {
			for _, f := range fields {
				score += countOccurrences(f, kw)
			}
		}
		if score == 0 {
			continue
		}

		var matched []string
		for _, d := range p.Decisions {
			low := strings.ToLower(d)
			for _, kw := range keywords {
				if strings.Contains(low, kw) {
					matched = append(matched, d)
					break
				}
			}
		}

		hits = append(hits, Hit{Packet: p, Score: score, MatchedDecisions: matched})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Packet.CreatedAt > hits[j].Packet.CreatedAt
	})
	return hits
}
