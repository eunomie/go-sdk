package templates

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codegen/generator"
	"codegen/introspection"
)

// The module FuncMap must expose the module-only helpers the module templates
// call, which the client FuncMap omits.
func TestGoTemplateFuncsForModuleHasModuleEntries(t *testing.T) {
	schema := &introspection.Schema{}
	fm := GoTemplateFuncsForModule(context.Background(), schema, schema, "v0.12.0", generator.Config{}, nil, nil, 1)
	for _, name := range []string{"IsPartial", "ModuleMainSrc", "IsModuleCode"} {
		require.Contains(t, fm, name, "module FuncMap must expose %s", name)
	}
}

// The client FuncMap must not gain module-only entries: keeping the client
// path unchanged is the point of splitting the constructors.
func TestGoTemplateFuncsClientHasNoModuleMain(t *testing.T) {
	schema := &introspection.Schema{}
	fm := GoTemplateFuncs(schema, schema, "v0.12.0", generator.Config{})
	require.Contains(t, fm, "BoundModule")
	require.NotContains(t, fm, "ModuleMainSrc", "client FuncMap must not gain the module dispatch helper")
}

// isPartial keys off the pass index: pass 0 is the bootstrap pass.
func TestIsPartialTracksPass(t *testing.T) {
	schema := &introspection.Schema{}
	partial := GoTemplateFuncsForModule(context.Background(), schema, schema, "v0.12.0", generator.Config{}, nil, nil, 0)
	require.True(t, partial["IsPartial"].(func() bool)())
	full := GoTemplateFuncsForModule(context.Background(), schema, schema, "v0.12.0", generator.Config{}, nil, nil, 1)
	require.False(t, full["IsPartial"].(func() bool)())
}
