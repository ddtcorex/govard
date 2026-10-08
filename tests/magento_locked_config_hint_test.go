package tests

import (
	"strings"
	"testing"

	"govard/internal/frameworks/magento2"
)

func TestMagentoLockedConfigHintNamesPathAndFix(t *testing.T) {
	args := []string{"exec", "-u", "www-data", "proj-php-1", "bin/magento", "config:set",
		"web/seo/use_rewrites", "1", "--no-interaction"}
	out := "The value you set has already been locked. To change the value, use the --lock-env option for the config:set command."
	hint := magento2.LockedConfigHint("Enable Web Server Rewrites", args, out)
	for _, want := range []string{"web/seo/use_rewrites", "app/etc/config.php", "app:config:dump", "--lock-env"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint omits %q:\n%s", want, hint)
		}
	}
}

func TestMagentoLockedConfigHintNamesBaseURLPathsForStoreConfigSet(t *testing.T) {
	args := []string{"bin/magento", "setup:store-config:set", "--base-url=https://a/", "--no-interaction"}
	hint := magento2.LockedConfigHint("Setting Base URLs", args, "has already been locked")
	if !strings.Contains(hint, "web/unsecure/base_url") || !strings.Contains(hint, "web/secure/base_url") {
		t.Fatalf("hint omits base url paths:\n%s", hint)
	}
}

func TestMagentoLockedConfigHintIgnoresOtherFailures(t *testing.T) {
	if hint := magento2.LockedConfigHint("x", []string{"config:set", "a/b/c", "1"}, "boom"); hint != "" {
		t.Fatalf("unrelated failure got a hint: %q", hint)
	}
}
