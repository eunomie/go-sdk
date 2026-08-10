package gogenerator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dschmidt/go-layerfs"
	"github.com/iancoleman/strcase"
	"github.com/psanford/memfs"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"

	"codegen/generator"
	"codegen/generator/gogenerator/templates"
	"codegen/introspection"
)

// GenerateModule generates a Go module's dagger.gen.go from the (already
// merged) schema. The module's own types are expected to be present in schema:
// unlike the engine's cmd/codegen, this generator does not merge them itself —
// the SDK's dang layer performs the merge via the engine's schema().merge()
// and hands the merged schema in. Otherwise it mirrors the engine flow:
//
//  1. bootstrap go.mod (and, for a fresh module, a base dagger.gen.go + starter
//     main.go), requesting another pass while anything was scaffolded;
//  2. load the module package;
//  3. generate dagger.gen.go against the module source.
func (g *GoGenerator) GenerateModule(ctx context.Context, schema *introspection.Schema, schemaVersion string) (*generator.GeneratedState, error) {
	mfs, pkgInfo, outDir, genSt, partial, err := g.bootstrapModule(ctx, schema, schemaVersion)
	if err != nil {
		return nil, err
	}
	if partial {
		genSt.NeedRegenerate = true
		return genSt, nil
	}

	pkg, fset, err := loadPackage(ctx, filepath.Join(g.Config.OutputDir, outDir), false)
	if err != nil {
		return nil, fmt.Errorf("load package %q: %w", outDir, err)
	}

	// respect existing package name
	pkgInfo.PackageName = pkg.Name

	if err := generateCode(ctx, g.Config, schema, schemaVersion, mfs, pkgInfo, &moduleGenCtx{pkg: pkg, fset: fset, pass: 1}); err != nil {
		return nil, fmt.Errorf("generate code: %w", err)
	}

	return genSt, nil
}

// GenerateModuleTypes bootstraps the module to a loadable state and, once it is
// loadable, emits the module's own types as introspection JSON. The dang layer
// merges that JSON into the dependency schema before calling GenerateModule.
//
// It shares the bootstrap loop with GenerateModule: while partial is true the
// returned state carries NeedRegenerate and the module-types JSON is nil; the
// caller writes the overlay, runs the post-commands and calls again.
func (g *GoGenerator) GenerateModuleTypes(ctx context.Context, schema *introspection.Schema, schemaVersion string) (*generator.GeneratedState, []byte, error) {
	// The caller merges the emitted types into this (dependency) schema, keyed
	// on the module name. A dependency installed under the module's own name
	// would make that merge a silent no-op — the module's types would never
	// enter the schema. Refuse it with an actionable error, as the engine does.
	if g.Config.ModuleConfig != nil {
		moduleName := g.Config.ModuleConfig.ModuleName
		if slices.Contains(schema.DependencyNames(), moduleName) {
			return nil, nil, fmt.Errorf(
				"a dependency is installed under the module's own name %q; "+
					"self-calls need distinct names — reinstall the dependency under "+
					"another name (dagger install --name)", moduleName)
		}
	}

	_, _, outDir, genSt, partial, err := g.bootstrapModule(ctx, schema, schemaVersion)
	if err != nil {
		return nil, nil, err
	}
	if partial {
		genSt.NeedRegenerate = true
		return genSt, nil, nil
	}

	pkg, fset, err := loadPackage(ctx, filepath.Join(g.Config.OutputDir, outDir), true)
	if err != nil {
		return nil, nil, fmt.Errorf("load package %q: %w", outDir, err)
	}

	emitter := templates.NewModuleIntrospectionEmitter(ctx, schema, schemaVersion, g.Config, pkg, fset)
	typesJSON, err := emitter.ModuleIntrospectionJSON(g.Config.ModuleConfig.ModuleName)
	if err != nil {
		return nil, nil, fmt.Errorf("emit module types: %w", err)
	}
	return genSt, typesJSON, nil
}

