// Package importer wires the concrete importers into a checkpoint.Registry.
// It lives outside internal/checkpoint so the core never imports an adapter
// (Invariant I4): the dependency arrow is checkpoint <- entire/entrypoint <-
// this package <- cmd.
package importer

import (
	"github.com/AnuwatThisuka/entrypoint/internal/checkpoint"
	"github.com/AnuwatThisuka/entrypoint/internal/importer/entire"
	"github.com/AnuwatThisuka/entrypoint/internal/importer/entrypoint"
)

// Default returns a Registry with every built-in importer registered:
// entrypoint's native packets and Entire's checkpoint export.
func Default() *checkpoint.Registry {
	r := checkpoint.NewRegistry()
	r.Register(entrypoint.New())
	r.Register(entire.New())
	return r
}
