package generator

import (
	"context"
	"io/fs"
	"os/exec"

	"codegen/introspection"
)

type Generator interface {
	// GenerateClient runs codegen in a context of a standalone client and
	// returns an overlay filesystem with the generated files.
	GenerateClient(ctx context.Context, schema *introspection.Schema, schemaVersion string) (*GeneratedState, error)
}

type GeneratedState struct {
	// Overlay is the overlay filesystem that contains generated code to write
	// over the output directory.
	Overlay fs.FS

	// PostCommands are commands that need to be run after the codegen has
	// finished. Module generation uses this to run `go mod tidy`, `go get` and
	// `go work use` against the generated go.mod.
	PostCommands []*exec.Cmd

	// NeedRegenerate is set when the generated files are not final and another
	// codegen pass is required (e.g. after bootstrapping go.mod / a base
	// dagger.gen.go / a starter main.go for a fresh module).
	NeedRegenerate bool
}
