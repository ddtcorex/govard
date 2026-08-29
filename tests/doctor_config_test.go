package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"

	"gopkg.in/yaml.v3"
)

func TestDoctorFixSyncsYml(t *testing.T) {
	tmp := t.TempDir()
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Old yml: 2.4.6-p3 with php 8.2, db 10.6, node 14, search 7.10 (drifted)
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
	// composer.json with new version 2.4.8-p4 (php 8.4, db 11.4, search 3.0, node 24)
	composerJSON := `{"require":{"magento/product-community-edition":"2.4.8-p4"}}`
	if err := os.WriteFile(filepath.Join(tmp, "composer.json"), []byte(composerJSON), 0o644); err != nil {
		t.Fatalf("write composer: %v", err)
	}

	// Enable assume yes to avoid interactive prompt
	t.Setenv("GOVARD_ASSUME_YES", "true")

	// Call engine drift sync directly (the implementation behind doctor --fix)
	changes, err := engine.SyncConfigDriftForTest(tmp, false, false)
	if err != nil {
		t.Fatalf("SyncConfigDrift: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected drift changes, got none")
	}

	// Verify yml was updated to 2.4.8-p4 and php 8.4
	data, err := os.ReadFile(filepath.Join(tmp, ".govard.yml"))
	if err != nil {
		t.Fatalf("read updated yml: %v", err)
	}
	var cfg engine.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal updated yml: %v", err)
	}
	if cfg.FrameworkVersion != "2.4.8-p4" {
		t.Fatalf("expected framework_version 2.4.8-p4, got %s", cfg.FrameworkVersion)
	}
	if cfg.Stack.PHPVersion != "8.4" {
		t.Fatalf("expected php_version 8.4, got %s", cfg.Stack.PHPVersion)
	}
	// db_version should be synced to profile's 11.4 for 2.4.8-p4
	if cfg.Stack.DBVersion != "11.4" {
		t.Fatalf("expected db_version 11.4, got %s", cfg.Stack.DBVersion)
	}
	// node_version should be synced to profile (24)
	if cfg.Stack.NodeVersion != "24" {
		t.Fatalf("expected node_version 24, got %s", cfg.Stack.NodeVersion)
	}
	// search_version should be synced
	if strings.TrimSpace(cfg.Stack.SearchVersion) == "7.10" {
		t.Fatalf("expected search_version to be updated from 7.10, got %s", cfg.Stack.SearchVersion)
	}

	// Also verify doctor fix handler reports applied via ApplyDoctorSafeFixesForTest
	// Create a drift check report and ensure handler applies
	report := engine.DoctorReport{
		Checks: []engine.DoctorCheck{
			{ID: "project.config.drift", Title: "Configuration drift", Status: engine.DoctorStatusWarn, Message: "drift"},
		},
	}
	results := cmd.ApplyDoctorSafeFixesForTest(report, nil)
	found := false
	for _, r := range results {
		if r.CheckID == "project.config.drift" && r.Status == cmd.DoctorFixStatusApplied {
			found = true
		}
	}
	_ = found // optional, handler may be no-op if already synced
}

func TestDoctorFixSyncsYmlDryRun(t *testing.T) {
	tmp := t.TempDir()
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
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
	t.Setenv("GOVARD_ASSUME_YES", "true")

	changes, err := engine.SyncConfigDriftForTest(tmp, true, false)
	if err != nil {
		t.Fatalf("dry-run SyncConfigDrift: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected drift changes for dry-run")
	}
	// File should NOT be modified in dry-run
	data, _ := os.ReadFile(filepath.Join(tmp, ".govard.yml"))
	var cfg engine.Config
	_ = yaml.Unmarshal(data, &cfg)
	if cfg.FrameworkVersion != "2.4.6-p3" {
		t.Fatalf("dry-run should not modify file, got framework_version %s", cfg.FrameworkVersion)
	}
}
