// Package trailer parses and writes the Entrypoint-Packet commit trailer
// that joins a commit to its packet note (survives rebase/squash, since
// the trailer travels with the message).
package trailer

import (
	"regexp"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
)

// Key is the trailer key stamped into commit messages.
const Key = "Entrypoint-Packet"

var trailerLine = regexp.MustCompile(`^Entrypoint-Packet:\s*(pk_[A-Za-z0-9]+)\s*$`)

// ParsePacketIDs extracts all packet ids from a commit message, in order.
func ParsePacketIDs(message string) []string {
	var ids []string
	for _, line := range strings.Split(message, "\n") {
		if m := trailerLine.FindStringSubmatch(line); m != nil {
			ids = append(ids, m[1])
		}
	}
	return ids
}

// AppendPacketTrailer adds an `Entrypoint-Packet: <id>` trailer via
// git interpret-trailers.
func AppendPacketTrailer(cwd, message, packetID string) (string, error) {
	return git.InterpretTrailersAdd(cwd, message, Key+": "+packetID)
}

// StripPacketTrailers removes all packet trailer lines (capture --force).
func StripPacketTrailers(message string) string {
	var kept []string
	for _, line := range strings.Split(message, "\n") {
		if !trailerLine.MatchString(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
