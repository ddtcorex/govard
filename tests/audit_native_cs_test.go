package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/frameworks"
	"govard/internal/frameworks/symfony"
	"govard/internal/frameworks/types"
	"govard/internal/frameworks/wordpress"
)

func readFileForTest(t *testing.T, paths ...string) []byte {
	t.Helper()
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			return data
		}
	}
	t.Fatalf("read %v failed", paths)
	return nil
}

func TestLintGovardUsesNativeWordPressWhenPresent(t *testing.T) {
	def, _ := frameworks.Get("wordpress")
	if def.AuditLint == nil {
		t.Fatal("wordpress AuditLint nil")
	}
	if def.AuditLint.CodingStandard != "WordPress" {
		t.Fatalf("expected WordPress, got %s", def.AuditLint.CodingStandard)
	}
	// Verify Dockerfile bundles WPCS natively so no fallback to PSR12
	content := readFileForTest(t, "docker/audit-magento/Dockerfile", "../docker/audit-magento/Dockerfile")
	text := string(content)
	if !strings.Contains(text, "wp-coding-standards/wpcs") {
		t.Fatalf("Dockerfile should bundle WPCS for WordPress native")
	}
	if !strings.Contains(text, "wpcs:^3.1") {
		t.Fatalf("Dockerfile should pin wpcs:^3.1, got %q", text)
	}
	if !strings.Contains(text, "dealerdirect/phpcodesniffer-composer-installer") {
		t.Fatalf("Dockerfile should bundle phpcodesniffer installer")
	}
	// Ensure lint_govard.go logs INFO bundled native, not WARNING fallback
	lintContent := readFileForTest(t, "internal/audit/lint_govard.go", "../internal/audit/lint_govard.go")
	lintText := string(lintContent)
	if !strings.Contains(lintText, "bundled, using native") {
		t.Fatalf("lint_govard.go should log bundled native INFO, got %q", lintText)
	}
}

func TestLintGovardUsesNativeSymfonyWhenPresent(t *testing.T) {
	def, _ := frameworks.Get("symfony")
	if def.AuditLint == nil {
		t.Fatal("symfony AuditLint nil")
	}
	if def.AuditLint.CodingStandard != "Symfony" {
		t.Fatalf("expected Symfony, got %s", def.AuditLint.CodingStandard)
	}
	content := readFileForTest(t, "docker/audit-magento/Dockerfile", "../docker/audit-magento/Dockerfile")
	text := string(content)
	if !strings.Contains(text, "phpstan/phpstan-symfony") {
		t.Fatalf("Dockerfile should bundle phpstan-symfony for Symfony native")
	}
	lintContent := readFileForTest(t, "internal/audit/lint_govard.go", "../internal/audit/lint_govard.go")
	if !strings.Contains(string(lintContent), "bundled, using native") {
		t.Fatalf("lint_govard should log bundled native")
	}
}

func TestDockerfileBundlesSymfonyAndWordPressCS(t *testing.T) {
	content := readFileForTest(t, "docker/audit-magento/Dockerfile", "../docker/audit-magento/Dockerfile")
	text := string(content)
	// Task requires both WPCS and Symfony CS (via phpstan extensions)
	for _, want := range []string{
		"wp-coding-standards/wpcs:^3.1",
		"dealerdirect/phpcodesniffer-composer-installer",
		"phpstan/phpstan-symfony",
		// phpstan/phpstan-wordpress does not exist as a package — szepeviktor/phpstan-wordpress is the correct one
		"szepeviktor/phpstan-wordpress",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}
	// Brief-exact snippet: installed_paths with wpcs + squizlabs + fallback
	if !strings.Contains(text, "squizlabs/php_codesniffer/src/Standards") {
		t.Fatalf("Dockerfile should set installed_paths with squizlabs Standards")
	}
	if !strings.Contains(text, "$(composer global config home)/vendor/bin/phpcs --config-set") {
		t.Fatalf("Dockerfile should retain || fallback phpcs --config-set for PATH robustness")
	}
	// Keep legacy szepeviktor package alongside new one (review finding 3)
	if !strings.Contains(text, "szepeviktor/phpstan-wordpress") {
		t.Fatalf("Dockerfile should keep szepeviktor/phpstan-wordpress alongside phpstan/phpstan-wordpress")
	}
	// Task 6 fix: bundle Symfony CS globally via escapestudios/symfony2-coding-standard
	if !strings.Contains(text, "escapestudios/symfony2-coding-standard") {
		t.Fatalf("Dockerfile should bundle escapestudios/symfony2-coding-standard for Symfony native")
	}
	if !strings.Contains(text, "escapestudios/symfony2-coding-standard:^3.0") {
		t.Fatalf("Dockerfile should pin escapestudios/symfony2-coding-standard:^3.0, got %q", text)
	}
	if !strings.Contains(text, "vendor/escapestudios/symfony2-coding-standard") {
		t.Fatalf("Dockerfile should set installed_paths to include Symfony standard")
	}
}

