package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func writeDDEVConfig(t *testing.T, root, name, body string) {
	t.Helper()

	dir := filepath.Join(root, ".ddev")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .ddev: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestMigrateFromDDEVReadsOverridesInOrder(t *testing.T) {
	root := t.TempDir()
	writeDDEVConfig(t, root, "config.yaml", "name: acme-shop\ntype: magento2\nphp_version: \"8.2\"\n")
	writeDDEVConfig(t, root, "config.override.yaml", "php_version: \"8.3\"\n")

	result, err := engine.MigrateFromDDEV(root)
	if err != nil {
		t.Fatalf("MigrateFromDDEV: %v", err)
	}

	if result.PHPVersion != "8.3" {
		t.Fatalf("PHPVersion = %q, want the override 8.3", result.PHPVersion)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("Sources = %v, want both files", result.Sources)
	}
	if filepath.Base(result.Sources[0]) != "config.yaml" || filepath.Base(result.Sources[1]) != "config.override.yaml" {
		t.Fatalf("Sources = %v, want base first then the override", result.Sources)
	}
}

func TestMigrateFromDDEVReportsUnmappedKeys(t *testing.T) {
	root := t.TempDir()
	writeDDEVConfig(t, root, "config.yaml", `name: acme-shop
type: magento2
hooks:
  post-start:
    - exec: "echo started"
performance_mode: mutagen
`)

	result, err := engine.MigrateFromDDEV(root)
	if err != nil {
		t.Fatalf("MigrateFromDDEV: %v", err)
	}

	joined := strings.Join(result.Unmapped, " | ")
	for _, want := range []string{"hooks", "performance_mode"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Unmapped = %v, want it to name %q", result.Unmapped, want)
		}
	}
}

func TestMigrateFromDDEVMapsDomainsAndToolchain(t *testing.T) {
	root := t.TempDir()
	writeDDEVConfig(t, root, "config.yaml", `name: acme-shop
type: magento2
php_version: "8.3"
webserver_type: apache-fpm
nodejs_version: "20"
composer_version: "2"
additional_hostnames:
  - admin
additional_fqdns:
  - shop.local
xdebug_enabled: true
`)

	result, err := engine.MigrateFromDDEV(root)
	if err != nil {
		t.Fatalf("MigrateFromDDEV: %v", err)
	}

	if result.NodeVersion != "20" {
		t.Fatalf("NodeVersion = %q, want 20", result.NodeVersion)
	}
	if result.ComposerVersion != "2" {
		t.Fatalf("ComposerVersion = %q, want 2", result.ComposerVersion)
	}
	if result.WebServer != "apache" {
		t.Fatalf("WebServer = %q, want apache", result.WebServer)
	}
	if !result.XdebugEnabled {
		t.Fatalf("XdebugEnabled = false, want true")
	}
	joined := strings.Join(result.ExtraDomains, " ")
	for _, want := range []string{"admin.acme-shop.test", "shop.local"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ExtraDomains = %v, want it to contain %q", result.ExtraDomains, want)
		}
	}
}

func TestMigrateFromDDEVNamesEveryDroppedKey(t *testing.T) {
	root := t.TempDir()
	writeDDEVConfig(t, root, "config.yaml", `name: acme-shop
type: magento2
upload_dirs:
  - ../media
webimage_extra_packages: [php8.3-ldap]
omit_containers: [dba]
`)

	result, err := engine.MigrateFromDDEV(root)
	if err != nil {
		t.Fatalf("MigrateFromDDEV: %v", err)
	}

	joined := strings.Join(result.Unmapped, " | ")
	for _, want := range []string{"upload_dirs", "webimage_extra_packages", "omit_containers"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Unmapped = %v, want it to name %q", result.Unmapped, want)
		}
	}
}