// bootstrapModule writes go.mod (and, for a fresh module, a base dagger.gen.go)
// into the overlay, returning the module output subpath and whether another
// pass is needed before the module can be loaded. When partial is false the
// module at outDir is loadable.
//
// It does not scaffold a starter main.go: initializing a new module is the
// SDK's initModule job, and generate only runs on already-initialized modules.
// An uninitialized module (no .go files) fails to load, as it should.
func (g *GoGenerator) bootstrapModule(ctx context.Context, schema *introspection.Schema, schemaVersion string) (_ *memfs.FS, _ *PackageInfo, outDir string, _ *generator.GeneratedState, partial bool, _ error) {
	if g.Config.ModuleConfig == nil {
		return nil, nil, "", nil, false, fmt.Errorf("GenerateModule called but module config is missing")
	}
	moduleConfig := g.Config.ModuleConfig

	generator.SetSchema(schema)

	outDir = filepath.Clean(moduleConfig.ModuleSourcePath)

	mfs := memfs.New()
	layers := []fs.FS{mfs}
	genSt := &generator.GeneratedState{}

	pkgInfo, needsRegen, err := g.bootstrapMod(mfs, genSt)
	if err != nil {
		return nil, nil, "", nil, false, fmt.Errorf("bootstrap package: %w", err)
	}
	partial = needsRegen

	genSt.Overlay = layerfs.New(layers...)

	if outDir != "." {
		mfs.MkdirAll(outDir, 0700)
		sub, err := mfs.Sub(outDir)
		if err != nil {
			return nil, nil, "", nil, false, err
		}
		mfs = sub.(*memfs.FS)
	}

	genFile := filepath.Join(g.Config.OutputDir, outDir, ClientGenFile)
	if _, err := os.Stat(genFile); err != nil {
		// assume package main, default for modules
		pkgInfo.PackageName = "main"

		// generate an initial dagger.gen.go from the base Dagger API so the
		// module's own source can type-check against internal/dagger on the
		// next pass
		if err := generateCode(ctx, g.Config, schema, schemaVersion, mfs, pkgInfo, &moduleGenCtx{pass: 0}); err != nil {
			return nil, nil, "", nil, false, fmt.Errorf("generate code: %w", err)
		}
		partial = true
	}

	return mfs, pkgInfo, outDir, genSt, partial, nil
}

