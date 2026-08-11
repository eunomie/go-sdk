package gogenerator

import (
	"io/fs"
	"testing"

	"github.com/psanford/memfs"
	"github.com/stretchr/testify/require"

	"codegen/generator"
)

// GenerateModuleTypes must refuse a module whose own name collides with an
// installed dependency: the caller merges the emitted types keyed on the module
// name, so a same-named dependency would make that merge a silent no-op and the
// module's own types would never enter the schema.
func TestGenerateModuleTypesRejectsDepUnderModuleName(t *testing.T) {
	// buildClientSchema contributes a dependency named "hello".
	g := &GoGenerator{Config: generator.Config{
		OutputDir: t.TempDir(),
		ModuleConfig: &generator.ModuleGeneratorConfig{
			ModuleName:       "hello",
			ModuleSourcePath: ".",
		},
	}}
	_, _, err := g.GenerateModuleTypes(t.Context(), buildClientSchema(), "v0.21.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "installed under the module's own name")
}

// A generated module's per-dependency binding files live under internal/dagger
// (package dagger). The module root is package main, so writing them at the
// root would put two packages in one directory and fail to build.
func TestModuleDepFilesUnderInternalDagger(t *testing.T) {
	schema := buildClientSchema()
	generator.SetSchema(schema)

	mfs := memfs.New()
	cfg := generator.Config{
		OutputDir: t.TempDir(),
		ModuleConfig: &generator.ModuleGeneratorConfig{
			ModuleName:       "app",
			ModuleSourcePath: ".",
		},
	}
	err := generateDependencyFiles(t.Context(), cfg, schema, "v0.21.0", mfs,
		&PackageInfo{PackageName: "main", PackageImport: "dagger/app"}, nil, []string{"hello"})
	require.NoError(t, err)

	_, err = fs.Stat(mfs, "internal/dagger/hello.gen.go")
	require.NoError(t, err, "module dep file must be under internal/dagger")
	_, err = fs.Stat(mfs, "hello.gen.go")
	require.Error(t, err, "module dep file must not be written at the module root")
}

// A standalone client's root package is "dagger", so its per-dependency files
// stay at the client root.
func TestClientDepFilesAtRoot(t *testing.T) {
	schema := buildClientSchema()
	generator.SetSchema(schema)

	mfs := memfs.New()
	cfg := generator.Config{
		OutputDir: t.TempDir(),
		ClientConfig: &generator.ClientGeneratorConfig{
			ModuleName: "app",
		},
	}
	err := generateDependencyFiles(t.Context(), cfg, schema, "v0.21.0", mfs,
		&PackageInfo{PackageName: "dagger", PackageImport: "app"}, nil, []string{"hello"})
	require.NoError(t, err)

	_, err = fs.Stat(mfs, "hello.gen.go")
	require.NoError(t, err, "client dep file must be at the root")
}
