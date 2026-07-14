// Package testutil provides an isolated scratch git repo for end-to-end
// tests. It never touches the entrypoint repo itself — every test gets a
// fresh temp repo with the compiled binary and fake bins on its PATH.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// ensureBinary compiles the entrypoint binary once per test run.
func ensureBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		moduleRoot := filepath.Join(filepath.Dir(file), "..", "..")
		dir, err := os.MkdirTemp("", "entrypoint-bin-")
		if err != nil {
			buildErr = err
			return
		}
		bin := filepath.Join(dir, "entrypoint")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/entrypoint")
		cmd.Dir = moduleRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{string(out)}
			return
		}
		binPath = bin
	})
	if buildErr != nil {
		t.Fatalf("build entrypoint binary: %v", buildErr)
	}
	return binPath
}

type buildError struct{ out string }

func (e *buildError) Error() string { return "go build failed: " + e.out }

// Result is the outcome of a CLI invocation.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Repo is a scratch git repository.
type Repo struct {
	t      *testing.T
	Dir    string
	BinDir string
	bin    string
	env    map[string]string
}

// NewRepo creates an isolated git repo with a stub `gh` (exit 1) on PATH.
func NewRepo(t *testing.T) *Repo {
	t.Helper()
	bin := ensureBinary(t)
	dir := t.TempDir()
	binDir := filepath.Join(dir, ".test-bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	r := &Repo{t: t, Dir: dir, BinDir: binDir, bin: bin, env: map[string]string{}}

	// Stub gh so no test reaches the network or the real GitHub CLI.
	r.InstallFakeBin("gh", "#!/bin/sh\nexit 1\n")

	r.Git("init", "--initial-branch=main")
	r.Git("config", "user.name", "Entrypoint Test")
	r.Git("config", "user.email", "test@entrypoint.invalid")
	r.Git("config", "commit.gpgsign", "false")
	// Keep .test-bin out of `git status` so dirty/clean assertions see truth.
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte(".test-bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return r
}

// Git runs a git command in the repo, failing the test on error.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// CommitFile writes a file and commits it, returning the new commit sha.
func (r *Repo) CommitFile(file, content, message string) string {
	r.t.Helper()
	path := filepath.Join(r.Dir, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.Git("add", file)
	r.Git("commit", "-m", message)
	return r.Git("rev-parse", "HEAD")
}

// SetEnv adds env vars passed to Run (e.g. GNUPGHOME for isolated signing).
func (r *Repo) SetEnv(k, v string) {
	r.env[k] = v
}

// Run invokes the entrypoint CLI inside the repo.
func (r *Repo) Run(args ...string) Result {
	r.t.Helper()
	cmd := exec.Command(r.bin, args...)
	cmd.Dir = r.Dir

	env := os.Environ()
	env = append(env, "PATH="+r.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for k, v := range r.env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			r.t.Fatalf("run entrypoint %s: %v", strings.Join(args, " "), err)
		}
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
}

// RunInDir invokes the CLI in an arbitrary directory (e.g. a second clone),
// sharing this repo's PATH and env.
func (r *Repo) RunInDir(dir string, args ...string) Result {
	r.t.Helper()
	cmd := exec.Command(r.bin, args...)
	cmd.Dir = dir

	env := os.Environ()
	env = append(env, "PATH="+r.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for k, v := range r.env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			r.t.Fatalf("run entrypoint %s: %v", strings.Join(args, " "), err)
		}
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
}

// RunStdin invokes the CLI feeding input on stdin (for the hook commands).
func (r *Repo) RunStdin(input string, args ...string) Result {
	r.t.Helper()
	cmd := exec.Command(r.bin, args...)
	cmd.Dir = r.Dir
	cmd.Stdin = strings.NewReader(input)

	env := os.Environ()
	env = append(env, "PATH="+r.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for k, v := range r.env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			r.t.Fatalf("run entrypoint %s: %v", strings.Join(args, " "), err)
		}
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
}

// InstallFakeBin writes an executable onto the repo's PATH so external
// tools (gh, gpg) can be faked without network or auth.
func (r *Repo) InstallFakeBin(name, script string) {
	r.t.Helper()
	path := filepath.Join(r.BinDir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		r.t.Fatal(err)
	}
}
