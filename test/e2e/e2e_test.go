package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnuwatThisuka/entrypoint/internal/testutil"
)

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected to contain %q, got:\n%s", needle, haystack)
	}
}

func mustNotContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected NOT to contain %q, got:\n%s", needle, haystack)
	}
}

// ghStub returns a gh fake that answers `repo view --json isPrivate` and
// `issue view` / `auth status` deterministically.
const ghAuthOK = `#!/bin/sh
if [ "$1" = "auth" ]; then exit 0; fi
exit 1
`

func TestCaptureResumeLog(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "hi\n", "init")

	res := r.Run("capture", "--goal", "port to go", "--done", "wrote core", "--next", "tests", "--decision", "cobra CLI")
	if res.ExitCode != 0 {
		t.Fatalf("capture failed: %s", res.Stderr)
	}
	mustContain(t, res.Stdout, "Captured pk_")
	mustContain(t, res.Stdout, "on main")

	resume := r.Run("resume")
	mustContain(t, resume.Stdout, "Goal: port to go")
	mustContain(t, resume.Stdout, "wrote core")
	mustContain(t, resume.Stdout, "Files touched: a.txt")

	logOut := r.Run("log")
	mustContain(t, logOut.Stdout, "v1  pk_")
	mustContain(t, logOut.Stdout, "port to go")
}

func TestResumeAtVersion(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "1\n", "c1")
	r.Run("capture", "--goal", "first")
	r.CommitFile("b.txt", "2\n", "c2")
	r.Run("capture", "--goal", "second")

	at1 := r.Run("resume", "--at", "1")
	mustContain(t, at1.Stdout, "Goal: first")
	latest := r.Run("resume")
	mustContain(t, latest.Stdout, "Goal: second")
}

// TestTrailerSurvivesRebase confirms a packet stays resolvable after the
// commit it is attached to gets a new SHA (as a rebase would produce).
func TestTrailerSurvivesRebase(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "1\n", "base")
	r.CommitFile("b.txt", "2\n", "work")
	r.Run("capture", "--goal", "survive rebase")

	before := r.Git("rev-parse", "HEAD")
	// Rewrite HEAD's SHA while preserving its message (and the trailer),
	// exactly as a rebase reword/replay would.
	r.Git("commit", "--amend", "--no-edit", "--date=2020-01-01T00:00:00")
	after := r.Git("rev-parse", "HEAD")
	if before == after {
		t.Fatal("expected HEAD sha to change")
	}

	resume := r.Run("resume")
	if resume.ExitCode != 0 {
		t.Fatalf("resume failed after rebase: %s", resume.Stderr)
	}
	mustContain(t, resume.Stdout, "Goal: survive rebase")
}

func TestPrivacyPublicRedacts(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("gh", `#!/bin/sh
if [ "$1" = "repo" ]; then echo '{"isPrivate":false}'; exit 0; fi
exit 1
`)
	r.Git("remote", "add", "origin", "https://example.invalid/x.git")
	r.CommitFile("a.txt", "hi\n", "init")

	res := r.Run("capture", "--goal", "public goal", "--done", "secret detail", "--next", "secret next")
	mustContain(t, res.Stdout, "Public repo detected")
	mustContain(t, res.Stdout, "redacted")

	resume := r.Run("resume")
	mustContain(t, resume.Stdout, "Goal: public goal") // goal kept
	mustContain(t, resume.Stdout, "redacted")          // banner shown
	mustNotContain(t, resume.Stdout, "secret detail")  // state stripped
	mustNotContain(t, resume.Stdout, "secret next")
}

func TestPrivacyPrivateKeepsFull(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("gh", `#!/bin/sh
if [ "$1" = "repo" ]; then echo '{"isPrivate":true}'; exit 0; fi
exit 1
`)
	r.Git("remote", "add", "origin", "https://example.invalid/x.git")
	r.CommitFile("a.txt", "hi\n", "init")

	res := r.Run("capture", "--goal", "private goal", "--done", "kept detail")
	mustNotContain(t, res.Stdout, "redacted")

	resume := r.Run("resume")
	mustContain(t, resume.Stdout, "kept detail")
}

func TestForceRedactedOnPrivate(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("gh", `#!/bin/sh
if [ "$1" = "repo" ]; then echo '{"isPrivate":true}'; exit 0; fi
exit 1
`)
	r.Git("remote", "add", "origin", "https://example.invalid/x.git")
	r.CommitFile("a.txt", "hi\n", "init")

	r.Run("capture", "--goal", "g", "--done", "hushhush", "--force-redacted")
	resume := r.Run("resume")
	mustNotContain(t, resume.Stdout, "hushhush")
}

func TestTicketLinking(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("gh", `#!/bin/sh
if [ "$1" = "issue" ]; then echo '{"title":"Fix login"}'; exit 0; fi
exit 1
`)
	r.CommitFile("a.txt", "hi\n", "init")

	res := r.Run("capture", "--goal", "g", "--ticket", "42")
	mustContain(t, res.Stdout, "Linked ticket #42 — Fix login")

	logOut := r.Run("log")
	mustContain(t, logOut.Stdout, "[#42]")
}

func TestWhyAcrossBranches(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "1\n", "c1")
	r.Run("capture", "--goal", "fix timeout on main")

	r.Git("checkout", "-b", "feature")
	r.CommitFile("b.txt", "2\n", "c2")
	r.Run("capture", "--goal", "add retry on feature", "--decision", "retry with backoff")

	why := r.Run("why", "retry")
	mustContain(t, why.Stdout, "add retry on feature")
	mustContain(t, why.Stdout, "Decision: retry with backoff")

	// Notes span branches: searching from feature still finds the main packet.
	why2 := r.Run("why", "timeout")
	mustContain(t, why2.Stdout, "fix timeout on main")
}

