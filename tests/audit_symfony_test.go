package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestSymfonyAuditLintProfileExists(t *testing.T) {
	def, _ := frameworks.Get("symfony")
	if def.AuditLint == nil {
		t.Fatal("symfony AuditLint should not be nil")
	}
	if def.AuditLint.CodingStandard == "" {
		t.Fatal("CodingStandard empty")
	}
	if len(def.AuditLint.Linters) == 0 {
		t.Fatal("Linters empty")
	}
	if def.AuditLint.CodingStandard != "Symfony" {
		t.Fatalf("CodingStandard = %q, want %q", def.AuditLint.CodingStandard, "Symfony")
	}
	if def.AuditLint.PHPStanExtension != "phpstan/phpstan-symfony" {
		t.Fatalf("PHPStanExtension = %q, want %q", def.AuditLint.PHPStanExtension, "phpstan/phpstan-symfony")
	}
	if def.AuditLint.PHPStanLevel != 5 {
		t.Fatalf("PHPStanLevel = %d, want 5", def.AuditLint.PHPStanLevel)
	}
}

func TestSymfonyResolveAuditTargetProject(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bin"), 0o755)
	os.WriteFile(filepath.Join(dir, "bin", "console"), []byte(""), 0o755)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"symfony/framework-bundle":"^7.0"}}`), 0o644)
	def, _ := frameworks.Get("symfony")
	target, ok, err := def.AuditTargetResolver(types.AuditTargetResolveRequest{StartPath: dir, ModeOverride: types.AuditTargetProject})
	if err != nil || !ok {
		t.Fatalf("expected ok, got ok=%v err=%v", ok, err)
	}
	if target.Framework != "symfony" || target.Mode != types.AuditTargetProject {
		t.Fatalf("wrong target %+v", target)
	}
}

func TestSymfonyResolveAuditTargetModuleReturnsHelpfulError(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bin"), 0o755)
	os.WriteFile(filepath.Join(dir, "bin", "console"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"symfony/framework-bundle":"^7.0"}}`), 0o644)
	def, _ := frameworks.Get("symfony")
	_, ok, err := def.AuditTargetResolver(types.AuditTargetResolveRequest{StartPath: dir, ModeOverride: types.AuditTargetModule})
	if !ok || err == nil {
		t.Fatalf("expected ok+error for module mode")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("wrong error %v", err)
	}
	if !strings.Contains(err.Error(), "use --mode project") {
		t.Fatalf("error should hint use --mode project, got %v", err)
	}
}
