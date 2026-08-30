package laravel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
)

type LaravelManager struct {
	ReadFile  func(string) ([]byte, error)
	WriteFile func(string, []byte, os.FileMode) error
}

func (m *LaravelManager) Backup(projectRoot string, config engine.Config) error {
	return nil
}

func (m *LaravelManager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	return m.UpdateEnv(projectRoot, "APP_URL", tunnelURL)
}

func (m *LaravelManager) Revert(projectRoot string, config engine.Config) error {
	return m.UpdateEnv(projectRoot, "APP_URL", fmt.Sprintf("https://%s", config.Domain))
}

// UpdateEnv is exported so symfony.SymfonyManager can reach it through
// embedding - Go does not promote unexported methods across packages.
func (m *LaravelManager) UpdateEnv(projectRoot string, key string, value string) error {
	return m.UpdateEnvFile(projectRoot, ".env", key, value)
}

// UpdateEnvFile updates a specific env file (e.g. ".env" or ".env.local") for key=value.
// Exported for Symfony which stores runtime env in .env.local.
func (m *LaravelManager) UpdateEnvFile(projectRoot string, envFile string, key string, value string) error {
	envPath := filepath.Join(projectRoot, envFile)
	read := m.ReadFile
	if read == nil {
		read = os.ReadFile
	}
	content, err := read(envPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comments and empty lines; match key= prefix exactly.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, key+"=") {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	write := m.WriteFile
	if write == nil {
		write = os.WriteFile
	}
	return write(envPath, []byte(strings.Join(lines, "\n")), conventions.DefaultFilePerm)
}
