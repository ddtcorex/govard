package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/shopware"
)

func TestShopwareInstallSyncsDomainAwareURLs(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "bin", "console"), []byte("#!/usr/bin/env php\n"), 0o755); err != nil {
		t.Fatalf("write console stub: %v", err)
	}

	commands := make([]string, 0, 4)
	shopwareBootstrap := shopware.NewShopwareBootstrap(bootstrap.Options{
		Runner: func(command string) error {
			commands = append(commands, command)
			return nil
		},
		DBHost: "db",
		DBUser: "shopware",
		DBPass: "shopware",
		DBName: "shopware",
		Domain: "sample.test",
	})

	if err := shopwareBootstrap.Install(projectDir); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	envContentBytes, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	envContent := string(envContentBytes)
	if !strings.Contains(envContent, "APP_URL=https://sample.test") {
		t.Fatalf("expected APP_URL to be domain-aware, got:\n%s", envContent)
	}
	if !strings.Contains(envContent, "PROXY_URL=https://sample.test") {
		t.Fatalf("expected PROXY_URL to be domain-aware, got:\n%s", envContent)
	}
	if !strings.Contains(envContent, "DATABASE_URL=mysql://shopware:shopware@db:3306/shopware") {
		t.Fatalf("expected DATABASE_URL to be rewritten, got:\n%s", envContent)
	}

	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "sales-channel:replace:url") || !strings.Contains(joined, "https://sample.test") {
		t.Fatalf("expected sales channel URL sync command, got:\n%s", joined)
	}
}
