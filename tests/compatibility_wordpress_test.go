package tests

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/engine"
	wordpress "govard/internal/frameworks/wordpress"

	"github.com/pterm/pterm"
)

func TestParseWPCliVersion(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"WP-CLI 2.8.1", "2.8.1"},
		{"WP-CLI 2.10.0", "2.10.0"},
		{"WP-CLI 1.4.0", "1.4.0"},
		{"", ""},
		{"WP-CLI x.y.z", ""},
		{"no version here", ""},
		{"php warning noise\nWP-CLI 2.8.1\n", "2.8.1"},
	}
	for _, c := range cases {
		if got := wordpress.ParseWPCliVersionForTest(c.raw); got != c.want {
			t.Errorf("ParseWPCliVersionForTest(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestWPCliVersionMatches(t *testing.T) {
	cases := []struct {
		raw   string
		want  string
		match bool
	}{
		{"WP-CLI 2.10.0", "2.10.0", true},
		{"WP-CLI 2.8.1", "2.10.0", false},
		{"WP-CLI 1.4.0", "2.10.0", false},
		{"", "2.10.0", false},
		{"WP-CLI 2.10.0", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := wordpress.WPCliVersionMatchesForTest(c.raw, c.want); got != c.match {
			t.Errorf("WPCliVersionMatchesForTest(%q, %q) = %v, want %v", c.raw, c.want, got, c.match)
		}
	}
}

func TestRecommendedWPCliVersion(t *testing.T) {
	cases := []struct {
		major int
		want  string
	}{
		{4, "2.4.0"},
		{5, "2.8.1"},
		{6, "2.10.0"},
		{7, ""},
		{0, ""},
	}
	for _, c := range cases {
		if got := wordpress.RecommendedWPCliVersionForTest(c.major); got != c.want {
			t.Errorf("RecommendedWPCliVersionForTest(%d) = %q, want %q", c.major, got, c.want)
		}
	}
}

// runFixWordPressCompatibility invokes the real FixWordPressCompatibility with a
// fake docker binary standing in for the project PHP container. The fake docker
// reports a WordPress 6.x install (wp-includes/version.php -> 6.8.1) and either
// exposes wp at the given version, or fails wp lookups until the install exec
// (the one containing curl) runs, which "installs" wp in the fake container.
func runFixWordPressCompatibility(t *testing.T, wpPresent bool, activeWPVersion string) (dockerLog string, output string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake docker shim targets POSIX sh")
	}

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "docker.log")

	present := "1"
	if !wpPresent {
		present = "0"
	}

	script := fmt.Sprintf("#!/bin/sh\nset -eu\necho \"$*\" >> %s\nstate=\"$(dirname %s)/.wp_installed\"\ncase \"$*\" in\n  *wp-includes/version.php*)\n    echo \"6.8.1\"\n    exit 0\n    ;;\n  *\"curl -sSfL\"*)\n    touch \"$state\"\n    exit 0\n    ;;\n  *\"wp --version\"*|*\"command -v wp\"*)\n    if [ \"%s\" != \"1\" ] && [ ! -f \"$state\" ]; then exit 127; fi\n    echo \"WP-CLI %s\"\n    exit 0\n    ;;\nesac\nexit 0\n", logPath, logPath, present, activeWPVersion)

	dockerPath := filepath.Join(shimDir, "docker")
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}

	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var captured bytes.Buffer
	pterm.SetDefaultOutput(&captured)
	defer pterm.SetDefaultOutput(os.Stdout)

	config := engine.Config{ProjectName: "wp-test", Framework: "wordpress"}
	if err := wordpress.FixWordPressCompatibility(config); err != nil {
		t.Fatalf("FixWordPressCompatibility: %v", err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake docker log: %v", err)
	}
	return string(logData), captured.String()
}

func TestFixWordPressCompatibilitySkipsDownloadWhenVersionMatches(t *testing.T) {
	dockerLog, output := runFixWordPressCompatibility(t, true, "2.10.0")

	if strings.Contains(dockerLog, "curl -sSfL") {
		t.Fatalf("expected no WP-CLI download when active version matches the pinned one, docker log: %s", dockerLog)
	}
	if !strings.Contains(output, "already active, skipping download") {
		t.Fatalf("expected a skip message on re-runs, got: %q", output)
	}
	if !strings.Contains(output, "WP-CLI 2.10.0") {
		t.Fatalf("expected the skip message to name the active version, got: %q", output)
	}
}

func TestFixWordPressCompatibilityUpgradesWhenVersionStale(t *testing.T) {
	dockerLog, output := runFixWordPressCompatibility(t, true, "2.8.1")

	if !strings.Contains(dockerLog, "curl -sSfL") {
		t.Fatalf("expected WP-CLI download when active version is stale, docker log: %s", dockerLog)
	}
	if !strings.Contains(dockerLog, "wp-cli-2.10.0.phar") {
		t.Fatalf("expected download of the pinned 2.10.0 phar for WordPress 6.x, docker log: %s", dockerLog)
	}
	if strings.Contains(output, "skipping download") {
		t.Fatalf("did not expect a skip message for a stale install, got: %q", output)
	}
}

func TestFixWordPressCompatibilityInstallsWhenMissing(t *testing.T) {
	dockerLog, output := runFixWordPressCompatibility(t, false, "")

	if !strings.Contains(dockerLog, "curl -sSfL") {
		t.Fatalf("expected WP-CLI download when wp is missing, docker log: %s", dockerLog)
	}
	if strings.Contains(output, "skipping download") {
		t.Fatalf("did not expect a skip message for a missing install, got: %q", output)
	}
}
