package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestPrestaShopSupportsTablePrefix(t *testing.T) {
	if !engine.FrameworkSupportsTablePrefix("prestashop") {
		t.Fatal("expected prestashop to support table prefixes")
	}
}

func TestDetectPrestaShopTablePrefix(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "app", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir app/config: %v", err)
	}

	content := `<?php

return array (
  'parameters' =>
  array (
    'database_host' => 'db',
    'database_name' => 'prestashop',
    'database_user' => 'prestashop',
    'database_password' => 'prestashop',
    'database_prefix' => 'shop_',
  ),
);
`
	if err := os.WriteFile(filepath.Join(configDir, "parameters.php"), []byte(content), 0o644); err != nil {
		t.Fatalf("write parameters.php: %v", err)
	}

	prefix := engine.DetectPrestaShopTablePrefix(root)
	if prefix != "shop_" {
		t.Fatalf("expected table prefix 'shop_', got %q", prefix)
	}

	if got := engine.DetectMagentoTablePrefix(root, "prestashop"); got != "shop_" {
		t.Fatalf("expected DetectMagentoTablePrefix to return 'shop_', got %q", got)
	}
}

func TestDetectPrestaShopTablePrefixMissingFile(t *testing.T) {
	root := t.TempDir()
	if prefix := engine.DetectPrestaShopTablePrefix(root); prefix != "" {
		t.Fatalf("expected empty prefix when parameters.php is missing, got %q", prefix)
	}
}