func (g *GoGenerator) bootstrapMod(mfs *memfs.FS, genSt *generator.GeneratedState) (*PackageInfo, bool, error) {
	moduleConfig := g.Config.ModuleConfig

	var needsRegen bool
	var daggerModPath string
	var goMod *modfile.File

	modname := fmt.Sprintf("dagger/%s", strcase.ToKebab(moduleConfig.ModuleName))
	// check for a go.mod already for the dagger module
	if content, err := os.ReadFile(filepath.Join(g.Config.OutputDir, moduleConfig.ModuleSourcePath, "go.mod")); err == nil {
		daggerModPath = moduleConfig.ModuleSourcePath

		goMod, err = modfile.ParseLax("go.mod", content, nil)
		if err != nil {
			return nil, false, fmt.Errorf("parse go.mod: %w", err)
		}
	}

	// could not find a go.mod, so we can init a basic one
	if goMod == nil {
		daggerModPath = moduleConfig.ModuleSourcePath
		goMod = new(modfile.File)

		goMod.AddModuleStmt(modname)
		goMod.AddGoStmt(goVersion)

		needsRegen = true
	}

	// sanity check the parsed go version
	//
	// if this fails, then the go.mod version is too high! and in that case, we
	// won't be able to load the resulting package
	if goMod.Go == nil {
		return nil, false, fmt.Errorf("go.mod has no go directive")
	}
	if semver.Compare("v"+goMod.Go.Version, "v"+goVersion) > 0 {
		return nil, false, fmt.Errorf("existing go.mod has unsupported version %v (highest supported version is %v)", goMod.Go.Version, goVersion)
	}

	if err := g.syncModReplaceAndTidy(goMod, genSt, daggerModPath); err != nil {
		return nil, false, err
	}

	// try and find a go.sum next to the go.mod, and use that to pin
	sum, err := os.ReadFile(filepath.Join(g.Config.OutputDir, daggerModPath, "go.sum"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("could not read go.sum: %w", err)
	}
	sum = append(sum, '\n')
	sum = append(sum, g.Config.ModuleConfig.SDKGoSum...)

	modBody, err := goMod.Format()
	if err != nil {
		return nil, false, fmt.Errorf("format go.mod: %w", err)
	}

	if err := mfs.MkdirAll(daggerModPath, 0700); err != nil {
		return nil, false, err
	}
	if err := mfs.WriteFile(filepath.Join(daggerModPath, "go.mod"), modBody, 0600); err != nil {
		return nil, false, err
	}
	if err := mfs.WriteFile(filepath.Join(daggerModPath, "go.sum"), sum, 0600); err != nil {
		return nil, false, err
	}

	packageImport, err := filepath.Rel(daggerModPath, moduleConfig.ModuleSourcePath)
	if err != nil {
		return nil, false, err
	}
	return &PackageInfo{
		// PackageName is unknown until we load the package
		PackageImport: path.Join(goMod.Module.Mod.Path, packageImport),

		DaggerPkgReplaced: isDaggerPkgCustomReplaced(goMod.Replace),
	}, needsRegen, nil
}

func (g *GoGenerator) syncModReplaceAndTidy(mod *modfile.File, genSt *generator.GeneratedState, modPath string) error {
	modDir := filepath.Join(g.Config.OutputDir, modPath)

	// if there is a go.work, we need to also set overrides there, otherwise
	// modules will have individually conflicting replace directives
	goWork, err := goEnv(modDir, "GOWORK")
	if err != nil {
		return fmt.Errorf("find go.work: %w", err)
	}

	// use the Go SDK library's go.mod as basis for pinning versions
	sdkMod, err := modfile.Parse("go.mod", g.Config.ModuleConfig.SDKGoMod, nil)
	if err != nil {
		return fmt.Errorf("parse SDK go.mod: %w", err)
	}
	modRequires := make(map[string]*modfile.Require)
	for _, req := range mod.Require {
		modRequires[req.Mod.Path] = req
	}
	for _, minReq := range sdkMod.Require {
		// check if mod already at least this version
		if currentReq, ok := modRequires[minReq.Mod.Path]; ok {
			if semver.Compare(currentReq.Mod.Version, minReq.Mod.Version) >= 0 {
				continue
			}
		}
		modRequires[minReq.Mod.Path] = minReq
		mod.AddNewRequire(minReq.Mod.Path, minReq.Mod.Version, minReq.Indirect)
	}

	// preserve any replace directives in sdk/go's go.mod (e.g. pre-1.0 packages)
	for _, minReq := range sdkMod.Replace {
		if _, ok := modRequires[minReq.New.Path]; !ok {
			// ignore anything that's sdk/go only
			continue
		}
		genSt.PostCommands = append(genSt.PostCommands,
			exec.Command("go", "mod", "edit", "-replace", minReq.Old.Path+"="+minReq.New.Path+"@"+minReq.New.Version))
		if goWork != "" {
			genSt.PostCommands = append(genSt.PostCommands,
				exec.Command("go", "work", "edit", "-replace", minReq.Old.Path+"="+minReq.New.Path+"@"+minReq.New.Version))
		}
	}

	// Check if the module go.mod replaces the dagger.io/dagger library with a
	// custom path. If so, keep it as is. Otherwise install the given
	// dagger.io/dagger package version.
	if !isDaggerPkgCustomReplaced(mod.Replace) {
		genSt.PostCommands = append(genSt.PostCommands,
			// Do not pass -u here: LibVersion pins dagger.io/dagger, while -u also
			// asks Go to upgrade transitive dependencies during generation.
			exec.Command("go", "get", daggerImportPath+"@"+g.Config.ModuleConfig.LibVersion))
	}

	genSt.PostCommands = append(genSt.PostCommands,
		// run 'go mod tidy' after generating to fix and prune dependencies
		//
		// NOTE: this has to happen before 'go work use' to synchronize Go version
		// bumps
		exec.Command("go", "mod", "tidy"),
	)

	if goWork != "" {
		// run "go work use ." after generating if we had a go.work at the root
		genSt.PostCommands = append(genSt.PostCommands, exec.Command("go", "work", "use", "."))
	}

	return nil
}

func goEnv(dir string, env string) (string, error) {
	buf := new(bytes.Buffer)
	findGoWork := exec.Command("go", "env", env)
	findGoWork.Dir = dir
	findGoWork.Stdout = buf
	findGoWork.Stderr = os.Stderr
	if err := findGoWork.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