func TestBlameAgentLine(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("x.go", "line1\n", "init")

	// Mark x.go as agent-written this session, then extend it and commit.
	r.Run("track", filepath.Join(r.Dir, "x.go"))
	r.CommitFile("x.go", "line1\nline2\nline3\n", "extend")
	r.Run("capture", "--goal", "extend x", "--decision", "add helper lines")

	blame := r.Run("blame", "x.go:2")
	mustContain(t, blame.Stdout, "agent-written")
	mustContain(t, blame.Stdout, "Goal: extend x")
	mustContain(t, blame.Stdout, "add helper lines")
}

func TestStatusReportsPacketAndDirty(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "hi\n", "init")
	r.Run("capture", "--goal", "g")

	st := r.Run("status")
	mustContain(t, st.Stdout, "Branch: main")
	mustContain(t, st.Stdout, "HEAD has pk_")
	mustContain(t, st.Stdout, "working tree clean")

	if err := os.WriteFile(filepath.Join(r.Dir, "b.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st2 := r.Run("status")
	mustContain(t, st2.Stdout, "uncommitted changes present")
}

func TestDoctorAllGood(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("gh", ghAuthOK)
	r.CommitFile("a.txt", "hi\n", "init")
	r.Run("capture", "--goal", "g")

	doc := r.Run("doctor")
	if doc.ExitCode != 0 {
		t.Fatalf("expected doctor to pass, exit %d:\n%s", doc.ExitCode, doc.Stdout)
	}
	mustContain(t, doc.Stdout, "0 failing")
}

func TestDoctorFlagsOrphanedNote(t *testing.T) {
	r := testutil.NewRepo(t)
	r.InstallFakeBin("gh", ghAuthOK)
	r.CommitFile("a.txt", "hi\n", "init")
	r.Run("capture", "--goal", "g")

	// Drop the trailer from HEAD's message: the note is now orphaned (no
	// reachable commit references its id).
	r.Git("commit", "--amend", "-m", "init")

	doc := r.Run("doctor")
	if doc.ExitCode == 0 {
		t.Fatalf("expected doctor to fail on orphaned note:\n%s", doc.Stdout)
	}
	mustContain(t, doc.Stdout, "orphaned notes")
}

func TestVerifyUnsigned(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "hi\n", "init")
	r.Run("capture", "--goal", "g")

	v := r.Run("verify")
	if v.ExitCode != 0 {
		t.Fatalf("verify should pass for unsigned, exit %d:\n%s", v.ExitCode, v.Stderr)
	}
	mustContain(t, v.Stdout, "unsigned")
	mustContain(t, v.Stdout, "1 unsigned")
}

func TestReportCSV(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "hi\n", "init")
	r.Run("capture", "--goal", "auditable goal", "--decision", "did a thing")

	rep := r.Run("report", "--from", "2000-01-01", "--to", "2100-01-01", "--format", "csv")
	if rep.ExitCode != 0 {
		t.Fatalf("report failed: %s", rep.Stderr)
	}
	mustContain(t, rep.Stdout, "packet_id,ticket,goal,decision,commit,date,signed")
	mustContain(t, rep.Stdout, "auditable goal")
	mustContain(t, rep.Stdout, "did a thing")
}

func TestSyncPushAndFetch(t *testing.T) {
	r := testutil.NewRepo(t)

	// Bare remote to push notes to.
	bare := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", "--initial-branch=main", bare).CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v\n%s", err, out)
	}
	r.Git("remote", "add", "origin", bare)
	r.CommitFile("a.txt", "hi\n", "init")
	r.Run("capture", "--goal", "shared goal")
	// Push the trailer-bearing commit, then its notes — a clone needs both:
	// the trailer joins commit → packet id, the note carries the body.
	r.Git("push", "-u", "origin", "main")

	push := r.Run("sync", "--push")
	if push.ExitCode != 0 {
		t.Fatalf("sync --push failed: %s", push.Stderr)
	}
	mustContain(t, push.Stdout, "Pushed entrypoint packets")

	// Second clone sees the packet after a plain sync — no code re-pull.
	clone := t.TempDir()
	if out, err := exec.Command("git", "clone", bare, clone).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	sync := r.RunInDir(clone, "sync")
	if sync.ExitCode != 0 {
		t.Fatalf("sync failed: %s", sync.Stderr)
	}
	mustContain(t, sync.Stdout, "Synced entrypoint packets")

	logOut := r.RunInDir(clone, "log")
	mustContain(t, logOut.Stdout, "shared goal")
}

func TestForceReplacePacket(t *testing.T) {
	r := testutil.NewRepo(t)
	r.CommitFile("a.txt", "hi\n", "init")
	r.Run("capture", "--goal", "first")

	// Second capture on the same HEAD without --force is an error.
	again := r.Run("capture", "--goal", "second")
	if again.ExitCode == 0 {
		t.Fatal("expected error capturing twice on same HEAD")
	}
	mustContain(t, again.Stderr, "already has an entrypoint packet")

	forced := r.Run("capture", "--goal", "second", "--force")
	if forced.ExitCode != 0 {
		t.Fatalf("forced capture failed: %s", forced.Stderr)
	}
	resume := r.Run("resume")
	mustContain(t, resume.Stdout, "Goal: second")
}
