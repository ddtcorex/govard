package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"
)

func TestMageOSSupportsTablePrefix(t *testing.T) {
	if !engine.FrameworkSupportsTablePrefix("mageos") {
		t.Fatal("expected mageos to support table prefixes")
	}
}

func TestDetectMageOSTablePrefix(t *testing.T) {
	root := t.TempDir()
	etcDir := filepath.Join(root, "app", "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		t.Fatalf("mkdir app/etc: %v", err)
	}

	content := `<?php
return [
    'db' => [
        'table_prefix' => 'mos_',
        'connection' => [
            'default' => [
                'host' => 'db',
                'dbname' => 'mageos',
                'username' => 'mageos',
                'password' => 'mageos',
            ],
        ],
    ],
];
`
	if err := os.WriteFile(filepath.Join(etcDir, "env.php"), []byte(content), 0o644); err != nil {
		t.Fatalf("write env.php: %v", err)
	}

	if got := magento2.DetectTablePrefix(root); got != "mos_" {
		t.Fatalf("expected table prefix 'mos_' via Magento 2 capability, got %q", got)
	}

	if got := engine.DetectFrameworkTablePrefix(root, "mageos"); got != "mos_" {
		t.Fatalf("expected generic detector to return 'mos_' for mageos, got %q", got)
	}
}
