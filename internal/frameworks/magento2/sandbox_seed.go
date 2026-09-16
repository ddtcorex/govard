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

// RewriteMagentoEnvForSandbox rewrites app/etc/env.php for a derived sandbox:
// every mapped key is replaced wherever it appears, and every service host
// that is not loopback-local becomes 127.0.0.1 (a single-container sandbox
// runs MariaDB and Redis on loopback; a compose-network hostname left over
// fails DNS at the first bin/magento call). Anything else passes through
// byte-identical. Mapped keys the file does not have are reported skipped:
// inventing new array structure textually is where corruption lives, so the
// caller prints the skip and the database keeps whatever value it has. An
// empty mapping still localizes hosts.
func RewriteMagentoEnvForSandbox(content []byte, mapping map[string]string) ([]byte, []string, error) {
	out := localizeServiceHosts(string(content))
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

// loopbackHosts are the host values a sandbox keeps: everything else is a
// compose-network (or remote) name that cannot resolve inside the sandbox.
var loopbackHosts = map[string]bool{"127.0.0.1": true, "localhost": true, "::1": true}

// localizeServiceHosts points every 'host'/'server' value at loopback unless
// it already is one. Ports, dbnames and credentials pass through untouched.
func localizeServiceHosts(content string) string {
	for _, key := range []string{"host", "server"} {
		re := singleQuotedValue(key)
		out := re.ReplaceAllStringFunc(content, func(match string) string {
			value := singleQuotedValue(key).FindStringSubmatch(match)[2]
			if loopbackHosts[value] {
				return match
			}
			return strings.Replace(match, "'"+value+"'", "'127.0.0.1'", 1)
		})
		content = out
	}
	return content
}
