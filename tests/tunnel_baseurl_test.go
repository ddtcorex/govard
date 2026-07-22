package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks"
)

func TestMagento2ManagerUpdate(t *testing.T) {
	var executedCommands []string
	mgr := &tunnel.Magento2Manager{
		Executor: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && args[0] == "exec" {
				executedCommands = append(executedCommands, strings.Join(args, " "))
			}
			return []byte(""), nil
		},
	}

	config := engine.Config{ProjectName: "demo", Domain: "demo.test"}
	err := mgr.Update(".", config, "https://tunnel.live")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have multiple commands: store-config:set, config:set (redirect), config:set (offloader), cache:flush
	foundStoreConfig := false
	for _, cmd := range executedCommands {
		if strings.Contains(cmd, "setup:store-config:set") && strings.Contains(cmd, "--base-url=https://tunnel.live/") {
			foundStoreConfig = true
			break
		}
	}

	if !foundStoreConfig {
		t.Fatalf("expected setup:store-config:set command, but not found in: %v", executedCommands)
	}
}

func TestLaravelManagerUpdate(t *testing.T) {
	envContent := "APP_NAME=Laravel\nAPP_URL=http://localhost\nDB_CONNECTION=mysql"
	var writtenContent string

	mgr := &tunnel.LaravelManager{
		ReadFile: func(path string) ([]byte, error) {
			return []byte(envContent), nil
		},
		WriteFile: func(path string, data []byte, mode os.FileMode) error {
			writtenContent = string(data)
			return nil
		},
	}

	config := engine.Config{Domain: "laravel.test"}
	err := mgr.Update(".", config, "https://tunnel.live")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(writtenContent, "APP_URL=https://tunnel.live") {
		t.Fatalf("expected APP_URL update, got:\n%s", writtenContent)
	}
}

func TestMagento1ManagerUpdate(t *testing.T) {
	var executedSQL string
	mgr := &tunnel.Magento1Manager{
		Executor: func(name string, args ...string) ([]byte, error) {
			if name == "docker" && args[0] == "exec" {
				executedSQL = args[len(args)-1]
			}
			return []byte(""), nil
		},
	}

	config := engine.Config{ProjectName: "demo", Domain: "demo.test"}
	// Update uses getPrefix which reads local.xml, but for this test we can mock it or just rely on empty prefix
	err := mgr.Update(".", config, "https://tunnel.live")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain the UPSERT logic for redirect_to_base
	if !strings.Contains(executedSQL, "web/url/redirect_to_base") {
		t.Fatalf("expected redirect_to_base update in SQL, got: %s", executedSQL)
	}
	// Should contain the mysql/mariadb detection script
	if !strings.Contains(executedSQL, "command -v mysql") {
		t.Fatalf("expected DB CLI detection script, got: %s", executedSQL)
	}
}

func TestBaseURLManagerFactory(t *testing.T) {
	tests := []struct {
		framework string
		expected  string
	}{
		{"magento2", "*tunnel.Magento2Manager"},
		{"mageos", "*tunnel.Magento2Manager"},
		{"magento1", "*tunnel.Magento1Manager"},
		{"openmage", "*tunnel.Magento1Manager"},
		{"Laravel", "*tunnel.LaravelManager"},
		{"wordpress", "*tunnel.WordPressManager"},
		{"Symfony", "*tunnel.SymfonyManager"},
		{"Unknown", "*tunnel.NoopManager"},
	}

	for _, tt := range tests {
		mgr := frameworks.NewBaseURLManager(tt.framework)
		typeName := fmt.Sprintf("%T", mgr)
		if typeName != tt.expected {
			t.Errorf("framework %s: expected type %s, got %s", tt.framework, tt.expected, typeName)
		}
	}
}
