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

// Linkage trailer keys recognized when joining a code commit to the record it
// produced. Entrypoint-Packet is our own; the Entire-* keys let us link
// records imported from Entire's checkpoints to their commits.
const (
	EntireCheckpointKey = "Entire-Checkpoint"
	EntireMetadataKey   = "Entire-Metadata"
)

var trailerLine = regexp.MustCompile(`^Entrypoint-Packet:\s*(pk_[A-Za-z0-9]+)\s*$`)

// linkedLine matches any recognized linkage trailer. Entire native ids are not
// pk_-shaped, so the value pattern is broader than ParsePacketIDs's.
var linkedLine = regexp.MustCompile(
	`^(?:Entrypoint-Packet|Entire-Checkpoint|Entire-Metadata):\s*([A-Za-z0-9_.:/-]+)\s*$`)

// ParsePacketIDs extracts all entrypoint packet ids from a commit message, in
// order. It recognizes only the native Entrypoint-Packet trailer.
func ParsePacketIDs(message string) []string {
	var ids []string
	for _, line := range strings.Split(message, "\n") {
		if m := trailerLine.FindStringSubmatch(line); m != nil {
			ids = append(ids, m[1])
		}
	}
	return ids
}

// ParseLinkedIDs extracts every native record id referenced by a recognized
// linkage trailer — entrypoint packets and Entire checkpoints alike — in
// order. Used to join a record to the code commit that produced it (Phase B).
func ParseLinkedIDs(message string) []string {
	var ids []string
	for _, line := range strings.Split(message, "\n") {
		if m := linkedLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
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
