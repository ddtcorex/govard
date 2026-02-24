package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestLoadConfigFromDirLayeredMerge(t *testing.T) {
	tempDir := t.TempDir()

	base := `project_name: demo
domain: demo.test
recipe: magento2
stack:
  php_version: "8.3"
  services:
    cache: redis
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
`
	local := `stack:
  php_version: "8.4"
  services:
    cache: valkey
remotes:
  staging:
    port: 2202
`
	envOverride := `domain: demo-staging.test
stack:
  services:
    search: elasticsearch
`

	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "govard.local.yml"), []byte(local), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "govard.staging.yml"), []byte(envOverride), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOVARD_ENV", "staging")

	cfg, loaded, err := engine.LoadConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3 loaded layers, got %d", len(loaded))
	}
	if cfg.Domain != "demo-staging.test" {
		t.Fatalf("expected env domain override, got %s", cfg.Domain)
	}
	if cfg.Stack.PHPVersion != "8.4" {
		t.Fatalf("expected local php override 8.4, got %s", cfg.Stack.PHPVersion)
	}
	if cfg.Stack.Services.Cache != "valkey" {
		t.Fatalf("expected cache valkey, got %s", cfg.Stack.Services.Cache)
	}
	if cfg.Stack.Services.Search != "elasticsearch" {
		t.Fatalf("expected env search override, got %s", cfg.Stack.Services.Search)
	}

	staging := cfg.Remotes["staging"]
	if staging.Port != 2202 {
		t.Fatalf("expected remote port 2202, got %d", staging.Port)
	}
	if staging.Host != "staging.example.com" {
		t.Fatalf("expected remote host merge, got %s", staging.Host)
	}
}

func TestLoadBaseConfigFromDirIgnoresOverrides(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte("project_name: demo\ndomain: base.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "govard.local.yml"), []byte("domain: local.test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := engine.LoadBaseConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load base config: %v", err)
	}
	if cfg.Domain != "base.test" {
		t.Fatalf("expected base domain, got %s", cfg.Domain)
	}
}

func TestLoadConfigFromDirRequiresBase(t *testing.T) {
	_, _, err := engine.LoadConfigFromDir(t.TempDir(), true)
	if err == nil {
		t.Fatal("expected error when base config is missing")
	}
}

func TestResolveConfigLayerPathsSkipsInvalidEnvName(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GOVARD_ENV", "../prod")

	paths := engine.ResolveConfigLayerPaths(tempDir)
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths for invalid env, got %d", len(paths))
	}
}

func TestLoadConfigFromDirPrefersProjectExtensionLocalOverride(t *testing.T) {
	tempDir := t.TempDir()

	base := `project_name: demo
domain: demo.test
recipe: laravel
stack:
  php_version: "8.2"
  services:
    cache: redis
`
	legacyLocal := `stack:
  php_version: "8.3"
`
	projectLocal := `stack:
  php_version: "8.4"
  services:
    cache: valkey
`

	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "govard.local.yml"), []byte(legacyLocal), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, ".govard"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".govard", "govard.local.yml"), []byte(projectLocal), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := engine.LoadConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Stack.PHPVersion != "8.4" {
		t.Fatalf("expected project-local php override 8.4, got %s", cfg.Stack.PHPVersion)
	}
	if cfg.Stack.Services.Cache != "valkey" {
		t.Fatalf("expected project-local cache override valkey, got %s", cfg.Stack.Services.Cache)
	}
}

func TestLoadConfigFromDirPrefersProjectExtensionEnvOverride(t *testing.T) {
	tempDir := t.TempDir()

	base := `project_name: demo
domain: demo.test
recipe: laravel
`
	legacyEnv := `domain: legacy-staging.test
`
	projectEnv := `domain: extension-staging.test
`

	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "govard.staging.yml"), []byte(legacyEnv), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, ".govard"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".govard", "govard.staging.yml"), []byte(projectEnv), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOVARD_ENV", "staging")

	cfg, _, err := engine.LoadConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Domain != "extension-staging.test" {
		t.Fatalf("expected extension env override, got %s", cfg.Domain)
	}
}

func TestLoadConfigFromDirParsesLockStrictFlag(t *testing.T) {
	tempDir := t.TempDir()

	base := `project_name: demo
domain: demo.test
recipe: magento2
lock:
  strict: true
`

	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := engine.LoadConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.Lock.Strict {
		t.Fatal("expected lock.strict=true from config")
	}
}

func TestLoadConfigFromDirParsesBlueprintRegistrySettings(t *testing.T) {
	tempDir := t.TempDir()

	base := `project_name: demo
domain: demo.test
recipe: legacytest
blueprint_registry:
  provider: HTTP
  url: https://example.com/blueprints.tar.gz
  checksum: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
  trusted: true
`

	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := engine.LoadConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.BlueprintRegistry.Provider != "http" {
		t.Fatalf("expected normalized provider http, got %s", cfg.BlueprintRegistry.Provider)
	}
	if cfg.BlueprintRegistry.Checksum != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected lowercase checksum normalization, got %s", cfg.BlueprintRegistry.Checksum)
	}
	if !cfg.BlueprintRegistry.Trusted {
		t.Fatal("expected blueprint registry trusted=true")
	}
}

func TestLoadConfigFromDirInfersHTTPProviderFromUppercaseScheme(t *testing.T) {
	tempDir := t.TempDir()

	base := `project_name: demo
domain: demo.test
recipe: legacytest
blueprint_registry:
  url: HTTPS://example.com/blueprints.tar.gz
  checksum: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  trusted: true
`

	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := engine.LoadConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.BlueprintRegistry.Provider != "http" {
		t.Fatalf("expected inferred provider http, got %s", cfg.BlueprintRegistry.Provider)
	}
}

func TestLoadConfigFromDirAllowsUppercaseHTTPSWithExplicitHTTPProvider(t *testing.T) {
	tempDir := t.TempDir()

	base := `project_name: demo
domain: demo.test
recipe: legacytest
blueprint_registry:
  provider: http
  url: HTTPS://example.com/blueprints.tar.gz
  checksum: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  trusted: true
`

	if err := os.WriteFile(filepath.Join(tempDir, "govard.yml"), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := engine.LoadConfigFromDir(tempDir, true); err != nil {
		t.Fatalf("expected config to accept uppercase https scheme, got error: %v", err)
	}
}
