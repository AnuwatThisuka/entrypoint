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
	// SignerMismatch: the signature is cryptographically valid, but the
	// signing key's identity does not match the commit's committer — the
	// body wasn't altered, yet it was signed by someone other than the
	// person who committed it. A forger who re-signs a rewritten body with
	// their own key lands here, not in Signed.
	SignerMismatch Status = "signer-mismatch"
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
	// GOODSIG line from --status-fd: "[GNUPG:] GOODSIG <keyid> <user id>".
	// The user id is the key's primary UID, typically "Name <email>".
	reGoodSig = regexp.MustCompile(`\[GNUPG:\] GOODSIG [0-9A-Fa-f]+ (.+)`)
	reUIDMail = regexp.MustCompile(`<([^>]+)>`)
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

// gpgVerify runs a detached-signature verification, returning the machine
// status (--status-fd) and human stderr separately. A cryptographically
// good signature exits 0; a bad/absent-key one exits non-zero (err set).
func gpgVerify(cwd, sigPath, bodyPath string) (status, stderrOut string, err error) {
	cmd := exec.Command("gpg", "--status-fd=1", "--batch", "--verify", sigPath, bodyPath)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

// committerEmail is the lowercased committer email of a commit, or "" when
// it can't be resolved. Used to bind a signature to who actually committed.
func committerEmail(cwd, sha string) string {
	if sha == "" {
		return ""
	}
	out, err := git.Run(cwd, "show", "-s", "--format=%ce", sha)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(out))
}

// signerIdentity extracts the signing key's primary UID and its email from
// a GOODSIG status line, both lowercased for comparison ("" when absent).
func signerIdentity(status string) (uid, email string) {
	m := reGoodSig.FindStringSubmatch(status)
	if m == nil {
		return "", ""
	}
	uid = strings.TrimSpace(m[1])
	if e := reUIDMail.FindStringSubmatch(uid); e != nil {
		email = strings.ToLower(e[1])
	}
	return uid, email
}

// Verify checks a packet's signature against its current body AND binds the
// signing key's identity to the commit's committer. commitSha is the commit
// the packet is attached to; pass "" to skip the identity binding (still a
// full crypto check). An unsigned packet is reported as such, not an error.
//
// A valid signature alone proves only that the body wasn't altered after
// signing — not who signed it. Without the committer binding, anyone whose
// public key is in the verifier's keyring could rewrite a packet and re-sign
// it with their own key, and it would read as "signed". The binding closes
// that gap: a valid signature by a key whose identity is not the committer
// is reported as SignerMismatch, not Signed.
func Verify(cwd, commitSha string, p *packet.Packet) (Result, error) {
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
	if err := os.WriteFile(bodyPath, []byte(body), 0o600); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(sigPath, []byte(p.Signature), 0o600); err != nil {
		return Result{}, err
	}

	status, stderr, err := gpgVerify(cwd, sigPath, bodyPath)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		if reGpgMissing.MatchString(msg) {
			return Result{}, fmt.Errorf("gpg is not installed or not on PATH — needed for verify.")
		}
		// A missing public key means we can't check the signature at all —
		// that is not evidence of tampering.
		if reNoPubkey.MatchString(msg) || reNoPubkey.MatchString(status) {
			detail := "signer's public key is not in your keyring"
			if m := reUsingKey.FindStringSubmatch(msg); m != nil {
				detail = fmt.Sprintf("signer's key %s is not in your keyring", m[1])
			}
			return Result{Status: Unverifiable, Detail: detail}, nil
		}
		return Result{Status: Tampered, Detail: "signature does not match current note content"}, nil
	}

	// Signature is cryptographically valid. Bind the signer to the committer.
	uid, signerEmail := signerIdentity(status)
	committer := committerEmail(cwd, commitSha)

	// When either identity can't be determined we can't prove a mismatch;
	// report signed but name the signer so a human can still judge.
	if signerEmail == "" || committer == "" {
		if uid != "" {
			return Result{Status: Signed, Detail: "signed by " + uid}, nil
		}
		return Result{Status: Signed}, nil
	}
	if signerEmail != committer {
		return Result{
			Status: SignerMismatch,
			Detail: fmt.Sprintf("signed by %s, but %s made the commit", signerEmail, committer),
		}, nil
	}
	return Result{Status: Signed, Detail: "signed by " + uid}, nil
}
