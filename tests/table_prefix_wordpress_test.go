package tests

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/wordpress"
)

func writeWPConfig(t *testing.T, dir string, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wp-config.php"), []byte(body), 0o644); err != nil {
		t.Fatalf("write wp-config.php: %v", err)
	}
}

func TestWordPressSupportsTablePrefix(t *testing.T) {
	if !engine.FrameworkSupportsTablePrefix("wordpress") {
		t.Fatal("expected wordpress to register a table prefix detector")
	}
}

func TestDetectWordPressTablePrefix(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"single quotes", "<?php\n$table_prefix = 'wp_';\n", "wp_"},
		{"double quotes", "<?php\n$table_prefix = \"site_\";\n", "site_"},
		{"no spaces", "<?php\n$table_prefix='x9_';\n", "x9_"},
		{"tabs and extra spaces", "<?php\n$table_prefix \t=   'tab_' ;\n", "tab_"},
		{"trailing comment", "<?php\n$table_prefix = 'c_'; // the prefix\n", "c_"},
		{"commented out line is ignored", "<?php\n// $table_prefix = 'old_';\n# $table_prefix = 'older_';\n$table_prefix = 'real_';\n", "real_"},
		{"last assignment wins", "<?php\n$table_prefix = 'first_';\n$table_prefix = 'second_';\n", "second_"},
		{"empty prefix", "<?php\n$table_prefix = '';\n", ""},
		{"dynamic value is not guessed", "<?php\n$table_prefix = getenv('WP_PREFIX') ?: 'wp_';\n", ""},
		{"invalid characters rejected", "<?php\n$table_prefix = 'a-b;drop';\n", ""},
		{"unrelated variable", "<?php\n$my_table_prefix = 'nope_';\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeWPConfig(t, root, tc.body)
			if got := wordpress.DetectTablePrefix(root); got != tc.want {
				t.Fatalf("DetectTablePrefix = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectWordPressTablePrefixSubdirAndMissing(t *testing.T) {
	root := t.TempDir()
	if got := wordpress.DetectTablePrefix(root); got != "" {
		t.Fatalf("missing wp-config.php: got %q", got)
	}
	writeWPConfig(t, filepath.Join(root, "wordpress"), "<?php\n$table_prefix = 'sub_';\n")
	if got := engine.DetectFrameworkTablePrefix(root, "wordpress"); got != "sub_" {
		t.Fatalf("generic detector in wordpress/ subdir = %q, want sub_", got)
	}
}

func TestWordPressRemotePayloadCarriesTablePrefix(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{
		"host": "db", "username": "u", "password": "p", "dbname": "d", "table_prefix": "wpx_",
	})
	env, err := wordpress.DecodeEnvironmentPayloadForTest(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.DB.TablePrefix != "wpx_" {
		t.Fatalf("TablePrefix = %q, want wpx_", env.DB.TablePrefix)
	}

	raw, _ = json.Marshal(map[string]string{
		"host": "db", "username": "u", "dbname": "d", "table_prefix": "bad;prefix",
	})
	env, err = wordpress.DecodeEnvironmentPayloadForTest(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.DB.TablePrefix != "" {
		t.Fatalf("unsafe prefix must be dropped, got %q", env.DB.TablePrefix)
	}
}

func TestWordPressRemoteProbeScriptReadsTablePrefix(t *testing.T) {
	if !strings.Contains(wordpress.DBProbePHPForTest(), "table_prefix") {
		t.Fatal("remote wp-config.php probe must read $table_prefix")
	}
}

// The prefix used for a remote dump must come from the remote itself. The
// configured prefix is detected from the LOCAL wp-config.php, so applying it to a
// remote whose own prefix could not be read would filter the wrong tables and
// leave user data in a dump that looks sanitized.
func TestWordPressNeverGuessesARemotePrefixFromTheLocalConfig(t *testing.T) {
	def, ok := frameworks.Get("wordpress")
	if !ok {
		t.Fatal("wordpress definition missing")
	}
	if def.RemoteDBUsesConfigTablePrefix {
		t.Fatal("wordpress must not use the local/configured prefix for a remote whose prefix is unknown")
	}
	if got := cmd.RemoteFallbackTablePrefixForTest("wordpress", "wp_"); got != "" {
		t.Fatalf("a failed remote probe must not fall back to the local prefix, got %q", got)
	}
	if got := cmd.RemoteFallbackTablePrefixForTest("magento2", "mg_"); got != "mg_" {
		t.Fatalf("magento2 keeps its configured-prefix fallback, got %q", got)
	}
}

func TestWordPressRemoteDumpFiltersPrefixedPrivacyTables(t *testing.T) {
	args := cmd.BuildIgnoredTableArgsForTest("wpdb", "wp_", false, true, "wordpress")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--ignore-table=wpdb.wp_users", "--ignore-table=wpdb.wp_usermeta", "--ignore-table=wpdb.wp_comments"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
}

func TestPrivacyFilterWarning(t *testing.T) {
	if got := cmd.PrivacyFilterWarningForTest("wordpress", "", false, false); got != "" {
		t.Fatalf("no flags, no warning, got %q", got)
	}
	if got := cmd.PrivacyFilterWarningForTest("wordpress", "wp_", true, true); got != "" {
		t.Fatalf("known prefix, no warning, got %q", got)
	}
	got := cmd.PrivacyFilterWarningForTest("wordpress", "", false, true)
	if !strings.Contains(got, "table prefix") || !strings.Contains(got, "--no-pii") {
		t.Fatalf("empty prefix on a prefixed framework must warn, got %q", got)
	}
	if got := cmd.PrivacyFilterWarningForTest("magento2", "", false, true); got != "" {
		t.Fatalf("magento2 is legitimately unprefixed, got %q", got)
	}
	if got := cmd.PrivacyFilterWarningForTest("custom", "", true, false); !strings.Contains(got, "nothing") {
		t.Fatalf("framework with no table list must warn that nothing is filtered, got %q", got)
	}
}

func TestPrivacyFilterRefusesAnUnfilteredPIIDump(t *testing.T) {
	// --no-pii that cannot match anything must stop the command, not warn.
	if err := cmd.CheckPrivacyFilterForTest("wordpress", "", false, true, false); err == nil {
		t.Fatal("--no-pii with an unknown WordPress prefix must be refused")
	}
	if err := cmd.CheckPrivacyFilterForTest("custom", "", false, true, false); err == nil {
		t.Fatal("--no-pii on a framework with no PII table list must be refused")
	}
	// A plan only describes; it may warn but must not fail.
	if err := cmd.CheckPrivacyFilterForTest("wordpress", "", false, true, true); err != nil {
		t.Fatalf("a plan must not fail, got %v", err)
	}
	// --no-noise is not personal data: warn only.
	if err := cmd.CheckPrivacyFilterForTest("wordpress", "", true, false, false); err != nil {
		t.Fatalf("--no-noise alone must only warn, got %v", err)
	}
	// A known prefix works, and magento2 is legitimately unprefixed.
	if err := cmd.CheckPrivacyFilterForTest("wordpress", "wp_", true, true, false); err != nil {
		t.Fatalf("known prefix must pass, got %v", err)
	}
	if err := cmd.CheckPrivacyFilterForTest("magento2", "", true, true, false); err != nil {
		t.Fatalf("magento2 must pass, got %v", err)
	}
	// No privacy flag, no check.
	if err := cmd.CheckPrivacyFilterForTest("wordpress", "", false, false, false); err != nil {
		t.Fatalf("no flags must pass, got %v", err)
	}
}
