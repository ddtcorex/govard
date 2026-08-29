package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestWordPressAuditLintProfileExists(t *testing.T) {
	def, _ := frameworks.Get("wordpress")
	if def.AuditLint == nil {
		t.Fatal("wordpress AuditLint should not be nil")
	}
	if def.AuditLint.CodingStandard == "" {
		t.Fatal("CodingStandard empty")
	}
	if len(def.AuditLint.Linters) == 0 {
		t.Fatal("Linters empty")
	}
	if def.AuditLint.CodingStandard != "WordPress" {
		t.Fatalf("CodingStandard = %q, want %q", def.AuditLint.CodingStandard, "WordPress")
	}
	if def.AuditLint.PHPStanLevel != 5 {
		t.Fatalf("PHPStanLevel = %d, want 5", def.AuditLint.PHPStanLevel)
	}
	wantVersions := []string{"8.1", "8.2", "8.3", "8.4"}
	if len(def.AuditLint.ProjectPHPVersions) != len(wantVersions) {
		t.Fatalf("ProjectPHPVersions = %v, want %v", def.AuditLint.ProjectPHPVersions, wantVersions)
	}
	for i, v := range wantVersions {
		if def.AuditLint.ProjectPHPVersions[i] != v {
			t.Fatalf("ProjectPHPVersions[%d] = %q, want %q", i, def.AuditLint.ProjectPHPVersions[i], v)
		}
	}
}

func TestWordPressResolveAuditTargetProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "wp-config.php"), []byte("<?php"), 0o644)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"johnpbloch/wordpress":"^6.0"}}`), 0o644)
	def, _ := frameworks.Get("wordpress")
	target, ok, err := def.AuditTargetResolver(types.AuditTargetResolveRequest{StartPath: dir, ModeOverride: types.AuditTargetProject})
	if err != nil || !ok {
		t.Fatalf("expected ok, got ok=%v err=%v", ok, err)
	}
	if target.Framework != "wordpress" || target.Mode != types.AuditTargetProject {
		t.Fatalf("wrong target %+v", target)
	}
	if target.ProjectRoot != dir {
		t.Fatalf("ProjectRoot = %q, want %q", target.ProjectRoot, dir)
	}
}

func TestWordPressBedrockLayout(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "web/wp/wp-includes"), 0o755)
	os.WriteFile(filepath.Join(dir, "web/wp/wp-includes/version.php"), []byte("<?php $wp_version='6.0';"), 0o644)
	def, _ := frameworks.Get("wordpress")
	target, ok, err := def.AuditTargetResolver(types.AuditTargetResolveRequest{StartPath: dir, ModeOverride: types.AuditTargetProject})
	if err != nil || !ok {
		t.Fatalf("expected ok for Bedrock layout, got ok=%v err=%v", ok, err)
	}
	if target.Framework != "wordpress" {
		t.Fatalf("Framework = %q, want wordpress", target.Framework)
	}
	if target.ProjectRoot != dir {
		t.Fatalf("ProjectRoot = %q, want %q", target.ProjectRoot, dir)
	}
	if target.Mode != types.AuditTargetProject {
		t.Fatalf("Mode = %q, want project", target.Mode)
	}
}

func TestWordPressResolveAuditTargetModuleReturnsHelpfulError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "wp-config.php"), []byte("<?php"), 0o644)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"johnpbloch/wordpress":"^6.0"}}`), 0o644)
	def, _ := frameworks.Get("wordpress")
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
