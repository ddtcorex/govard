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
	// Symfony often uses APP_URL or similar, but frequently SITE_URL
	// Let's support both or just APP_URL for consistency if it's there
	return m.UpdateEnv(projectRoot, "APP_URL", tunnelURL)
}
func (m *SymfonyManager) Revert(projectRoot string, config engine.Config) error {
	return m.UpdateEnv(projectRoot, "APP_URL", fmt.Sprintf("https://%s", config.Domain))
}
