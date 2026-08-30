package symfony

import (
	"fmt"

	"govard/internal/engine"
	"govard/internal/frameworks/laravel"
)

type SymfonyManager struct {
	laravel.LaravelManager
}

func (m *SymfonyManager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	// Symfony's primary env file is .env.local (created by bootstrap with DATABASE_URL).
	// Update .env.local first, then .env as fallback so tunnel works regardless of
	// which file the project uses for APP_URL. Laravel's .env handling stays
	// unchanged — this only affects Symfony.
	if err := m.UpdateEnvFile(projectRoot, ".env.local", "APP_URL", tunnelURL); err == nil {
		_ = m.UpdateEnv(projectRoot, "APP_URL", tunnelURL)
		return nil
	}
	return m.UpdateEnv(projectRoot, "APP_URL", tunnelURL)
}
func (m *SymfonyManager) Revert(projectRoot string, config engine.Config) error {
	localURL := fmt.Sprintf("https://%s", config.Domain)
	if err := m.UpdateEnvFile(projectRoot, ".env.local", "APP_URL", localURL); err == nil {
		_ = m.UpdateEnv(projectRoot, "APP_URL", localURL)
		return nil
	}
	return m.UpdateEnv(projectRoot, "APP_URL", localURL)
}
