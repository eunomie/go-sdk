package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const validEntrypoint = `type Entrypoint implements ModuleEntrypoint {
  pub types(workspace: Workspace!): [TypeDef!]! { [] }
  pub call(
    workspace: Workspace!,
    receiverType: String!,
    receiverValue: JSON,
    fnName: String!,
    fnArgs: JSON!,
  ): JSON! {
    ("null" :: JSON!)
  }
}
`

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.dang"), []byte(validEntrypoint), 0o600))

	err := Check(context.Background(), dir, filepath.Join("..", "..", "cmd", "dagger-go-sdk-codegen", "introspection", "testdata", "schema.json"))
	require.NoError(t, err)
}

// Released engines pass a workspace to types and call, so an entrypoint with
// sources declares it nullable on both: the entrypoint still conforms, and
// types refuses to run without the files outside the module.
func TestCheckAcceptsSourcesEntrypoint(t *testing.T) {
	dir := t.TempDir()
	sources := `type Entrypoint implements ModuleEntrypoint {
  pub sources(workspace: Workspace!): Directory! { directory }
  pub types(workspace: Workspace): [TypeDef!]! {
    if (workspace != null) {
      raise "needs a newer engine"
    }
    []
  }
  pub call(
    workspace: Workspace,
    receiverType: String!,
    receiverValue: JSON,
    fnName: String!,
    fnArgs: JSON!,
    callDigest: String,
  ): JSON! {
    ("null" :: JSON!)
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.dang"), []byte(sources), 0o600))

	err := Check(context.Background(), dir, filepath.Join("..", "..", "cmd", "dagger-go-sdk-codegen", "introspection", "testdata", "schema.json"))
	require.NoError(t, err)
}

func TestCheckRejectsInterfaceMismatch(t *testing.T) {
	dir := t.TempDir()
	invalid := `type Entrypoint implements ModuleEntrypoint {
  pub types(workspace: Workspace!): [TypeDef!]! { [] }
  pub call(workspace: Workspace!): JSON! { ("null" :: JSON!) }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.dang"), []byte(invalid), 0o600))

	err := Check(context.Background(), dir, filepath.Join("..", "..", "cmd", "dagger-go-sdk-codegen", "introspection", "testdata", "schema.json"))
	require.ErrorContains(t, err, "does not satisfy ModuleEntrypoint")
}

func TestCheckRejectsArgumentList(t *testing.T) {
	dir := t.TempDir()
	invalid := `type Entrypoint implements ModuleEntrypoint {
  pub types(workspace: Workspace!): [TypeDef!]! { [] }
  pub call(
    workspace: Workspace!,
    receiverType: String!,
    receiverValue: JSON,
    fnName: String!,
    fnArgs: [FunctionCallArgValue!]!,
  ): JSON! {
    ("null" :: JSON!)
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.dang"), []byte(invalid), 0o600))

	err := Check(context.Background(), dir, filepath.Join("..", "..", "cmd", "dagger-go-sdk-codegen", "introspection", "testdata", "schema.json"))
	require.ErrorContains(t, err, "does not satisfy ModuleEntrypoint")
}
