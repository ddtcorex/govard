package tests

import (
	"strings"
	"testing"

	"govard/internal/frameworks/magento2"
)

// The seed restores the origin's rows verbatim, so a seeded sandbox keeps the
// origin's base_url and answers the verify check with a redirect to the origin
// domain — found on a real Magento 2 rehearsal. The seed's post-import rewrite
// has to point every scope of both base_url paths at the sandbox's own web
// URL: a sandbox has exactly one web endpoint, so per-scope variance must all
// land on it, and the derived `{{secure_base_url}}*` URLs resolve dynamically
// and need no change.
func TestSandboxBaseURLStatementsPointEveryScopeAtTheSandbox(t *testing.T) {
	env := []byte(`<?php return ['db' => ['table_prefix' => 'mg_']];` + "\n")
	got := magento2.SandboxBaseURLStatements(env, "http://127.0.0.1:32768/")
	if len(got) != 1 {
		t.Fatalf("statements = %v, want the one base_url update", got)
	}
	for _, want := range []string{
		"UPDATE mg_core_config_data",
		"value='http://127.0.0.1:32768/'",
		"'web/unsecure/base_url'",
		"'web/secure/base_url'",
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("the statement must contain %q, got: %s", want, got[0])
		}
	}
}

func TestSandboxBaseURLStatementsWithoutAPrefix(t *testing.T) {
	env := []byte("<?php return [];\n")
	got := magento2.SandboxBaseURLStatements(env, "http://127.0.0.1:32768/")
	if len(got) != 1 || !strings.Contains(got[0], "UPDATE core_config_data") {
		t.Fatalf("statements = %v, want the unprefixed update", got)
	}
}

// A prefix that cannot name a table must yield nothing, not corrupt SQL: the
// value comes from the origin's file, which the seed does not control.
func TestSandboxBaseURLStatementsRefuseAnInvalidPrefix(t *testing.T) {
	env := []byte("<?php return ['db' => ['table_prefix' => 'mg-; DROP']];\n")
	if got := magento2.SandboxBaseURLStatements(env, "http://127.0.0.1:32768/"); len(got) != 0 {
		t.Fatalf("statements = %v, want nothing rather than corrupt SQL", got)
	}
}

func TestSandboxBaseURLStatementsEscapeTheURL(t *testing.T) {
	env := []byte("<?php return [];\n")
	got := magento2.SandboxBaseURLStatements(env, "http://127.0.0.1:32768/o'clock/")
	if len(got) != 1 || !strings.Contains(got[0], "o''clock") {
		t.Fatalf("statements = %v, want the quote doubled", got)
	}
}
