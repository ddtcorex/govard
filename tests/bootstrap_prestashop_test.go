package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestBootstrapPkgPrestaShopFreshInstallUnsupported(t *testing.T) {
	prestashop := bootstrap.NewPrestaShopBootstrap(bootstrap.Options{})

	if prestashop.SupportsFreshInstall() {
		t.Fatal("expected PrestaShop fresh install to remain unsupported")
	}
	if !prestashop.SupportsClone() {
		t.Fatal("expected PrestaShop to support clone")
	}

	if err := prestashop.CreateProject(t.TempDir()); err == nil {
		t.Fatal("expected CreateProject to return an error for PrestaShop")
	}
}

func TestBootstrapPkgPrestaShopPostCloneGeneratesParametersFile(t *testing.T) {
	projectDir := t.TempDir()
	prestashop := bootstrap.NewPrestaShopBootstrap(bootstrap.Options{
		DBHost:      "db",
		DBUser:      "shopuser",
		DBPass:      "shoppass",
		DBName:      "shopdb",
		TablePrefix: "shop_",
	})

	if err := prestashop.PostClone(projectDir); err != nil {
		t.Fatalf("PostClone() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectDir, "app", "config", "parameters.php"))
	if err != nil {
		t.Fatalf("read parameters.php: %v", err)
	}
	generated := string(content)

	for _, expected := range []string{
		"'database_host' => 'db'",
		"'database_user' => 'shopuser'",
		"'database_password' => 'shoppass'",
		"'database_name' => 'shopdb'",
		"'database_prefix' => 'shop_'",
	} {
		if !strings.Contains(generated, expected) {
			t.Fatalf("expected generated parameters.php to contain %q, got:\n%s", expected, generated)
		}
	}
}

func TestBootstrapPkgPrestaShopPostClonePatchesExistingParametersFile(t *testing.T) {
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, "app", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir app/config: %v", err)
	}

	existing := `<?php

return array (
  'parameters' =>
  array (
    'database_host' => 'remote-host',
    'database_port' => '',
    'database_name' => 'remote_db',
    'database_user' => 'remote_user',
    'database_password' => 'remote_pass',
    'database_prefix' => 'ps_',
    'mailer_transport' => 'smtp',
  ),
);
`
	parametersPath := filepath.Join(configDir, "parameters.php")
	if err := os.WriteFile(parametersPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write parameters.php: %v", err)
	}

	prestashop := bootstrap.NewPrestaShopBootstrap(bootstrap.Options{
		DBHost:      "db",
		DBUser:      "localuser",
		DBPass:      "localpass",
		DBName:      "localdb",
		TablePrefix: "ps_",
	})

	if err := prestashop.PostClone(projectDir); err != nil {
		t.Fatalf("PostClone() error = %v", err)
	}

	content, err := os.ReadFile(parametersPath)
	if err != nil {
		t.Fatalf("read parameters.php: %v", err)
	}
	patched := string(content)

	if !strings.Contains(patched, "'database_host' => 'db'") {
		t.Fatalf("expected database_host to be patched, got:\n%s", patched)
	}
	if !strings.Contains(patched, "'database_user' => 'localuser'") {
		t.Fatalf("expected database_user to be patched, got:\n%s", patched)
	}
	if !strings.Contains(patched, "'database_name' => 'localdb'") {
		t.Fatalf("expected database_name to be patched, got:\n%s", patched)
	}
	if !strings.Contains(patched, "'mailer_transport' => 'smtp'") {
		t.Fatalf("expected untouched keys to survive patching, got:\n%s", patched)
	}
}

func TestBootstrapPkgPrestaShopPostCloneCreatesWritableDirs(t *testing.T) {
	projectDir := t.TempDir()
	prestashop := bootstrap.NewPrestaShopBootstrap(bootstrap.Options{})

	if err := prestashop.PostClone(projectDir); err != nil {
		t.Fatalf("PostClone() error = %v", err)
	}

	for _, dir := range []string{filepath.Join("var", "cache"), filepath.Join("var", "logs")} {
		info, err := os.Stat(filepath.Join(projectDir, dir))
		if err != nil {
			t.Fatalf("expected %s to be created: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}
}
