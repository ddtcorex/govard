package magento2

import (
	"fmt"
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
// byte-identical. An empty mapping is a copy.
func RewriteMagentoEnvForSandbox(content []byte, mapping map[string]string) ([]byte, error) {
	if len(mapping) == 0 {
		out := make([]byte, len(content))
		copy(out, content)
		return out, nil
	}
	out := string(content)
	for _, key := range slices.Sorted(maps.Keys(mapping)) {
		value := mapping[key]
		re := singleQuotedValue(key)
		if !re.MatchString(out) {
			return nil, fmt.Errorf("cannot rewrite env.php: key %q not found as a single-quoted value", key)
		}
		// The value is replacement text, where `$` starts a group reference:
		// double it so a `$` in the value survives verbatim.
		out = re.ReplaceAllString(out, "${1}"+strings.ReplaceAll(value, "$", "$$")+"${3}")
	}
	return []byte(out), nil
}
