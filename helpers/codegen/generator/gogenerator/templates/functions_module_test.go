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

// The client and module FuncMaps expose the same keys: the whole template
// tree (client + module templates) is parsed against one FuncMap, and
// text/template resolves every referenced function at parse time. Client
// safety comes from the IsModuleCode guards and a nil modulePkg, not from a
// narrower map. This test pins that the module keys stay present on the client
// FuncMap so the shared parse keeps working.
func TestClientFuncMapCarriesModuleKeysForSharedParse(t *testing.T) {
	schema := &introspection.Schema{}
	fm := GoTemplateFuncs(schema, schema, "v0.12.0", generator.Config{})
	for _, name := range []string{"BoundModule", "IsPartial", "ModuleMainSrc"} {
		require.Contains(t, fm, name, "client FuncMap must expose %s for the shared template parse", name)
	}
}

// isPartial keys off the pass index: pass 0 is the bootstrap pass.
func TestIsPartialTracksPass(t *testing.T) {
	schema := &introspection.Schema{}
	partial := GoTemplateFuncsForModule(context.Background(), schema, schema, "v0.12.0", generator.Config{}, nil, nil, 0)
	require.True(t, partial["IsPartial"].(func() bool)())
	full := GoTemplateFuncsForModule(context.Background(), schema, schema, "v0.12.0", generator.Config{}, nil, nil, 1)
	require.False(t, full["IsPartial"].(func() bool)())
}
