package gitstore_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/checkpoint"
	"github.com/AnuwatThisuka/entrypoint/internal/gitstore"
	"github.com/AnuwatThisuka/entrypoint/internal/importer"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// newRepo makes an isolated scratch git repo. Seeding uses the git binary's
// plumbing (test-only); the gitstore library under test stays pure go-git.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "", "init", "--initial-branch=main")
	gitRun(t, dir, "", "config", "user.name", "Entrypoint Test")
	gitRun(t, dir, "", "config", "user.email", "test@entrypoint.invalid")
	gitRun(t, dir, "", "config", "commit.gpgsign", "false")
	return dir
}

func gitRun(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// hashObject writes content as a blob and returns its sha.
func hashObject(t *testing.T, dir, content string) string {
	t.Helper()
	return gitRun(t, dir, content, "hash-object", "-w", "--stdin")
}

// seedEntireRef builds a fake Entire-shaped ref: one session subdirectory
// (chk_1) holding metadata.json plus the raw blobs that must never be read.
func seedEntireRef(t *testing.T, dir, ref, metadata string) {
	t.Helper()
	metaHash := hashObject(t, dir, metadata)
	promptHash := hashObject(t, dir, "raw prompt text — must not be read")
	transcriptHash := hashObject(t, dir, `{"not":"json we parse"}`)

	subtree := gitRun(t, dir,
		"100644 blob "+metaHash+"\tmetadata.json\n"+
			"100644 blob "+promptHash+"\tprompt.txt\n"+
			"100644 blob "+transcriptHash+"\tfull.jsonl\n",
		"mktree")

	parent := gitRun(t, dir, "040000 tree "+subtree+"\tchk_1\n", "mktree")
	commit := gitRun(t, dir, "", "commit-tree", parent, "-m", "entire snapshot")
	gitRun(t, dir, "", "update-ref", ref, commit)
}

const sampleMetadata = `{
  "id": "chk_1",
  "created_at": "2026-01-02T15:04:05+07:00",
  "branch": "feature/x",
  "agent": "claude-code",
  "model": "claude-opus",
  "visibility": "full",
  "files_touched": ["a.go"],
  "ticket": {"id": "JIRA-9", "source": "jira"},
  "summary": {"goal": "ship it", "decisions": ["use go-git"], "done": ["x"], "next": ["y"]}
}`

func TestRebuildEntireRefWithCommitLinkage(t *testing.T) {
	dir := newRepo(t)
	const entireRef = "refs/entire/checkpoints/v1"
	seedEntireRef(t, dir, entireRef, sampleMetadata)

	// A code commit on main whose trailer references the checkpoint.
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "", "add", "a.go")
	gitRun(t, dir, "", "commit", "-m", "feat: thing", "-m", "Entire-Checkpoint: chk_1")
	codeCommit := gitRun(t, dir, "", "rev-parse", "HEAD")

	w, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	links, err := w.CommitLinks(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if links["chk_1"] != codeCommit {
		t.Fatalf("commit link chk_1 = %q, want %q", links["chk_1"], codeCommit)
	}

	sessions, err := w.Rebuild(ctx, gitstore.RefSpec{Ref: entireRef, Importer: "entire"}, importer.Default(), links)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.Source.Importer != "entire" || s.Source.NativeID != "chk_1" {
		t.Errorf("bad source: %+v", s.Source)
	}
	if s.Source.Ref != entireRef {
		t.Errorf("ref not propagated: %q", s.Source.Ref)
	}
	if s.Summary.Goal != "ship it" || len(s.Summary.Decisions) != 1 {
		t.Errorf("summary not mapped: %+v", s.Summary)
	}
	if s.Visibility != checkpoint.VisibilityFull {
		t.Errorf("visibility = %q", s.Visibility)
	}
	// The linkage deliverable: Commit.SHA filled from the trailer.
	if s.Commit.SHA != codeCommit {
		t.Errorf("commit linkage: SHA = %q, want %q", s.Commit.SHA, codeCommit)
	}
	if s.CreatedAt.Hour() != 8 { // +07:00 normalized to UTC
		t.Errorf("created_at not UTC-normalized: %v", s.CreatedAt)
	}
}

func TestWalkIsLazyAndReadsMetadataOnly(t *testing.T) {
	dir := newRepo(t)
	const entireRef = "refs/entire/checkpoints/v1"
	seedEntireRef(t, dir, entireRef, sampleMetadata)

	w, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	var count int
	for raw, err := range w.Walk(context.Background(), gitstore.RefSpec{Ref: entireRef, Importer: "entire"}) {
		if err != nil {
			t.Fatal(err)
		}
		count++
		if raw.Path != "chk_1" {
			t.Errorf("session path = %q", raw.Path)
		}
		// File reads exactly the blob asked for and nothing else — the raw
		// transcript is present in the tree but only read on demand (I3).
		meta, err := raw.File("metadata.json")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(meta), "chk_1") {
			t.Errorf("metadata.json content unexpected: %s", meta)
		}
		if _, err := raw.File("does-not-exist.json"); err == nil {
			t.Error("expected error reading missing blob")
		}
	}
	if count != 1 {
		t.Fatalf("walked %d sessions, want 1", count)
	}
}

