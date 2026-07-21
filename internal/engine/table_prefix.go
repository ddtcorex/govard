package engine

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	tablePrefixPattern           = regexp.MustCompile(`^[A-Za-z0-9_]*$`)
	magentoEnvTablePrefixExpr    = regexp.MustCompile(`(?i)['"]table_prefix['"]\s*=>\s*['"]([^'"]*)['"]`)
	prestashopEnvTablePrefixExpr = regexp.MustCompile(`(?i)['"]database_prefix['"]\s*=>\s*['"]([^'"]*)['"]`)
)

func NormalizeTablePrefix(prefix string) string {
	return strings.TrimSpace(prefix)
}

func ValidateTablePrefix(prefix string) bool {
	return tablePrefixPattern.MatchString(NormalizeTablePrefix(prefix))
}

func SafeTablePrefix(prefix string) string {
	normalized := NormalizeTablePrefix(prefix)
	if !ValidateTablePrefix(normalized) {
		return ""
	}
	return normalized
}

func FrameworkSupportsTablePrefix(framework string) bool {
	switch normalizeFrameworkManifestKey(framework) {
	case "magento2", "magento1", "openmage", "prestashop", "mageos":
		return true
	default:
		return false
	}
}

func DetectMagentoTablePrefix(root string, framework string) string {
	if !FrameworkSupportsTablePrefix(framework) {
		return ""
	}
	switch normalizeFrameworkManifestKey(framework) {
	case "magento2", "mageos":
		return DetectMagento2TablePrefix(root)
	case "magento1", "openmage":
		return DetectMagento1TablePrefix(root)
	case "prestashop":
		return DetectPrestaShopTablePrefix(root)
	default:
		return ""
	}
}

func DetectMagento2TablePrefix(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "app", "etc", "env.php"))
	if err != nil {
		return ""
	}
	matches := magentoEnvTablePrefixExpr.FindStringSubmatch(string(data))
	if len(matches) != 2 {
		return ""
	}
	return NormalizeTablePrefix(matches[1])
}

func DetectMagento1TablePrefix(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "app", "etc", "local.xml"))
	if err != nil {
		return ""
	}

	var localXML struct {
		Global struct {
			Resources struct {
				DB struct {
					TablePrefix string `xml:"table_prefix"`
				} `xml:"db"`
			} `xml:"resources"`
		} `xml:"global"`
	}
	if err := xml.Unmarshal(data, &localXML); err != nil {
		return ""
	}
	return NormalizeTablePrefix(localXML.Global.Resources.DB.TablePrefix)
}

func DetectPrestaShopTablePrefix(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "app", "config", "parameters.php"))
	if err != nil {
		return ""
	}
	matches := prestashopEnvTablePrefixExpr.FindStringSubmatch(string(data))
	if len(matches) != 2 {
		return ""
	}
	return NormalizeTablePrefix(matches[1])
}
