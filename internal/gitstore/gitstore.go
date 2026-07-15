// Package gitstore makes git the authoritative store for checkpoint records
// (Invariant I1). It reads session records out of a single ref's tree and
// writes entrypoint's own records back into a dedicated ref — all with pure
// go-git, never shelling out and never touching the working tree or code
// branches (I5 and the Phase B guardrails).
//
// go-git version note: the plan names go-git v6, but v6 has no stable release
// (alpha only). We depend on the stable v5 line instead — equally pure-Go /
// no-CGO, so I5 holds — deviating from the plan's stated version deliberately.
package gitstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"sort"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/AnuwatThisuka/entrypoint/internal/checkpoint"
	"github.com/AnuwatThisuka/entrypoint/internal/packet"
	"github.com/AnuwatThisuka/entrypoint/internal/trailer"
)

// PacketsRef is the ref entrypoint writes its own normalized packets to. It is
// a records ref, never a code branch — walking or writing it never disturbs
// the user's code.
const PacketsRef = "refs/entrypoint/packets/v1"

// sessionFile is the blob name a native packet is stored under inside its
// session subtree. It mirrors the entrypoint importer's expected file name.
const sessionFile = "packet.json"

// RefSpec names a ref and the importer that maps its session subtrees.
type RefSpec struct {
	Ref      string // e.g. "refs/entire/checkpoints/v1" or PacketsRef
	Importer string // e.g. "entire" or "entrypoint"
}

// RefWalker reads and writes checkpoint records against a single repository's
// object store. It is safe to reuse across refs.
type RefWalker struct {
	repo *gogit.Repository
}

// Open opens the repository at repoPath for record read/write. It performs no
// checkout and leaves the working tree untouched.
func Open(repoPath string) (*RefWalker, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("gitstore: open %q: %w", repoPath, err)
	}
	return &RefWalker{repo: repo}, nil
}

// Fetch pulls exactly one ref from remote (a URL or path) into the local
// object store under the same ref name. Only the single refspec is fetched, so
// code branches and the working tree are never touched (Phase B guardrail).
func (w *RefWalker) Fetch(ctx context.Context, remote, ref string) error {
	rem := gogit.NewRemote(w.repo.Storer, &config.RemoteConfig{
		Name: "gitstore-fetch",
		URLs: []string{remote},
	})
	spec := config.RefSpec(fmt.Sprintf("+%s:%s", ref, ref))
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("gitstore: bad ref %q: %w", ref, err)
	}
	err := rem.FetchContext(ctx, &gogit.FetchOptions{
		RefSpecs: []config.RefSpec{spec},
		Tags:     gogit.NoTags,
	})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("gitstore: fetch %s from %s: %w", ref, remote, err)
	}
	return nil
}

// Walk yields one RawSession per top-level session subdirectory under spec.Ref.
// RawSession.File lazily reads sibling blobs from that subtree, so raw
// transcripts are never read unless an importer explicitly asks for them (I3).
func (w *RefWalker) Walk(ctx context.Context, spec RefSpec) iter.Seq2[checkpoint.RawSession, error] {
	return func(yield func(checkpoint.RawSession, error) bool) {
		tree, err := w.resolveTree(spec.Ref)
		if err != nil {
			yield(checkpoint.RawSession{}, err)
			return
		}
		for _, e := range tree.Entries {
			if e.Mode != filemode.Dir {
				continue // sessions are subdirectories; skip stray blobs
			}
			if ctx.Err() != nil {
				yield(checkpoint.RawSession{}, ctx.Err())
				return
			}
			subtree, err := w.repo.TreeObject(e.Hash)
			if err != nil {
				if !yield(checkpoint.RawSession{}, fmt.Errorf("gitstore: read subtree %q: %w", e.Name, err)) {
					return
				}
				continue
			}
			raw := checkpoint.RawSession{
				Importer: spec.Importer,
				Ref:      spec.Ref,
				Path:     e.Name,
				File:     fileReader(subtree),
			}
			if !yield(raw, nil) {
				return
			}
		}
	}
}