func TestWritePacketPreservesSignature(t *testing.T) {
	dir := newRepo(t)
	w, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	const sig = "-----BEGIN PGP SIGNATURE-----\nDUMMYSIGDATA\n-----END PGP SIGNATURE-----"
	p := &packet.Packet{
		ID:           "pk_test0001",
		Version:      1,
		CreatedAt:    "2026-01-02T15:04:05Z",
		Branch:       "main",
		Goal:         "ship",
		State:        packet.State{Done: []string{"a"}, Next: []string{"b"}},
		Decisions:    []string{"d1"},
		FilesTouched: []string{"a.go"},
		Visibility:   "full",
		Signature:    sig,
	}

	if _, err := w.WritePacket(ctx, p); err != nil {
		t.Fatal(err)
	}

	// The written blob must be byte-identical to Marshal (signature included),
	// so verify-from-git keeps working: "not indexed" != "discarded".
	wantBody, _ := packet.Marshal(p)
	var found bool
	for raw, err := range w.Walk(ctx, gitstore.RefSpec{Ref: gitstore.PacketsRef, Importer: "entrypoint"}) {
		if err != nil {
			t.Fatal(err)
		}
		if raw.Path != p.ID {
			continue
		}
		found = true
		got, err := raw.File("packet.json")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != wantBody {
			t.Errorf("packet.json not byte-identical:\n got %q\nwant %q", got, wantBody)
		}
		var round packet.Packet
		if err := json.Unmarshal(got, &round); err != nil {
			t.Fatal(err)
		}
		if round.Signature != sig {
			t.Errorf("signature not preserved in git: %q", round.Signature)
		}
	}
	if !found {
		t.Fatal("written packet not found on PacketsRef")
	}

	// And it maps through the registry with no trailer → empty Commit.SHA.
	sessions, err := w.Rebuild(ctx, gitstore.RefSpec{Ref: gitstore.PacketsRef, Importer: "entrypoint"}, importer.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Source.NativeID != p.ID {
		t.Fatalf("rebuild from PacketsRef wrong: %+v", sessions)
	}
	if sessions[0].Commit.SHA != "" {
		t.Errorf("no trailer, but Commit.SHA = %q", sessions[0].Commit.SHA)
	}
}

func TestWritePacketAppendsSecondSession(t *testing.T) {
	dir := newRepo(t)
	w, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	mk := func(id string) *packet.Packet {
		return &packet.Packet{ID: id, Version: 1, CreatedAt: "2026-01-02T15:04:05Z", Branch: "main", Goal: "g", Visibility: "redacted"}
	}
	if _, err := w.WritePacket(ctx, mk("pk_aaaa1111")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.WritePacket(ctx, mk("pk_bbbb2222")); err != nil {
		t.Fatal(err)
	}

	sessions, err := w.Rebuild(ctx, gitstore.RefSpec{Ref: gitstore.PacketsRef, Importer: "entrypoint"}, importer.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("append lost a session: got %d, want 2", len(sessions))
	}
}
