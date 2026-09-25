package magento2

import (
	"fmt"
	"strings"

	"govard/internal/engine"
)

// SandboxBaseURLStatements returns the UPDATEs that point a seeded database at
// the sandbox's own web URL: both base_url paths, every scope. A sandbox has
// exactly one web endpoint, so per-scope variance must all land on it; the
// derived `{{secure_base_url}}*` URLs resolve dynamically and need no change.
//
// The table prefix comes from the origin env.php content the seed already
// reads. An empty prefix is the common case and stays unprefixed; a prefix
// that cannot name a table yields no statements rather than corrupt SQL,
// because the value comes from the origin's file, which the seed does not
// control. The URL is quote-doubled for the same reason.
func SandboxBaseURLStatements(envContent []byte, baseURL string) []string {
	prefix := ""
	if matches := tablePrefixExpr.FindSubmatch(envContent); len(matches) == 2 {
		prefix = engine.NormalizeTablePrefix(string(matches[1]))
	}
	if !engine.ValidateTablePrefix(prefix) {
		return nil
	}
	table := prefix + "core_config_data"
	return []string{
		fmt.Sprintf("UPDATE %s SET value='%s' WHERE path IN ('web/unsecure/base_url','web/secure/base_url')",
			table, strings.ReplaceAll(baseURL, "'", "''")),
	}
}