// fileReader returns a lazy blob reader bound to one session subtree. Each call
// reads exactly the named blob and nothing else.
func fileReader(subtree *object.Tree) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		f, err := subtree.File(name)
		if err != nil {
			return nil, fmt.Errorf("gitstore: blob %q: %w", name, err)
		}
		r, err := f.Reader()
		if err != nil {
			return nil, err
		}
		defer func() { _ = r.Close() }()
		return io.ReadAll(r)
	}
}

// CommitLinks scans commit messages reachable from each rev (read-only log,
// newest first) and maps each referenced native record id to the code commit
// that produced it. It recognizes entrypoint and Entire linkage trailers. The
// newest referencing commit wins. With no revs it scans HEAD.
func (w *RefWalker) CommitLinks(ctx context.Context, revs ...string) (map[string]string, error) {
	if len(revs) == 0 {
		revs = []string{"HEAD"}
	}
	links := make(map[string]string)
	for _, rev := range revs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		h, err := w.repo.ResolveRevision(plumbing.Revision(rev))
		if err != nil {
			return nil, fmt.Errorf("gitstore: resolve %q: %w", rev, err)
		}
		commits, err := w.repo.Log(&gogit.LogOptions{From: *h})
		if err != nil {
			return nil, fmt.Errorf("gitstore: log %q: %w", rev, err)
		}
		err = commits.ForEach(func(c *object.Commit) error {
			for _, id := range trailer.ParseLinkedIDs(c.Message) {
				if _, seen := links[id]; !seen {
					links[id] = c.Hash.String()
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("gitstore: walk log %q: %w", rev, err)
		}
	}
	return links, nil
}

// Rebuild walks spec.Ref, maps every session through reg, and overlays
// Commit.SHA from links when the mapped session does not already carry one
// (the gap FromPacket leaves). It is the shared primitive Phase C's
// rebuild-index and Phase E's ingest both build on.
func (w *RefWalker) Rebuild(ctx context.Context, spec RefSpec, reg *checkpoint.Registry, links map[string]string) ([]checkpoint.Session, error) {
	var out []checkpoint.Session
	for raw, err := range w.Walk(ctx, spec) {
		if err != nil {
			return nil, err
		}
		s, err := reg.Import(raw)
		if err != nil {
			return nil, fmt.Errorf("gitstore: import %q: %w", raw.Path, err)
		}
		if s.Commit.SHA == "" {
			if sha, ok := links[s.Source.NativeID]; ok {
				s.Commit.SHA = sha
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// WritePacket appends p to PacketsRef as a "<id>/packet.json" session subtree,
// storing the full serialized packet — INCLUDING any GPG signature — so the
// record stays verifiable from git, the source of truth. It builds new
// blob/tree/commit objects and moves the ref; it never checks out, and never
// touches code branches or the working tree. Returns the new ref commit sha.
func (w *RefWalker) WritePacket(ctx context.Context, p *packet.Packet) (string, error) {
	if p == nil || p.ID == "" {
		return "", fmt.Errorf("gitstore: refusing to write packet without id")
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	body, err := packet.Marshal(p) // full body, signature included
	if err != nil {
		return "", fmt.Errorf("gitstore: marshal packet %q: %w", p.ID, err)
	}
	blobHash, err := w.writeBlob([]byte(body))
	if err != nil {
		return "", err
	}
	subHash, err := w.writeTree([]object.TreeEntry{
		{Name: sessionFile, Mode: filemode.Regular, Hash: blobHash},
	})
	if err != nil {
		return "", err
	}

	entries, parent, err := w.parentEntries(PacketsRef)
	if err != nil {
		return "", err
	}
	entries = upsertEntry(entries, object.TreeEntry{Name: p.ID, Mode: filemode.Dir, Hash: subHash})
	treeHash, err := w.writeTree(entries)
	if err != nil {
		return "", err
	}

	when := time.Now()
	sig := object.Signature{Name: "entrypoint", Email: "entrypoint@localhost", When: when}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   fmt.Sprintf("entrypoint: packet %s\n", p.ID),
		TreeHash:  treeHash,
	}
	if !parent.IsZero() {
		commit.ParentHashes = []plumbing.Hash{parent}
	}
	commitHash, err := w.writeCommit(commit)
	if err != nil {
		return "", err
	}

	ref := plumbing.NewHashReference(plumbing.ReferenceName(PacketsRef), commitHash)
	if err := w.repo.Storer.SetReference(ref); err != nil {
		return "", fmt.Errorf("gitstore: move %s: %w", PacketsRef, err)
	}
	return commitHash.String(), nil
}

// resolveTree resolves a ref to its tree, peeling a commit or tag as needed.
func (w *RefWalker) resolveTree(ref string) (*object.Tree, error) {
	r, err := w.repo.Reference(plumbing.ReferenceName(ref), true)
	if err != nil {
		return nil, fmt.Errorf("gitstore: ref %q: %w", ref, err)
	}
	obj, err := w.repo.Object(plumbing.AnyObject, r.Hash())
	if err != nil {
		return nil, fmt.Errorf("gitstore: object for %q: %w", ref, err)
	}
	switch o := obj.(type) {
	case *object.Tree:
		return o, nil
	case *object.Commit:
		return o.Tree()
	case *object.Tag:
		target, err := o.Object()
		if err != nil {
			return nil, err
		}
		if c, ok := target.(*object.Commit); ok {
			return c.Tree()
		}
		if t, ok := target.(*object.Tree); ok {
			return t, nil
		}
		return nil, fmt.Errorf("gitstore: tag %q does not point at a tree/commit", ref)
	default:
		return nil, fmt.Errorf("gitstore: ref %q is not a tree/commit", ref)
	}
}

// parentEntries returns the tree entries of ref's current commit and that
// commit's hash. A missing ref yields no entries and a zero hash (first write).
func (w *RefWalker) parentEntries(ref string) ([]object.TreeEntry, plumbing.Hash, error) {
	r, err := w.repo.Reference(plumbing.ReferenceName(ref), true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, plumbing.ZeroHash, nil
	}
	if err != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("gitstore: ref %q: %w", ref, err)
	}
	commit, err := w.repo.CommitObject(r.Hash())
	if err != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("gitstore: commit for %q: %w", ref, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, plumbing.ZeroHash, err
	}
	return append([]object.TreeEntry(nil), tree.Entries...), r.Hash(), nil
}

func (w *RefWalker) writeBlob(data []byte) (plumbing.Hash, error) {
	obj := w.repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	writer, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return plumbing.ZeroHash, err
	}
	if err := writer.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	h, err := w.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("gitstore: store blob: %w", err)
	}
	return h, nil
}

func (w *RefWalker) writeTree(entries []object.TreeEntry) (plumbing.Hash, error) {
	// git canonical trees are sorted by name; sort for determinism and interop.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	tree := &object.Tree{Entries: entries}
	obj := w.repo.Storer.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	h, err := w.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("gitstore: store tree: %w", err)
	}
	return h, nil
}

func (w *RefWalker) writeCommit(commit *object.Commit) (plumbing.Hash, error) {
	obj := w.repo.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	h, err := w.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("gitstore: store commit: %w", err)
	}
	return h, nil
}

// upsertEntry replaces an entry with the same name, or appends it.
func upsertEntry(entries []object.TreeEntry, e object.TreeEntry) []object.TreeEntry {
	for i := range entries {
		if entries[i].Name == e.Name {
			entries[i] = e
			return entries
		}
	}
	return append(entries, e)
}
