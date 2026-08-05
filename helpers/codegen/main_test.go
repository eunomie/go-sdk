package main

import (
	"context"
	"testing"

	"codegen/generator"
)

func TestModuleFlagsConfigRequiresNameAndSource(t *testing.T) {
	if _, err := (&moduleFlags{sourcePath: "."}).config(); err == nil {
		t.Fatal("expected error when --module-name is missing")
	}
	if _, err := (&moduleFlags{name: "app"}).config(); err == nil {
		t.Fatal("expected error when --module-source-path is missing")
	}
	cfg, err := (&moduleFlags{name: "app", sourcePath: ".", libVersion: "v1.2.3"}).config()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ModuleConfig == nil || cfg.ModuleConfig.ModuleName != "app" || cfg.ModuleConfig.LibVersion != "v1.2.3" {
		t.Fatalf("config not populated: %+v", cfg.ModuleConfig)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	if err := runGenerateModule(context.Background(), []string{"--module-name", "app"}); err == nil {
		t.Fatal("expected error: --module-source-path is required")
	}
	if err := runModuleTypes(context.Background(), []string{"--module-name", "app", "--module-source-path", "."}); err == nil {
		t.Fatal("expected error: --module-types-out is required")
	}
}

func TestValidateBoundModuleKind(t *testing.T) {
	tests := []struct {
		name    string
		mod     generator.BoundModule
		wantErr bool
	}{
		{name: "git", mod: generator.BoundModule{Kind: "GIT_SOURCE", Ref: "github.com/foo/bar@main", Pin: "abc"}},
		{name: "local", mod: generator.BoundModule{Kind: "LOCAL_SOURCE", Path: "/mods/bar"}},
		{name: "dir (local module resolves as dir)", mod: generator.BoundModule{Kind: "DIR_SOURCE", Path: "/mods/bar"}},
		{name: "unknown rejected", mod: generator.BoundModule{Kind: "WAT"}, wantErr: true},
		{name: "empty rejected", mod: generator.BoundModule{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBoundModuleKind(tt.mod)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
