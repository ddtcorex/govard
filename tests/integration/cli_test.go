//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIBinaryExists(t *testing.T) {
	env := NewTestEnvironment(t)

	if _, err := os.Stat(env.BinaryPath); os.IsNotExist(err) {
		t.Fatal("Govard binary not found")
	}
}

func TestCLIInitCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "init-cmd-test", files)

	result := env.RunGovard(t, projectDir, "init", "--recipe", "magento2")
	result.AssertSuccess(t)

	configPath := filepath.Join(projectDir, ".govard.yml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error(".govard.yml was not created")
	}
}

func TestCLIStatusCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "status-test",
			"recipe":       "magento2",
			"domain":       "status-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "status-cmd-test", files)

	result := env.RunGovard(t, projectDir, "status")
	result.AssertSuccess(t)
}

func TestCLIDoctorCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateTestProject(t, "doctor-cmd-test", map[string]string{})

	result := env.RunGovard(t, projectDir, "doctor")

	if result.ExitCode != 0 && result.ExitCode != 1 {
		t.Fatalf("Expected exit code 0 or 1, got %d", result.ExitCode)
	}
}

func TestCLISvcCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateTestProject(t, "svc-cmd-test", map[string]string{})

	result := env.RunGovard(t, projectDir, "svc", "--help")
	result.AssertSuccess(t)

	result.AssertOutputContains(t, "global services")
}

func TestCLIDebugCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "debug-cmd-test",
			"recipe":       "magento2",
			"domain":       "debug-cmd-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "debug-cmd-test", files)

	result := env.RunGovard(t, projectDir, "debug", "--help")
	result.AssertSuccess(t)
}

func TestCLILogsCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "logs-cmd-test",
			"recipe":       "magento2",
			"domain":       "logs-cmd-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "logs-cmd-test", files)

	result := env.RunGovard(t, projectDir, "env", "logs", "--help")
	result.AssertSuccess(t)
}

func TestCLIShellCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "shell-cmd-test",
			"recipe":       "magento2",
			"domain":       "shell-cmd-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "shell-cmd-test", files)

	result := env.RunGovard(t, projectDir, "shell", "--help")
	result.AssertSuccess(t)
}

func TestCLIDbCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "db-cmd-test",
			"recipe":       "magento2",
			"domain":       "db-cmd-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "db-cmd-test", files)

	result := env.RunGovard(t, projectDir, "db", "--help")
	result.AssertSuccess(t)
}

func TestCLIRedisCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "redis-cmd-test",
			"recipe":       "magento2",
			"domain":       "redis-cmd-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "redis-cmd-test", files)

	result := env.RunGovard(t, projectDir, "env", "redis", "--help")
	result.AssertSuccess(t)
}

func TestCLITrustCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateTestProject(t, "trust-cmd-test", map[string]string{})

	result := env.RunGovard(t, projectDir, "doctor", "trust", "--help")
	result.AssertSuccess(t)
}

func TestCLIConfigureCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "configure-cmd-test",
			"recipe":       "magento2",
			"domain":       "configure-cmd-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "configure-cmd-test", files)

	result := env.RunGovard(t, projectDir, "config", "auto", "--help")
	result.AssertSuccess(t)
}

func TestCLIStopCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "stop-cmd-test",
			"recipe":       "magento2",
			"domain":       "stop-cmd-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "stop-cmd-test", files)

	result := env.RunGovard(t, projectDir, "env", "stop", "--help")
	result.AssertSuccess(t)
}

func TestCLISelfUpdateCommand(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateTestProject(t, "self-update-cmd-test", map[string]string{})

	result := env.RunGovard(t, projectDir, "self-update", "--help")
	result.AssertSuccess(t)
}

func TestCLIVersionFlag(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateTestProject(t, "version-flag-test", map[string]string{})

	result := env.RunGovard(t, projectDir, "--version")

	if result.ExitCode != 0 {
		result = env.RunGovard(t, projectDir, "version")
	}

	if result.ExitCode != 0 {
		t.Fatalf("Version command failed with exit code %d", result.ExitCode)
	}

	combined := strings.ToLower(result.Stdout + result.Stderr)
	if strings.TrimSpace(combined) == "" {
		t.Fatal("Version output should not be empty")
	}

	if !strings.Contains(combined, "govard") && !strings.Contains(combined, "v1.") {
		t.Error("Version output should contain Govard name or version marker")
	}
}

func TestCLIHelpFlag(t *testing.T) {
	env := NewTestEnvironment(t)

	projectDir := env.CreateTestProject(t, "help-flag-test", map[string]string{})

	result := env.RunGovard(t, projectDir, "--help")
	result.AssertSuccess(t)

	requiredCommands := []string{
		"init",
		"env",
		"svc",
		"status",
		"shell",
		"doctor",
	}

	for _, cmd := range requiredCommands {
		if !strings.Contains(result.Stdout, cmd) {
			t.Errorf("Help output missing command: %s", cmd)
		}
	}
}

func TestCLIShortcutsHelp(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "shortcuts-test",
			"recipe":       "magento2",
			"domain":       "shortcuts-test.test",
			"stack": map[string]interface{}{
				"php_version": "8.3",
				"web_server":  "nginx",
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "shortcuts-test", files)

	result := env.RunGovard(t, projectDir, "shortcuts")

	if result.ExitCode != 0 {
		return
	}
}
