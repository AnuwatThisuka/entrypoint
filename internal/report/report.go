// Package report builds auditor-readable exports (Phase 8) from existing
// packet read paths — no new storage. CSV is a flat table; the PDF is
// hand-rolled (Helvetica) since the report is a flat table and needs no
// dependency.
package report

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// Row is one flat report line.
type Row struct {
	PacketID string
	Ticket   string
	Goal     string
	Decision string
	Commit   string
	Date     string
	Signed   string // "y" | "n"
}

// DateRange is an inclusive [From, To] window.
type DateRange struct {
	From time.Time
	To   time.Time
}

var bareDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ParseDate parses YYYY-MM-DD (or a full ISO timestamp). A bare --to date
// is treated as end-of-day inclusive. Errors are plain-language.
func ParseDate(value, which string) (time.Time, error) {
	if bareDate.MatchString(value) {
		layout := "2006-01-02T15:04:05.000Z"
		suffix := "T00:00:00.000Z"
		if which == "to" {
			suffix = "T23:59:59.999Z"
		}
		t, err := time.Parse(layout, value+suffix)
		if err == nil {
			return t, nil
		}
	} else if t, err := parseISO(value); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf(
		"Invalid --%s date %q — use YYYY-MM-DD or an ISO 8601 timestamp.", which, value)
}

// parseISO accepts the common ISO 8601 shapes JS `new Date(...)` handles.
func parseISO(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date")
}

// FilterByDate keeps packets whose createdAt is within range.
func FilterByDate(packets []packet.BranchPacket, r DateRange) []packet.BranchPacket {
	var out []packet.BranchPacket
	for _, bp := range packets {
		t, err := parseISO(bp.Packet.CreatedAt)
		if err != nil {
			continue
		}
		if !t.Before(r.From) && !t.After(r.To) {
			out = append(out, bp)
		}
	}
	return out
}

// ToRows renders packets as report rows, newest createdAt first.
func ToRows(packets []packet.BranchPacket) []Row {
	sorted := make([]packet.BranchPacket, len(packets))
	copy(sorted, packets)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Packet.CreatedAt > sorted[j].Packet.CreatedAt
	})
	rows := make([]Row, 0, len(sorted))
	for _, bp := range sorted {
		rows = append(rows, rowFrom(bp.Packet, bp.CommitSha))
	}
	return rows
}

func rowFrom(p *packet.Packet, commitSha string) Row {
	ticket := ""
	if p.Ticket != nil {
		ticket = p.Ticket.ID
	}
	signed := "n"
	if p.Signature != "" {
		signed = "y"
	}
	return Row{
		PacketID: p.ID,
		Ticket:   ticket,
		Goal:     p.Goal,
		Decision: strings.Join(p.Decisions, "; "),
		Commit:   commitSha,
		Date:     p.CreatedAt,
		Signed:   signed,
	}
}

var csvSpecial = regexp.MustCompile(`[",\n\r]`)

func csvEscape(value string) string {
	if csvSpecial.MatchString(value) {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

// ToCSV renders a flat CSV with a header row.
func ToCSV(rows []Row) string {
	var b strings.Builder
	b.WriteString("packet_id,ticket,goal,decision,commit,date,signed\n")
	for _, r := range rows {
		fields := []string{r.PacketID, r.Ticket, r.Goal, r.Decision, r.Commit, r.Date, r.Signed}
		for i, f := range fields {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(csvEscape(f))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
