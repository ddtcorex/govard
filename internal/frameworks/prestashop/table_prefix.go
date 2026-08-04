package prestashop

import (
	"os"
	"path/filepath"
	"regexp"

	"govard/internal/engine"
)

var tablePrefixExpr = regexp.MustCompile(`(?i)['"]database_prefix['"]\s*=>\s*['"]([^'"]*)['"]`)

// DetectTablePrefix reads PrestaShop's parameters.php database-prefix setting.
func DetectTablePrefix(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "app", "config", "parameters.php"))
	if err != nil {
		return ""
	}
	matches := tablePrefixExpr.FindStringSubmatch(string(data))
	if len(matches) != 2 {
		return ""
	}
	return engine.NormalizeTablePrefix(matches[1])
}
