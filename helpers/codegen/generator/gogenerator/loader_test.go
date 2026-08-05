package gogenerator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadPackage type-checks a real on-disk Go module with the local toolchain,
// no engine involved.
func TestLoadPackageReadsPackageName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/mod\n\ngo 1.23\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\ntype Mod struct{}\n\nfunc (Mod) Hello() string { return \"hi\" }\n"), 0o644))

	pkg, fset, err := loadPackage(context.Background(), dir, false)
	require.NoError(t, err)
	require.NotNil(t, fset)
	require.Equal(t, "main", pkg.Name)
	require.NotNil(t, pkg.Types.Scope().Lookup("Mod"), "the module type should be type-checked and visible")
}

// An empty dir inside a module has no package name; allowEmpty gates whether
// that is an error (module bootstrap needs allowEmpty=true).
func TestLoadPackageEmptyDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/mod\n\ngo 1.23\n"), 0o644))

	_, _, err := loadPackage(context.Background(), dir, false)
	require.Error(t, err, "an empty package must be rejected when allowEmpty is false")

	pkg, _, err := loadPackage(context.Background(), dir, true)
	require.NoError(t, err, "an empty package is allowed when allowEmpty is true")
	require.Empty(t, pkg.Name)
}
