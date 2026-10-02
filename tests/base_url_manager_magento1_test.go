package tests

import (
	"strings"
	"testing"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks/magento1"
)

func TestBaseURLManagerMagento1KeepsPasswordOutOfArgv(t *testing.T) {
	var captured []string
	manager := &magento1.Magento1Manager{
		Executor: func(name string, args ...string) ([]byte, error) {
			captured = append(captured, args...)
			return nil, nil
		},
	}
	if err := manager.Update(t.TempDir(), engine.Config{ProjectName: "demo"}, "https://t.example"); err != nil {
		t.Fatalf("update: %v", err)
	}
	script := captured[len(captured)-1]
	pass := conventions.DefaultMagentoDBPass
	if !strings.Contains(script, "export MYSQL_PWD="+conventions.ShellQuote(pass)+"; ") {
		t.Fatalf("expected MYSQL_PWD export, got %q", script)
	}
	if strings.Contains(script, "-p"+pass) || strings.Contains(script, "-p"+conventions.ShellQuote(pass)) {
		t.Fatalf("password must not be a -p argument, got %q", script)
	}
	if !strings.Contains(script, "--no-defaults") {
		t.Fatalf("expected --no-defaults, got %q", script)
	}
}
