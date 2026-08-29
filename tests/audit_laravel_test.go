package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestLaravelAuditLintProfileExists(t *testing.T) {
	def, _ := frameworks.Get("laravel")
	if def.AuditLint == nil {
		t.Fatal("laravel AuditLint should not be nil")
	}
	if def.AuditLint.CodingStandard == "" {
		t.Fatal("CodingStandard empty")
	}
	if len(def.AuditLint.Linters) == 0 {
		t.Fatal("Linters empty")
	}
}

func TestLaravelResolveAuditTargetProject(t *testing.T) {
	dir := t.TempDir()
	// fake laravel project: artisan + composer.json with laravel/framework
	os.WriteFile(filepath.Join(dir, "artisan"), []byte("#!/usr/bin/env php"), 0o755)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"laravel/framework":"^11.0"}}`), 0o644)
	def, _ := frameworks.Get("laravel")
	target, ok, err := def.AuditTargetResolver(types.AuditTargetResolveRequest{StartPath: dir, ModeOverride: types.AuditTargetProject})
	if err != nil || !ok {
		t.Fatalf("expected ok, got ok=%v err=%v", ok, err)
	}
	if target.Framework != "laravel" || target.Mode != types.AuditTargetProject {
		t.Fatalf("wrong target %+v", target)
	}
}

func TestLaravelResolveAuditTargetModuleReturnsHelpfulError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "artisan"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"laravel/framework":"^11.0"}}`), 0o644)
	def, _ := frameworks.Get("laravel")
	_, ok, err := def.AuditTargetResolver(types.AuditTargetResolveRequest{StartPath: dir, ModeOverride: types.AuditTargetModule})
	if !ok || err == nil {
		t.Fatalf("expected ok+error for module mode")
	}
	if !contains(err.Error(), "not supported") {
		t.Fatalf("wrong error %v", err)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
