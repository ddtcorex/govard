package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
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
	executor := m.Executor
	if executor == nil {
		executor = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	args := append([]string{"exec", "-u", user, "-w", conventions.DefaultWorkDir, container, "bin/magento"}, magentoArgs...)
	_, err := executor("docker", args...)
	return err
}

func (m *Magento2Manager) flushRedis(config engine.Config) {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.RedisSuffix)
	executor := m.Executor
	if executor == nil {
		executor = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	// Best effort flush
	_, _ = executor("docker", "exec", containerName, "redis-cli", "flushall")
}

type LaravelManager struct {
	ReadFile  func(string) ([]byte, error)
	WriteFile func(string, []byte, os.FileMode) error
}

func (m *LaravelManager) Backup(projectRoot string, config engine.Config) error {
	return nil
}

func (m *LaravelManager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	return m.updateEnv(projectRoot, "APP_URL", tunnelURL)
}

func (m *LaravelManager) Revert(projectRoot string, config engine.Config) error {
	return m.updateEnv(projectRoot, "APP_URL", fmt.Sprintf("https://%s", config.Domain))
}

func (m *LaravelManager) updateEnv(projectRoot string, key string, value string) error {
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

// Similar structs for other frameworks...

type Magento1Manager struct {
	Executor func(name string, args ...string) ([]byte, error)
}

func (m *Magento1Manager) getPrefix(projectRoot string) string {
	xmlPath := filepath.Join(projectRoot, "app", "etc", "local.xml")
	content, err := os.ReadFile(xmlPath)
	if err != nil {
		return ""
	}
	s := string(content)
	startTag := "<table_prefix><![CDATA["
	endTag := "]]></table_prefix>"

	startIdx := strings.Index(s, startTag)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(startTag)

	endIdx := strings.Index(s[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}

	return s[startIdx : startIdx+endIdx]
}

func (m *Magento1Manager) Backup(projectRoot string, config engine.Config) error { return nil }
func (m *Magento1Manager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	prefix := m.getPrefix(projectRoot)
	if prefix == "" {
		prefix = config.TablePrefix
	}

	// 1. Update URLs
	sql := fmt.Sprintf("UPDATE %score_config_data SET value='%s/' WHERE path IN ('web/unsecure/base_url', 'web/secure/base_url')", prefix, tunnelURL)
	_, _ = m.executeDockerMysql(containerName, sql)

	// 2. Disable redirect to base URL to handle tunnel domain mismatch
	sql = fmt.Sprintf("INSERT INTO %score_config_data (scope, scope_id, path, value) VALUES ('default', 0, 'web/url/redirect_to_base', '0') ON DUPLICATE KEY UPDATE value='0'", prefix)
	_, err := m.executeDockerMysql(containerName, sql)
	return err
}
func (m *Magento1Manager) Revert(projectRoot string, config engine.Config) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localURL := fmt.Sprintf("https://%s/", config.Domain)
	prefix := m.getPrefix(projectRoot)
	if prefix == "" {
		prefix = config.TablePrefix
	}

	// 1. Restore local URLs
	sql := fmt.Sprintf("UPDATE %score_config_data SET value='%s' WHERE path IN ('web/unsecure/base_url', 'web/secure/base_url')", prefix, localURL)
	_, _ = m.executeDockerMysql(containerName, sql)

	// 2. Restore redirect to base URL
	sql = fmt.Sprintf("UPDATE %score_config_data SET value='1' WHERE path='web/url/redirect_to_base'", prefix)
	_, err := m.executeDockerMysql(containerName, sql)
	return err
}
func (m *Magento1Manager) executeDockerMysql(container string, sql string) ([]byte, error) {
	executor := m.Executor
	if executor == nil {
		executor = func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		}
	}
	// Use smart detection for mysql vs mariadb binary
	script := fmt.Sprintf(
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else exit 1; fi && "$DB_CLI" -u%s -p%s %s -e %s`,
		conventions.DefaultMagentoDBUser,
		conventions.DefaultMagentoDBPass,
		conventions.DefaultMagentoDBName,
		conventions.ShellQuote(sql),
	)

	return executor("docker", "exec", container, "sh", "-lc", script)
}

type WordPressManager struct {
	Executor func(name string, args ...string) ([]byte, error)
}

func (m *WordPressManager) Backup(projectRoot string, config engine.Config) error { return nil }
func (m *WordPressManager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPFPMSuffix)
	// wp option update siteurl <url> && wp option update home <url>
	_, err := m.executeWP(containerName, "option", "update", "siteurl", tunnelURL)
	if err != nil {
		return err
	}
	_, err = m.executeWP(containerName, "option", "update", "home", tunnelURL)
	return err
}
func (m *WordPressManager) Revert(projectRoot string, config engine.Config) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPFPMSuffix)
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
	fullArgs := append([]string{"exec", "-u", conventions.UserWWWData, container, "wp"}, args...)
	return executor("docker", fullArgs...)
}

type SymfonyManager struct {
	LaravelManager
}

func (m *SymfonyManager) Update(projectRoot string, config engine.Config, tunnelURL string) error {
	// Symfony often uses APP_URL or similar, but frequently SITE_URL
	// Let's support both or just APP_URL for consistency if it's there
	return m.updateEnv(projectRoot, "APP_URL", tunnelURL)
}
func (m *SymfonyManager) Revert(projectRoot string, config engine.Config) error {
	return m.updateEnv(projectRoot, "APP_URL", fmt.Sprintf("https://%s", config.Domain))
}
