package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"

	"gopkg.in/yaml.v3"
)

func TestConfigDriftDetection(t *testing.T) {
	tmp := t.TempDir()
	oldYml := `project_name: drift-test
framework: magento2
framework_version: 2.4.6-p3
domain: drift-test.test
stack:
  php_version: "8.2"
  db_version: "10.6"
  node_version: "14"
  search_version: "7.10"
  services:
    db: mariadb
    search: elasticsearch
`
	if err := os.WriteFile(filepath.Join(tmp, ".govard.yml"), []byte(oldYml), 0o644); err != nil {
		t.Fatalf("write yml: %v", err)
	}
	composerJSON := `{"require":{"magento/product-community-edition":"2.4.8-p4"}}`
	if err := os.WriteFile(filepath.Join(tmp, "composer.json"), []byte(composerJSON), 0o644); err != nil {
		t.Fatalf("write composer: %v", err)
	}

	warnings := engine.CollectConfigDriftWarningsForTest(tmp)
	if len(warnings) == 0 {
		t.Fatal("expected drift warnings, got none")
	}
	foundFramework := false
	foundPHP := false
	for _, w := range warnings {
		if strings.Contains(w, "framework_version") {
			foundFramework = true
		}
		if strings.Contains(w, "php_version") {
			foundPHP = true
		}
	}
	if !foundFramework {
		t.Fatalf("expected framework_version drift warning, got %v", warnings)
	}
	if !foundPHP {
		t.Fatalf("expected php_version drift warning, got %v", warnings)
	}
}

func TestConfigNormalizeHandlesDrift(t *testing.T) {
	cfg := engine.Config{
		Framework:        "magento2",
		FrameworkVersion: "2.4.8-p4",
		Stack: engine.Stack{
			PHPVersion:    "8.4",
			DBVersion:     "11.4",
			NodeVersion:   "24",
			SearchVersion: "3.0",
		},
	}
	engine.NormalizeConfig(&cfg, "")
	if cfg.Stack.PHPVersion != "8.4" {
		t.Fatalf("expected php 8.4 preserved, got %s", cfg.Stack.PHPVersion)
	}
	if cfg.FrameworkVersion != "2.4.8-p4" {
		t.Fatalf("expected framework_version preserved, got %s", cfg.FrameworkVersion)
	}
}

func TestConfigValidateDrift(t *testing.T) {
	cfg := engine.Config{
		ProjectName:      "demo",
		Framework:        "magento2",
		FrameworkVersion: "2.4.8-p4",
		Domain:           "demo.test",
		Stack: engine.Stack{
			PHPVersion:    "8.4",
			DBVersion:     "11.4",
			NodeVersion:   "24",
			SearchVersion: "3.0",
			Services: engine.Services{
				DB:        "mariadb",
				Search:    "opensearch",
				WebServer: "nginx",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}
	if err := engine.ValidateConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	// Invalid node version should still validate (free-form) but drift helper should flag
	bad := cfg
	bad.Stack.NodeVersion = "14"
	// Validate should not error on old node, drift detection should
	warnings := engine.CollectConfigDriftWarningsForTestWithConfig(bad, engine.ProjectMetadata{Framework: "magento2", Version: "2.4.8-p4"})
	foundNode := false
	for _, w := range warnings {
		if strings.Contains(w, "node_version") {
			foundNode = true
		}
	}
	if !foundNode {
		t.Fatalf("expected node_version drift warning for 14, got %v", warnings)
	}
}

func TestPrepareConfigForWritePreservesDriftSync(t *testing.T) {
	cfg := engine.Config{
		ProjectName:      "drift-test",
		Framework:        "magento2",
		FrameworkVersion: "2.4.8-p4",
		Domain:           "drift-test.test",
		Stack: engine.Stack{
			PHPVersion:    "8.4",
			DBVersion:     "11.4",
			NodeVersion:   "24",
			SearchVersion: "3.0",
			Services: engine.Services{
				DB:     "mariadb",
				Search: "opensearch",
			},
		},
	}
	writable := engine.PrepareConfigForWrite(cfg)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["framework_version"] != "2.4.8-p4" {
		t.Fatalf("expected framework_version preserved, got %v", out["framework_version"])
	}
}
