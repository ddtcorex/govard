package magento1

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
)

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
