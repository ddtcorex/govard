package magento2

import (
	"maps"
	"regexp"
	"slices"
	"strings"
)

// singleQuotedValue matches one single-quoted key's single-quoted value:
// `'base_url' => 'https://shop.test/'`. The replacement keeps the key and the
// quoting and swaps only the value, so every unrelated line stays
// byte-identical. Double-quoted or Heredoc values are left alone on purpose:
// guessing at grammars the project may not use corrupts files.
func singleQuotedValue(key string) *regexp.Regexp {
	return regexp.MustCompile(`('` + regexp.QuoteMeta(key) + `'\s*=>\s*')([^']*)(')`)
}

// RewriteMagentoEnvForSandbox rewrites app/etc/env.php for a derived sandbox.
// Every key present in mapping is replaced everywhere it appears (base_url
// occurs twice: secure and unsecure); anything unmapped passes through
// byte-identical. Mapped keys the file does not have are reported skipped:
// inventing new array structure textually is where corruption lives, so the
// caller prints the skip and the database keeps whatever base_url it has. An
// empty mapping is a copy.
func RewriteMagentoEnvForSandbox(content []byte, mapping map[string]string) ([]byte, []string, error) {
	if len(mapping) == 0 {
		out := make([]byte, len(content))
		copy(out, content)
		return out, nil, nil
	}
	out := string(content)
	var skipped []string
	for _, key := range slices.Sorted(maps.Keys(mapping)) {
		value := mapping[key]
		re := singleQuotedValue(key)
		if !re.MatchString(out) {
			skipped = append(skipped, key)
			continue
		}
		// The value is replacement text, where `$` starts a group reference:
		// double it so a `$` in the value survives verbatim.
		out = re.ReplaceAllString(out, "${1}"+strings.ReplaceAll(value, "$", "$$")+"${3}")
	}
	return []byte(out), skipped, nil
}
