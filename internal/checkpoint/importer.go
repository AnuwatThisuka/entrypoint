package checkpoint

import "fmt"

// RawSession is the un-normalized handle an Importer maps into a Session. It
// is deliberately thin: identity plus a lazy blob reader. Transcripts and
// prompts are read only if an importer actually needs them, and by default it
// does not — summaries are built from lightweight metadata, keeping raw
// content by-reference in the git tree (Invariant I3).
type RawSession struct {
	// Importer names which registered importer should handle this record.
	Importer string
	// Ref is the git ref this record was read from, propagated onto Source.Ref.
	Ref string
	// Path locates the record within the ref tree (a session subdirectory),
	// used for diagnostics and as a fallback identity hint.
	Path string
	// File lazily reads a named sibling blob from this session's subtree.
	// Importers call it only for the small metadata they map; it must not be
	// used to slurp full transcripts into the index.
	File func(name string) ([]byte, error)
}

// Importer maps one source format's RawSession into the normalized Session.
// It is the only place a source-native schema (field names, file names) may
// appear — the core stays format-agnostic (Invariant I4).
type Importer interface {
	// Name is the stable importer id, also stamped onto Source.Importer.
	Name() string
	// Import maps raw into a normalized, finalized Session. Implementations
	// return a checkpoint error (e.g. ErrIncomplete) wrapped with context on
	// records they cannot map.
	Import(raw RawSession) (Session, error)
}

// Registry dispatches a RawSession to the importer named by RawSession.Importer.
// It is the single seam through which every external format enters the core.
type Registry struct {
	importers map[string]Importer
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{importers: make(map[string]Importer)}
}

// Register adds im under its Name. Registering two importers with the same
// name is a programmer error and panics at wiring time (never per-request).
func (r *Registry) Register(im Importer) {
	name := im.Name()
	if _, dup := r.importers[name]; dup {
		panic(fmt.Sprintf("checkpoint: importer %q already registered", name))
	}
	r.importers[name] = im
}

// Names returns the registered importer names (unordered).
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.importers))
	for name := range r.importers {
		out = append(out, name)
	}
	return out
}

// Import routes raw to its importer and returns the normalized Session. An
// unknown importer name is a typed error, not a panic — the ref tree is
// untrusted input.
func (r *Registry) Import(raw RawSession) (Session, error) {
	im, ok := r.importers[raw.Importer]
	if !ok {
		return Session{}, fmt.Errorf("checkpoint: no importer registered for %q: %w", raw.Importer, ErrIncomplete)
	}
	return im.Import(raw)
}
