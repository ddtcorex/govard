package magento2

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"govard/internal/engine"
)

var tablePrefixExpr = regexp.MustCompile(`(?i)['"]table_prefix['"]\s*=>\s*['"]([^'"]*)['"]`)

// DetectTablePrefix reads Magento 2-compatible env.php configuration.
// Mage-OS inherits this detector through its resolved Magento 2 definition.
func DetectTablePrefix(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "app", "etc", "env.php"))
	if err != nil {
		return ""
	}
	matches := tablePrefixExpr.FindStringSubmatch(string(data))
	if len(matches) != 2 {
		return ""
	}
	return engine.NormalizeTablePrefix(matches[1])
}

func ResolveBootstrapTablePrefix(configuredPrefix string) (string, error) {
	prefix := engine.NormalizeTablePrefix(configuredPrefix)
	if prefix == "" {
		prefix = engine.NormalizeTablePrefix(os.Getenv("TABLE_PREFIX"))
	}
	if !engine.ValidateTablePrefix(prefix) {
		return "", fmt.Errorf("invalid table prefix %q (allowed: letters, numbers, and underscore)", prefix)
	}
	return prefix, nil
}
