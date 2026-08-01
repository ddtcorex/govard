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
	envPath := filepath.Join(projectRoot, ".env")
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
		if strings.HasPrefix(line, key+"=") {
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
