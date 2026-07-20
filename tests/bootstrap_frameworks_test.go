package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestBootstrapPkgLaravelFreshCommands(t *testing.T) {
	cases := []struct {
		version  string
		expected string
	}{
		{"11", "laravel/laravel"},
		{"10", "laravel/laravel:^10.0"},
		{"9", "laravel/laravel:^9.0"},
		{"", "laravel/laravel"},
	}

	for _, tc := range cases {
		opts := bootstrap.Options{Version: tc.version}
		laravel := bootstrap.NewLaravelBootstrap(opts)
		cmds := laravel.FreshCommands()

		if len(cmds) == 0 {
			t.Fatalf("expected commands for version %s, got none", tc.version)
		}

		if !containsSubstring(cmds[0], tc.expected) {
			t.Errorf("expected command to contain %q for version %s, got %q", tc.expected, tc.version, cmds[0])
		}
	}
}

func TestBootstrapPkgDrupalFreshCommands(t *testing.T) {
	cases := []struct {
		version  string
		expected string
	}{
		{"11", "drupal/recommended-project"},
		{"10", "drupal/recommended-project:^10"},
		{"9", "drupal/recommended-project:^9"},
		{"", "drupal/recommended-project"},
	}

	for _, tc := range cases {
		opts := bootstrap.Options{Version: tc.version}
		drupal := bootstrap.NewDrupalBootstrap(opts)
		cmds := drupal.FreshCommands()

		if len(cmds) == 0 {
			t.Fatalf("expected commands for version %s, got none", tc.version)
		}

		if !containsSubstring(cmds[0], tc.expected) {
			t.Errorf("expected command to contain %q for version %s, got %q", tc.expected, tc.version, cmds[0])
		}
	}
}

func TestBootstrapPkgOpenMageFreshCommands(t *testing.T) {
	opts := bootstrap.Options{}
	openmage := bootstrap.NewOpenMageBootstrap(opts)
	cmds := openmage.FreshCommands()

	if len(cmds) == 0 {
		t.Fatal("expected commands for OpenMage, got none")
	}

	if !containsSubstring(cmds[0], "openmage/magento-lts") {
		t.Errorf("expected command to contain 'openmage/magento-lts', got %q", cmds[0])
	}
}

func TestBootstrapPkgOpenMagePostCloneWritesTablePrefix(t *testing.T) {
	projectDir := t.TempDir()
	openmage := bootstrap.NewOpenMageBootstrap(bootstrap.Options{TablePrefix: "demo_"})

	if err := openmage.PostClone(projectDir); err != nil {
		t.Fatalf("PostClone() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectDir, "app", "etc", "local.xml"))
	if err != nil {
		t.Fatalf("read local.xml: %v", err)
	}
	if !strings.Contains(string(content), "<table_prefix><![CDATA[demo_]]></table_prefix>") {
		t.Fatalf("expected local.xml table prefix, got:\n%s", string(content))
	}
}

func TestBootstrapPkgNextJSFreshInstallSupport(t *testing.T) {
	opts := bootstrap.Options{}
	nextjs := bootstrap.NewNextJSBootstrap(opts)

	if !nextjs.SupportsFreshInstall() {
		t.Error("expected Next.js to support fresh install")
	}

	if !nextjs.SupportsClone() {
		t.Error("expected Next.js to support clone")
	}
}

func TestBootstrapPkgEmdashFreshInstallSupport(t *testing.T) {
	opts := bootstrap.Options{}
	emdash := bootstrap.NewEmdashBootstrap(opts)

	if !emdash.SupportsFreshInstall() {
		t.Error("expected Emdash to support fresh install")
	}

	if emdash.SupportsClone() {
		t.Error("expected Emdash clone support to remain disabled")
	}
}

func TestPatchEmdashAstroConfigAddsTrustedForwardedDomainSupport(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "astro.config.mjs")
	initial := `import node from "@astrojs/node";
import react from "@astrojs/react";
import { defineConfig } from "astro/config";
import emdash, { local } from "emdash/astro";
import { sqlite } from "emdash/db";

export default defineConfig({
	output: "server",
	adapter: node({
		mode: "standalone",
	}),
	devToolbar: { enabled: false },
});
`
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write astro config: %v", err)
	}

	if err := bootstrap.PatchEmdashAstroConfigForTest(projectDir); err != nil {
		t.Fatalf("patch astro config: %v", err)
	}

	contentBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read astro config: %v", err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, "const trustedForwardedDomain = process.env.GOVARD_TRUSTED_DOMAIN?.trim();") {
		t.Fatalf("expected trusted forwarded domain env var support, got:\n%s", content)
	}
	if !strings.Contains(content, "allowedDomains: [{ hostname: trustedForwardedDomain, protocol: \"https\" }]") {
		t.Fatalf("expected Astro allowedDomains config, got:\n%s", content)
	}
	if !strings.Contains(content, "\"#auth/passkey-config.js\": fileURLToPath(new URL(\"./src/govard/passkey-config.ts\", import.meta.url))") {
		t.Fatalf("expected Astro alias override for passkey config, got:\n%s", content)
	}
	if strings.Index(content, "const trustedForwardedDomain") < strings.Index(content, "import { sqlite } from \"emdash/db\";") {
		t.Fatalf("expected trustedForwardedDomain to be declared after imports, got:\n%s", content)
	}
}

func TestWriteEmdashPasskeyShimCreatesGovardOverride(t *testing.T) {
	projectDir := t.TempDir()

	if err := bootstrap.WriteEmdashPasskeyShimForTest(projectDir); err != nil {
		t.Fatalf("write passkey shim: %v", err)
	}

	shimPath := filepath.Join(projectDir, "src", "govard", "passkey-config.ts")
	contentBytes, err := os.ReadFile(shimPath)
	if err != nil {
		t.Fatalf("read passkey shim: %v", err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, "const trustedForwardedDomain = process.env.GOVARD_TRUSTED_DOMAIN?.trim();") {
		t.Fatalf("expected trusted domain env override in shim, got:\n%s", content)
	}
	if !strings.Contains(content, "const origin = trustedForwardedDomain ? \"https://\" + trustedForwardedDomain : url.origin;") {
		t.Fatalf("expected https origin override in shim, got:\n%s", content)
	}
}

func TestBootstrapDispatcherAllFrameworks(t *testing.T) {
	frameworks := []string{
		"magento2",
		"magento1",
		"openmage",
		"symfony",
		"laravel",
		"drupal",
		"wordpress",
		"nextjs",
		"emdash",
		"shopware",
		"cakephp",
		"prestashop",
	}

	opts := bootstrap.DefaultOptions()

	for _, fw := range frameworks {
		t.Run(fw, func(t *testing.T) {
			err := bootstrap.Run(fw, opts)
			if err != nil {
				t.Fatalf("Run(%s) failed: %v", fw, err)
			}
		})
	}
}
