// Package sessionfiles maintains the agent file-write log (Phase 5): which
// files the agent wrote to this session, at file granularity, fed by the
// Claude Code PostToolUse hook. It lives under .git/ so it is never
// committed, and is cleared when a capture closes the session.
package sessionfiles

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
)

func logPath(cwd string) (string, error) {
	gitDir, err := git.Run(cwd, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, "entrypoint", "agent-files"), nil
}

func repoRoot(cwd string) (string, error) {
	return git.Run(cwd, "rev-parse", "--show-toplevel")
}

// ToRepoRelative normalizes a user/hook-supplied path to repo-root-relative
// form, resolving symlinks so git's physical root and the path compare equal.
func ToRepoRelative(cwd, file string) (string, error) {
	root, err := repoRoot(cwd)
	if err != nil {
		return "", err
	}
	base := cwd
	if base == "" {
		base, _ = os.Getwd()
	}
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(base, file)
	}
	physical := physicalPath(abs)
	rootPhysical := physicalPath(root)
	rel, err := filepath.Rel(rootPhysical, physical)
	if err != nil {
		return "", err
	}
	return rel, nil
}

// physicalPath resolves symlinks (e.g. macOS /var -> /private/var). The
// file may not exist yet, so resolve the nearest existing ancestor and
// reattach the remainder.
func physicalPath(abs string) string {
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs
	}
	return filepath.Join(physicalPath(parent), filepath.Base(abs))
}

// Add records files as agent-touched for the current session (deduplicated).
func Add(cwd string, files []string) error {
	existing, err := Read(cwd)
	if err != nil {
		return err
	}
	for _, f := range files {
		rel, err := ToRepoRelative(cwd, f)
		if err != nil {
			return err
		}
		existing[rel] = true
	}
	target, err := logPath(cwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(existing))
	for k := range existing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return os.WriteFile(target, []byte(strings.Join(keys, "\n")+"\n"), 0o644)
}

// Read returns the repo-relative paths the agent wrote to this session.
func Read(cwd string) (map[string]bool, error) {
	set := map[string]bool{}
	target, err := logPath(cwd)
	if err != nil {
		return set, nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return set, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	return set, nil
}

// Clear removes the log — called after a capture closes out the session.
func Clear(cwd string) error {
	target, err := logPath(cwd)
	if err != nil {
		return nil
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
