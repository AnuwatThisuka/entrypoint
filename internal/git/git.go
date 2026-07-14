// Package git is a thin wrapper over the git binary. We shell out (rather
// than use a Go git library) because notes/trailers are edge-case git
// operations where matching exactly what the user would see running the
// same command themselves keeps behavior predictable.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// NotesRef is the ref all entrypoint packet notes live under.
const NotesRef = "refs/notes/entrypoint"

// Error is a failed git subprocess, carrying stderr for plain-language
// wrapping upstream (see cmd root error handling).
type Error struct {
	Args   []string
	Stderr string
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s failed: %s", strings.Join(e.Args, " "), e.Stderr)
}

// AsError reports whether err is a *git.Error and returns it.
func AsError(err error) (*Error, bool) {
	var ge *Error
	if errors.As(err, &ge) {
		return ge, true
	}
	return nil, false
}

// run executes git in cwd, optionally feeding input on stdin, and returns
// trimmed stdout. A non-zero exit becomes an *Error carrying stderr.
func run(cwd string, input string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", &Error{Args: args, Stderr: msg}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Run is the exported form of run for callers outside this package that
// need a raw git invocation (e.g. push/fetch/remote).
func Run(cwd string, args ...string) (string, error) {
	return run(cwd, "", args...)
}

// exitCode runs git and returns its process exit code (no error for a
// clean non-zero exit) — used where the exit status itself is the answer.
func exitCode(cwd string, args ...string) (int, error) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func IsRepo(cwd string) bool {
	_, err := run(cwd, "", "rev-parse", "--is-inside-work-tree")
	return err == nil
}

func HeadSha(cwd string) (string, error) {
	return run(cwd, "", "rev-parse", "--verify", "HEAD")
}

func HasCommits(cwd string) bool {
	_, err := HeadSha(cwd)
	return err == nil
}

// CurrentBranch is the branch name, or "HEAD" when detached.
func CurrentBranch(cwd string) (string, error) {
	return run(cwd, "", "rev-parse", "--abbrev-ref", "HEAD")
}

// HeadMessage is the full commit message (subject + body) of HEAD.
func HeadMessage(cwd string) (string, error) {
	return run(cwd, "", "log", "-1", "--format=%B")
}

// AmendHeadMessage rewrites HEAD's message without touching the tree and
// returns the new HEAD sha. --allow-empty because a message-only amend
// never changes the tree and HEAD may be an empty checkpoint commit.
func AmendHeadMessage(cwd, message string) (string, error) {
	if _, err := run(cwd, message,
		"commit", "--amend", "--allow-empty", "--no-verify", "-F", "-"); err != nil {
		return "", err
	}
	return HeadSha(cwd)
}

// HeadChangedFiles lists files changed by the HEAD commit.
func HeadChangedFiles(cwd string) ([]string, error) {
	out, err := run(cwd, "",
		"diff-tree", "--no-commit-id", "--name-only", "-r", "--root", "HEAD")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// WorkingTreeDirty reports whether the working tree or index has changes.
func WorkingTreeDirty(cwd string) (bool, error) {
	out, err := run(cwd, "", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// Upstream is the tracked upstream of the current branch.
type Upstream struct {
	Ref    string // e.g. "origin/main"
	Remote string // e.g. "origin"
	Branch string // e.g. "main"
}

// UpstreamRef returns the tracked upstream, or nil when none is set.
func UpstreamRef(cwd string) (*Upstream, error) {
	ref, err := run(cwd, "",
		"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return nil, nil // no upstream configured
	}
	if i := strings.IndexByte(ref, '/'); i >= 0 {
		return &Upstream{Ref: ref, Remote: ref[:i], Branch: ref[i+1:]}, nil
	}
	return &Upstream{Ref: ref, Remote: ref, Branch: ""}, nil
}

// IsAncestor reports whether ancestor is an ancestor of (or equal to)
// descendant. merge-base --is-ancestor exits 0=yes / 1=no / 128=bad ref.
func IsAncestor(cwd, ancestor, descendant string) (bool, error) {
	code, err := exitCode(cwd, "merge-base", "--is-ancestor", ancestor, descendant)
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

// LocalNotesRefSha is the local notes-ref object sha, or "" when absent.
func LocalNotesRefSha(cwd string) (string, error) {
	out, err := run(cwd, "", "rev-parse", "--verify", "--quiet", NotesRef)
	if err != nil {
		return "", nil
	}
	return out, nil
}

// RemoteNotesRefSha reports the notes-ref sha on a remote. The bool is
// false when the remote can't be reached; the string is "" when the
// remote simply doesn't carry the ref.
func RemoteNotesRefSha(cwd, remote string) (sha string, reachable bool) {
	out, err := run(cwd, "", "ls-remote", remote, NotesRef)
	if err != nil {
		return "", false // unreachable / not configured
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", true
	}
	return fields[0], true
}

// NotesAdd attaches (or overwrites) an entrypoint note on a commit.
func NotesAdd(cwd, sha, content string) error {
	_, err := run(cwd, content,
		"notes", "--ref="+NotesRef, "add", "-f", "-F", "-", sha)
	return err
}

// NotesShow reads the note on a commit; ok is false when there is none.
func NotesShow(cwd, sha string) (content string, ok bool) {
	out, err := run(cwd, "", "notes", "--ref="+NotesRef, "show", sha)
	if err != nil {
		return "", false
	}
	return out, true
}

// Note is a [noteObjectSha, annotatedCommitSha] pair.
type Note struct {
	NoteSha   string
	CommitSha string
}

// NotesList returns every entrypoint note; empty when the ref is absent.
func NotesList(cwd string) []Note {
	out, err := run(cwd, "", "notes", "--ref="+NotesRef, "list")
	if err != nil {
		return nil // notes ref doesn't exist yet
	}
	var notes []Note
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		n := Note{}
		if len(fields) > 0 {
			n.NoteSha = fields[0]
		}
		if len(fields) > 1 {
			n.CommitSha = fields[1]
		}
		notes = append(notes, n)
	}
	return notes
}

// CommitEntry is a commit sha with its full message.
type CommitEntry struct {
	Sha     string
	Message string
}

// LogCommits returns commits reachable from revs (default HEAD), newest
// first, with full messages. %x1f separates sha from message, %x1e
// separates commits — control chars that can't appear in shas and are
// vanishingly unlikely in messages.
func LogCommits(cwd string, revs ...string) ([]CommitEntry, error) {
	if len(revs) == 0 {
		revs = []string{"HEAD"}
	}
	args := append([]string{"log", "--format=%H%x1f%B%x1e"}, revs...)
	out, err := run(cwd, "", args...)
	if err != nil {
		return nil, err
	}
	var entries []CommitEntry
	for _, chunk := range strings.Split(out, "\x1e") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		sep := strings.IndexByte(chunk, '\x1f')
		if sep < 0 {
			continue
		}
		entries = append(entries, CommitEntry{
			Sha:     strings.TrimSpace(chunk[:sep]),
			Message: chunk[sep+1:],
		})
	}
	return entries, nil
}

// CommitMessage is the full message of an arbitrary commit.
func CommitMessage(cwd, sha string) (string, error) {
	return run(cwd, "", "log", "-1", "--format=%B", sha)
}

var zeroSha = regexp.MustCompile(`^0+$`)

// BlameLineCommit resolves a file:line in the working tree to the commit
// that introduced it. Returns "" for lines not committed yet (all-zero sha).
func BlameLineCommit(cwd, file string, line int) (string, error) {
	out, err := run(cwd, "",
		"blame", "-L", fmt.Sprintf("%d,%d", line, line), "--porcelain", "--", file)
	if err != nil {
		return "", err
	}
	first := splitLines(out)
	if len(first) == 0 {
		return "", nil
	}
	sha := strings.SplitN(first[0], " ", 2)[0]
	if sha == "" || zeroSha.MatchString(sha) {
		return "", nil
	}
	return sha, nil
}

// InterpretTrailersAdd pipes a message through git interpret-trailers to
// append a trailer, so placement/blank-line rules match git's own.
func InterpretTrailersAdd(cwd, message, trailer string) (string, error) {
	return run(cwd, message,
		"interpret-trailers", "--if-exists", "add", "--trailer", trailer)
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
