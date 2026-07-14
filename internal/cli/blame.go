package cli

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/AnuwatThisuka/entrypoint/internal/blocks"
	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/sessionfiles"
	"github.com/AnuwatThisuka/entrypoint/internal/trailer"
	"github.com/spf13/cobra"
)

var blameTarget = regexp.MustCompile(`^(.+):(\d+)$`)

// resolveDecisions maps a block's decisionRef (an index into decisions[] or
// free text) to the decision lines to show.
func resolveDecisions(p *packet.Packet, decisionRef string) []string {
	if decisionRef != "" {
		if idx, err := strconv.Atoi(decisionRef); err == nil && idx >= 0 && idx < len(p.Decisions) {
			return []string{p.Decisions[idx]}
		}
		return []string{decisionRef}
	}
	return p.Decisions
}

func newBlame() *cobra.Command {
	return &cobra.Command{
		Use:   "blame <target>",
		Short: "Find out why a line of code exists: entrypoint blame <file>:<line>",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			if !git.IsRepo(cwd) {
				return fmt.Errorf("Not inside a git repository.")
			}

			target := args[0]
			m := blameTarget.FindStringSubmatch(target)
			if m == nil {
				return fmt.Errorf("Expected <file>:<line>, e.g. src/db/batch.js:50 — got %q.", target)
			}
			file := m[1]
			line, _ := strconv.Atoi(m[2])

			sha, err := git.BlameLineCommit(cwd, file, line)
			if err != nil {
				return err
			}
			if sha == "" {
				fmt.Printf("%s: not committed yet — no entrypoint data.\n", target)
				return nil
			}

			message, err := git.CommitMessage(cwd, sha)
			if err != nil {
				return err
			}
			ids := trailer.ParsePacketIDs(message)
			sh := short(sha)
			if len(ids) == 0 {
				fmt.Printf("%s: commit %s has no entrypoint packet.\n", target, sh)
				return nil
			}

			byID, err := packet.ReadAll(cwd)
			if err != nil {
				return err
			}
			var p *packet.Packet
			for _, id := range ids {
				if found, ok := byID[id]; ok {
					p = found
					break
				}
			}
			if p == nil {
				fmt.Printf("%s: commit %s references a packet whose note is missing "+
					"(try `entrypoint sync`).\n", target, sh)
				return nil
			}

			relFile, err := sessionfiles.ToRepoRelative(cwd, file)
			if err != nil {
				return err
			}
			var block *packet.Block
			for i := range p.Blocks {
				if blocks.Contains(p.Blocks[i], relFile, line) {
					block = &p.Blocks[i]
					break
				}
			}

			if block != nil && block.Type == "human" {
				fmt.Printf("%s — human edit (%s, commit %s). No agent decision recorded.\n",
					target, block.Range, sh)
				return nil
			}

			scope := "no block-level data for this line"
			if block != nil {
				scope = fmt.Sprintf("agent-written (%s)", block.Range)
			}
			fmt.Printf("%s — %s, commit %s\n", target, scope, sh)
			fmt.Printf("Packet: %s (v%d) on %s\n", p.ID, p.Version, p.Branch)
			fmt.Printf("Goal: %s\n", p.Goal)
			ref := ""
			if block != nil {
				ref = block.DecisionRef
			}
			decisions := resolveDecisions(p, ref)
			if len(decisions) > 0 {
				fmt.Println("Decisions:")
				for _, d := range decisions {
					fmt.Printf("  - %s\n", d)
				}
			}
			return nil
		},
	}
}
