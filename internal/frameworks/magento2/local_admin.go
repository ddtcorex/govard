package magento2

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"govard/internal/conventions"
)

var (
	frontNamePattern   = regexp.MustCompile(`(?i)['"]frontName['"]\s*=>\s*['"]([^'"]+)['"]`)
	tablePrefixPattern = regexp.MustCompile(`(?i)['"]table_prefix['"]\s*=>\s*['"]([^'"]*)['"]`)
)

// DetectLocalAdminMetadata reads Magento's env.php backend front name and
// table prefix. Missing/unreadable configuration has no metadata.
func DetectLocalAdminMetadata(projectRoot string) (string, string) {
	content, err := os.ReadFile(filepath.Join(projectRoot, "app", "etc", "env.php"))
	if err != nil {
		return "", ""
	}

	raw := string(content)
	frontName := ""
	tablePrefix := ""
	if match := frontNamePattern.FindStringSubmatch(raw); len(match) == 2 {
		frontName = strings.Trim(strings.TrimSpace(match[1]), "/")
	}
	if match := tablePrefixPattern.FindStringSubmatch(raw); len(match) == 2 {
		tablePrefix = strings.TrimSpace(match[1])
	}
	return frontName, tablePrefix
}

// BuildLocalAdminSettingsQuery returns Magento's core_config_data query for
// locally configured admin URL overrides.
func BuildLocalAdminSettingsQuery(tablePrefix string) string {
	return "SELECT path, value FROM " + tablePrefix + "core_config_data" +
		" WHERE path IN ('admin/url/use_custom','admin/url/use_custom_path','admin/url/custom','admin/url/custom_path')"
}

// ResolveLocalAdminURL applies Magento's env.php and core_config_data admin
// routing rules to a local base URL.
func ResolveLocalAdminURL(baseURL string, envFrontName string, dbValues map[string]string) string {
	frontName := strings.Trim(strings.TrimSpace(envFrontName), "/")
	if frontName == "" {
		frontName = conventions.DefaultAdminPath
	}

	if truthyConfig(dbValues["admin/url/use_custom_path"]) {
		if customPath := normalizeAdminTarget(dbValues["admin/url/custom_path"]); customPath != "" {
			if isURLTarget(customPath) {
				return customPath
			}
			return joinURLWithPath(baseURL, customPath)
		}
	}

	if truthyConfig(dbValues["admin/url/use_custom"]) {
		if custom := normalizeAdminTarget(dbValues["admin/url/custom"]); custom != "" {
			if isURLTarget(custom) {
				return custom
			}
			return joinURLWithPath(baseURL, custom)
		}
	}

	return joinURLWithPath(baseURL, frontName)
}

func truthyConfig(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeAdminTarget(raw string) string { return strings.Trim(strings.TrimSpace(raw), "/") }

func isURLTarget(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func joinURLWithPath(baseURL string, path string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	trimmedPath := strings.Trim(strings.TrimSpace(path), "/")
	if trimmedPath == "" {
		return base
	}
	return base + "/" + trimmedPath
}
