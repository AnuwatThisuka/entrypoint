// Package blocks derives block-level blame data (Phase 5): the hunk ranges
// of the commit a packet is attached to, each tagged agent/human by whether
// the file appears in the session's agent file-write log. File granularity
// is deliberate — the log tracks file writes, not keystrokes.
package blocks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

var (
	hunkHeader  = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	newFileLine = regexp.MustCompile(`^\+\+\+ b/(.*)$`)
	rangeRe     = regexp.MustCompile(`^L(\d+)-L(\d+)$`)
)

// ParseDiff turns unified=0 diff text into blocks.
func ParseDiff(diff string, agentFiles map[string]bool) []packet.Block {
	var out []packet.Block
	currentFile := ""

	for _, line := range strings.Split(diff, "\n") {
		if m := newFileLine.FindStringSubmatch(line); m != nil {
			currentFile = m[1]
			continue
		}
		if strings.HasPrefix(line, "+++ /dev/null") {
			currentFile = "" // file deletion — no new lines to attribute
			continue
		}

		m := hunkHeader.FindStringSubmatch(line)
		if m == nil || currentFile == "" {
			continue
		}

		start, _ := strconv.Atoi(m[1])
		count := 1
		if m[2] != "" {
			count, _ = strconv.Atoi(m[2])
		}
		if count == 0 {
			continue // pure deletion hunk
		}

		typ := "human"
		if agentFiles[currentFile] {
			typ = "agent"
		}
		out = append(out, packet.Block{
			File:  currentFile,
			Range: fmt.Sprintf("L%d-L%d", start, start+count-1),
			Type:  typ,
		})
	}
	return out
}

// ComputeHead returns blocks for the HEAD commit (root commits included).
func ComputeHead(cwd string, agentFiles map[string]bool) ([]packet.Block, error) {
	diff, err := git.Run(cwd, "show", "--unified=0", "--format=", "HEAD")
	if err != nil {
		return nil, err
	}
	return ParseDiff(diff, agentFiles), nil
}

// Contains reports whether line in file (repo-relative) falls in the block.
func Contains(b packet.Block, file string, line int) bool {
	if b.File != file {
		return false
	}
	m := rangeRe.FindStringSubmatch(b.Range)
	if m == nil {
		return false
	}
	lo, _ := strconv.Atoi(m[1])
	hi, _ := strconv.Atoi(m[2])
	return line >= lo && line <= hi
}
