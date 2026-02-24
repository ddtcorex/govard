package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestEnginePkgResolveConfigLayerPathsIncludesEnvWhenValid(t *testing.T) {
	t.Setenv("GOVARD_ENV", "staging")
	root := t.TempDir()
	paths := engine.ResolveConfigLayerPaths(root)
	if len(paths) != 5 {
		t.Fatalf("expected 5 paths, got %d", len(paths))
	}
}

func TestEnginePkgLoadConfigFromDirLayeredMerge(t *testing.T) {
	t.Setenv("GOVARD_ENV", "staging")
	root := t.TempDir()

	mustWriteTestFile(t, filepath.Join(root, "govard.yml"), `project_name: demo
recipe: laravel
domain: demo.test
stack:
  php_version: "8.3"
  services:
    cache: redis
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/app
`)
	mustWriteTestFile(t, filepath.Join(root, "govard.local.yml"), `stack:
  php_version: "8.4"
`)
	mustWriteTestFile(t, filepath.Join(root, "govard.staging.yml"), `domain: legacy-staging.test
`)
	mustWriteTestFile(t, filepath.Join(root, ".govard", "govard.staging.yml"), `domain: extension-staging.test
stack:
  services:
    cache: valkey
`)

	cfg, loaded, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("LoadConfigFromDir() error = %v", err)
	}
	if len(loaded) != 4 {
		t.Fatalf("expected 4 loaded files, got %d", len(loaded))
	}
	if cfg.Domain != "extension-staging.test" {
		t.Fatalf("expected env extension override, got %s", cfg.Domain)
	}
	if cfg.Stack.PHPVersion != "8.4" {
		t.Fatalf("expected local php override 8.4, got %s", cfg.Stack.PHPVersion)
	}
	if cfg.Stack.Services.Cache != "valkey" {
		t.Fatalf("expected cache valkey, got %s", cfg.Stack.Services.Cache)
	}
}

func TestEnginePkgValidateConfigRejectsInvalidValues(t *testing.T) {
	base := engine.Config{
		ProjectName: "demo",
		Domain:      "demo.test",
		Stack: engine.Stack{Services: engine.Services{
			WebServer: "nginx",
			Search:    "none",
			Cache:     "none",
			Queue:     "none",
		}},
	}
	bad := base
	bad.Stack.Services.WebServer = "caddy"
	if err := engine.ValidateConfig(bad); err == nil {
		t.Fatal("expected invalid web server error")
	}
}

func TestEnginePkgValidateConfigAllowsHybridWebServer(t *testing.T) {
	cfg := engine.Config{
		ProjectName: "demo",
		Domain:      "demo.test",
		Stack: engine.Stack{Services: engine.Services{
			WebServer: "hybrid",
			Search:    "none",
			Cache:     "none",
			Queue:     "none",
		}},
	}
	if err := engine.ValidateConfig(cfg); err != nil {
		t.Fatalf("expected hybrid web server to be valid, got %v", err)
	}
}

func TestEnginePkgNormalizeAndFrameworkDefaults(t *testing.T) {
	cfg := engine.Config{Recipe: "magento2"}
	engine.NormalizeConfig(&cfg)
	if cfg.Stack.Services.Cache != "valkey" || cfg.Stack.Services.Search != "opensearch" {
		t.Fatalf("unexpected normalized services: %+v", cfg.Stack.Services)
	}

	framework, ok := engine.GetFrameworkConfig("laravel")
	if !ok || framework.DefaultWebServer != "nginx" {
		t.Fatalf("unexpected laravel framework config: %+v", framework)
	}
}

func TestEnginePkgDetectFrameworkByComposerPackage(t *testing.T) {
	root := t.TempDir()
	data, err := json.Marshal(map[string]map[string]string{"require": {"laravel/framework": "11.0.0"}})
	if err != nil {
		t.Fatalf("marshal composer payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), data, 0o644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}

	got := engine.DetectFramework(root)
	if got.Framework != "laravel" || got.Version != "11.0.0" {
		t.Fatalf("unexpected detected metadata: %+v", got)
	}
}

func mustWriteTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
