package tunnel

import (
	"govard/internal/engine"
)

type BaseURLManager interface {
	Backup(projectRoot string, config engine.Config) error
	Update(projectRoot string, config engine.Config, tunnelURL string) error
	Revert(projectRoot string, config engine.Config) error
}

type NoopManager struct{}

func (m *NoopManager) Backup(projectRoot string, config engine.Config) error { return nil }
func (m *NoopManager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	return nil
}
func (m *NoopManager) Revert(projectRoot string, config engine.Config) error { return nil }