func TestLintGovardFallbackLogIsInfoNotWarning(t *testing.T) {
	content := readFileForTest(t, "internal/audit/lint_govard.go", "../internal/audit/lint_govard.go")
	text := string(content)
	if strings.Contains(text, `pterm.Warning.Printf("CodingStandard %q not found`) {
		t.Fatalf("lint_govard.go should not use Warning for missing CodingStandard when bundled; should be Info")
	}
	if !strings.Contains(text, "bundled, using native") {
		t.Fatalf("lint_govard.go missing bundled Info log")
	}
	if !strings.Contains(text, "falling back to PSR12") {
		t.Fatalf("lint_govard.go fallback should log distinct 'falling back to PSR12' at INFO")
	}
	if !strings.Contains(text, "logGovardLintBundledCodingStandard") || !strings.Contains(text, "logGovardLintFallbackCodingStandard") {
		t.Fatalf("lint_govard.go should extract helpers to avoid duplicate logging")
	}
	// Should not have early unconditional INFO before error — only on success path
	// Helper fallback appears 3 times, bundled only once on success
	if count := strings.Count(text, "bundled, using native"); count != 1 && count != 2 {
		// bundled helper definition + one success call = 2 occurrences total
		t.Fatalf("lint_govard.go should have bundled log only on success path (expected 2 occurrences incl helper), got %d", count)
	}
}

// Brief-exact: WordPress Bedrock (web/wp) vs classic (wp-includes/version.php) both resolve to WordPress with native CS.
func TestLintGovardUsesNativeWordPressWhenPresent_BriefExact_BedrockAndClassic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		layout func(root string)
	}{
		{name: "bedrock web/wp", layout: func(root string) {
			if err := os.MkdirAll(filepath.Join(root, "web", "wp", "wp-includes"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "web", "wp", "wp-includes", "version.php"), []byte("<?php $wp_version='6.6';"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "classic wp-includes", layout: func(root string) {
			if err := os.MkdirAll(filepath.Join(root, "wp-includes"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "wp-includes", "version.php"), []byte("<?php $wp_version='6.6';"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.layout(root)
			// Simulate brief's lintSettingsFor(AuditTarget{Framework:"wordpress", ProjectRoot:"/tmp/wp-bedrock", Mode:"project"})
			target, ok, err := wordpress.ResolveAuditTarget(types.AuditTargetResolveRequest{StartPath: root, ModeOverride: types.AuditTargetProject})
			if err != nil {
				t.Fatalf("ResolveAuditTarget error: %v", err)
			}
			if !ok {
				t.Fatalf("expected WordPress project detected at %s", root)
			}
			if target.Framework != "wordpress" || target.Mode != types.AuditTargetProject {
				t.Fatalf("unexpected target %#v", target)
			}
			// lintSettingsFor equivalent: framework's AuditLint profile
			def, _ := frameworks.Get(target.Framework)
			if def.AuditLint == nil || def.AuditLint.CodingStandard != "WordPress" {
				t.Fatalf("expected WordPress native CS, got %#v", def.AuditLint)
			}
			if def.AuditLint.CodingStandard == "PSR12" {
				t.Fatalf("should be WordPress not fallback PSR12 for %s", tc.name)
			}
		})
	}
}

func TestLintGovardUsesNativeSymfonyWhenPresent_BriefExact(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "console"), []byte("#!/usr/bin/env php"), 0o755); err != nil {
		t.Fatal(err)
	}
	target, ok, err := symfony.ResolveAuditTarget(types.AuditTargetResolveRequest{StartPath: root, ModeOverride: types.AuditTargetProject})
	if err != nil {
		t.Fatalf("ResolveAuditTarget error: %v", err)
	}
	if !ok {
		t.Fatalf("expected Symfony project detected at %s", root)
	}
	if target.Framework != "symfony" || target.Mode != types.AuditTargetProject {
		t.Fatalf("unexpected target %#v", target)
	}
	def, _ := frameworks.Get(target.Framework)
	if def.AuditLint == nil || def.AuditLint.CodingStandard != "Symfony" {
		t.Fatalf("expected Symfony native CS, got %#v", def.AuditLint)
	}
}
