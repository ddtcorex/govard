package wordpress

import (
	"fmt"
	"os/exec"

	"govard/internal/conventions"
	"govard/internal/engine"
)

type WordPressManager struct {
	Executor func(name string, args ...string) ([]byte, error)
}

func (m *WordPressManager) Backup(projectRoot string, config engine.Config) error { return nil }
func (m *WordPressManager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	// wp option update siteurl <url> && wp option update home <url>
	_, err := m.executeWP(containerName, "option", "update", "siteurl", tunnelURL)
	if err != nil {
		return err
	}
	_, err = m.executeWP(containerName, "option", "update", "home", tunnelURL)
	return err
}
func (m *WordPressManager) Revert(projectRoot string, config engine.Config) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	localURL := fmt.Sprintf("https://%s", config.Domain)
	_, err := m.executeWP(containerName, "option", "update", "siteurl", localURL)
	if err != nil {
		return err
	}
	_, err = m.executeWP(containerName, "option", "update", "home", localURL)
	return err
}
func (m *WordPressManager) executeWP(container string, args ...string) ([]byte, error) {
	executor := m.Executor
	if executor == nil {
		executor = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	fullArgs := append([]string{"exec", "-u", conventions.UserWWWData, "-w", conventions.DefaultWorkDir, container, "wp", "--allow-root"}, args...)
	return executor("docker", fullArgs...)
}
