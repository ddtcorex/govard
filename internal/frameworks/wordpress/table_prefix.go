package wordpress

import (
	"os"
	"path/filepath"
	"regexp"

	"govard/internal/engine"
)

// tablePrefixExpr matches a literal `$table_prefix = '...';` assignment at the
// start of a line, with either quote style. The line anchor skips `//` and `#`
// commented copies. A dynamic value (getenv(), concatenation) deliberately does
// not match: guessing a prefix would filter the wrong tables silently.
var tablePrefixExpr = regexp.MustCompile(`(?m)^[ \t]*\$table_prefix[ \t]*=[ \t]*(?:'([^'\r\n]*)'|"([^"\r\n]*)")[ \t]*;`)

// DetectTablePrefix reads the literal `$table_prefix` from wp-config.php, in
// the project root or in the conventional `wordpress/` subdirectory.
func DetectTablePrefix(root string) string {
	for _, dir := range []string{root, filepath.Join(root, "wordpress")} {
		data, err := os.ReadFile(filepath.Join(dir, "wp-config.php"))
		if err != nil {
			continue
		}
		if prefix, ok := parseTablePrefix(string(data)); ok {
			return prefix
		}
	}
	return ""
}

// parseTablePrefix returns the last literal assignment (the one WordPress
// ends up with) and whether any assignment was found. Unsafe values are
// rejected as empty.
func parseTablePrefix(content string) (string, bool) {
	matches := tablePrefixExpr.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return "", false
	}
	last := matches[len(matches)-1]
	value := last[1]
	if value == "" {
		value = last[2]
	}
	return engine.SafeTablePrefix(value), true
}
