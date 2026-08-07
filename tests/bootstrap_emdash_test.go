package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/frameworks/emdash"

	"govard/internal/engine/bootstrap"
)

func TestBootstrapPkgEmdashFreshInstallSupport(t *testing.T) {
	opts := bootstrap.Options{}
	emdashBootstrap := emdash.NewEmdashBootstrap(opts)

	if !emdashBootstrap.SupportsFreshInstall() {
		t.Error("expected Emdash to support fresh install")
	}

	if emdashBootstrap.SupportsClone() {
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

	if err := emdash.PatchEmdashAstroConfigForTest(projectDir); err != nil {
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

	if err := emdash.WriteEmdashPasskeyShimForTest(projectDir); err != nil {
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
