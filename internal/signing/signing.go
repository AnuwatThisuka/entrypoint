// Package signing provides opt-in GPG signing over packet bodies (Phase 8),
// using the committer's existing `git config user.signingkey` — no new key
// management.
package signing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AnuwatThisuka/entrypoint/internal/git"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
)

// Status of a packet's signature.
type Status string

const (
	Signed       Status = "signed"
	Unsigned     Status = "unsigned"
	Tampered     Status = "tampered"
	Unverifiable Status = "unverifiable"
)

// Result reports a verification outcome.
type Result struct {
	Status Status
	Detail string
}

// CanonicalBody rebuilds a packet body in fixed field order, excluding the
// signature, as compact JSON (matching JS JSON.stringify: no HTML escaping).
func CanonicalBody(p *packet.Packet) (string, error) {
	clone := *p
	clone.Signature = ""
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(&clone); err != nil {
		return "", err
	}
	// Encode appends a trailing newline — which is exactly the body shape we
	// sign (JSON.stringify(body) + "\n").
	return b.String(), nil
}

// GetSigningKey reads user.signingkey from git config, or "" if unset.
func GetSigningKey(cwd string) string {
	key, err := git.Run(cwd, "config", "--get", "user.signingkey")
	if err != nil {
		return ""
	}
	return key
}

func gpg(cwd, input string, args ...string) (string, error) {
	cmd := exec.Command("gpg", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gpg %s failed: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

var (
	reGpgMissing = regexp.MustCompile(`(?i)not found|no such file|gpg: command not found|executable file not found`)
	reNoSecret   = regexp.MustCompile(`(?i)secret key not available|no secret key|skipped`)
	reNoPubkey   = regexp.MustCompile(`(?i)no public key|NO_PUBKEY`)
	reUsingKey   = regexp.MustCompile(`using (?:\w+ )?key ([0-9A-Fa-f]{8,40})`)
)

// Sign detached-signs the canonical packet body with the configured key,
// returning an ASCII-armored signature for packet.Signature.
func Sign(cwd string, p *packet.Packet) (string, error) {
	key := GetSigningKey(cwd)
	if key == "" {
		return "", fmt.Errorf(
			"No GPG signing key configured. Set `git config user.signingkey` " +
				"to a key id, or omit --sign.")
	}
	body, err := CanonicalBody(p)
	if err != nil {
		return "", err
	}
	sig, err := gpg(cwd, body,
		"--batch", "--yes", "--detach-sign", "--armor", "-u", key, "--output", "-")
	if err != nil {
		msg := err.Error()
		switch {
		case reGpgMissing.MatchString(msg):
			return "", fmt.Errorf(
				"gpg is not installed or not on PATH — needed for --sign. " +
					"Install GnuPG, or omit --sign.")
		case reNoSecret.MatchString(msg):
			return "", fmt.Errorf(
				"GPG key %q is not available in your keyring. "+
					"Check `gpg --list-secret-keys`, or omit --sign.", key)
		}
		return "", fmt.Errorf("Could not sign packet: %s", msg)
	}
	return sig, nil
}

// Verify checks a packet's signature against its current body. An unsigned
// packet is reported as such, not an error.
func Verify(cwd string, p *packet.Packet) (Result, error) {
	if p.Signature == "" {
		return Result{Status: Unsigned}, nil
	}
	body, err := CanonicalBody(p)
	if err != nil {
		return Result{}, err
	}
	dir, err := os.MkdirTemp("", "entrypoint-verify-")
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	bodyPath := filepath.Join(dir, "body.json")
	sigPath := filepath.Join(dir, "body.json.asc")
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(sigPath, []byte(p.Signature), 0o644); err != nil {
		return Result{}, err
	}

	if _, err := gpg(cwd, "", "--batch", "--verify", sigPath, bodyPath); err != nil {
		msg := err.Error()
		if reGpgMissing.MatchString(msg) {
			return Result{}, fmt.Errorf("gpg is not installed or not on PATH — needed for verify.")
		}
		// A missing public key means we can't check the signature at all —
		// that is not evidence of tampering.
		if reNoPubkey.MatchString(msg) {
			detail := "signer's public key is not in your keyring"
			if m := reUsingKey.FindStringSubmatch(msg); m != nil {
				detail = fmt.Sprintf("signer's key %s is not in your keyring", m[1])
			}
			return Result{Status: Unverifiable, Detail: detail}, nil
		}
		return Result{Status: Tampered, Detail: "signature does not match current note content"}, nil
	}
	return Result{Status: Signed}, nil
}
