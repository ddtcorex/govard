package magento2

import (
	"fmt"

	"govard/internal/conventions"
	"govard/internal/engine"
)

type Magento2Manager struct {
	Executor func(name string, args ...string) ([]byte, error)
}

func (m *Magento2Manager) Backup(projectRoot string, config engine.Config) error {
	return nil
}

func (m *Magento2Manager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	user := config.ResolveProjectExecUser(conventions.UserWWWData)

	// 1. Update both secure and unsecure URLs
	_ = m.executeMagento(containerName, user, "setup:store-config:set",
		"--base-url="+tunnelURL+"/", "--base-url-secure="+tunnelURL+"/", "--no-interaction")

	// 2. Disable redirect to base URL to prevent loop if Host header mismatch
	_ = m.executeMagento(containerName, user, "config:set", "web/url/redirect_to_base", "0")

	// 3. Ensure offloader header is set for Cloudflare/Proxy detection
	_ = m.executeMagento(containerName, user, "config:set", "web/secure/offloader_header", "X-Forwarded-Proto")

	// 4. Flush Redis if available
	m.flushRedis(config)

	// 6. Flush Magento cache
	return m.executeMagento(containerName, user, "cache:flush")
}

func (m *Magento2Manager) Revert(projectRoot string, config engine.Config) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	user := config.ResolveProjectExecUser(conventions.UserWWWData)
	localURL := fmt.Sprintf("https://%s/", config.Domain)

	// 1. Restore local URLs
	_ = m.executeMagento(containerName, user, "setup:store-config:set",
		"--base-url="+localURL, "--base-url-secure="+localURL, "--no-interaction")

	// 2. Restore redirect to base URL
	_ = m.executeMagento(containerName, user, "config:set", "web/url/redirect_to_base", "1")

	// 3. Flush Redis
	m.flushRedis(config)

	// 4. Flush Magento cache
	return m.executeMagento(containerName, user, "cache:flush")
}

func (m *Magento2Manager) executeMagento(container string, user string, magentoArgs ...string) error {
	executor := engine.ResolveDockerExecutor(m.Executor)
	args := append([]string{"exec", "-u", user, "-w", conventions.DefaultWorkDir, container, "bin/magento"}, magentoArgs...)
	_, err := executor("docker", args...)
	return err
}

func (m *Magento2Manager) flushRedis(config engine.Config) {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.RedisSuffix)
	executor := engine.ResolveDockerExecutor(m.Executor)
	// Best effort flush
	_, _ = executor("docker", "exec", containerName, "redis-cli", "flushall")
}
