package blocks

import (
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

func TestParseDiffTagsAgentAndHuman(t *testing.T) {
	diff := "" +
		"+++ b/agent.go\n" +
		"@@ -0,0 +1,3 @@\n" +
		"+++ b/human.go\n" +
		"@@ -0,0 +10,2 @@\n"
	agentFiles := map[string]bool{"agent.go": true}

	got := ParseDiff(diff, agentFiles)
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %+v", len(got), got)
	}
	if got[0].File != "agent.go" || got[0].Range != "L1-L3" || got[0].Type != "agent" {
		t.Fatalf("bad agent block: %+v", got[0])
	}
	if got[1].File != "human.go" || got[1].Range != "L10-L11" || got[1].Type != "human" {
		t.Fatalf("bad human block: %+v", got[1])
	}
}

func TestParseDiffSkipsPureDeletion(t *testing.T) {
	diff := "+++ b/x.go\n@@ -1,2 +0,0 @@\n"
	if got := ParseDiff(diff, nil); len(got) != 0 {
		t.Fatalf("expected no blocks for pure deletion, got %+v", got)
	}
}

func TestParseDiffFileDeletion(t *testing.T) {
	diff := "+++ /dev/null\n@@ -1,3 +0,0 @@\n"
	if got := ParseDiff(diff, nil); len(got) != 0 {
		t.Fatalf("expected no blocks for file deletion, got %+v", got)
	}
}

func TestContains(t *testing.T) {
	b := packet.Block{File: "a.go", Range: "L5-L10", Type: "agent"}
	if !Contains(b, "a.go", 7) {
		t.Fatal("7 should be in L5-L10")
	}
	if Contains(b, "a.go", 11) {
		t.Fatal("11 should not be in L5-L10")
	}
	if Contains(b, "b.go", 7) {
		t.Fatal("different file should not match")
	}
}
