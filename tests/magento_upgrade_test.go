package tests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"

	"github.com/pterm/pterm"
)

func TestMergeComposerMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string]interface{}
		target   map[string]interface{}
		key      string
		expected map[string]interface{}
	}{
		{
			name: "add new object key",
			current: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0.0",
				},
			},
			target: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/core": "2.4.8",
				},
			},
			key: "require",
			expected: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0.0",
					"sample/core":    "2.4.8",
				},
			},
		},
		{
			name: "override existing key in object",
			current: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/core":    "2.4.7",
					"sample/package": "1.0.0",
				},
			},
			target: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/core": "2.4.8",
				},
			},
			key: "require",
			expected: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0.0",
					"sample/core":    "2.4.8",
				},
			},
		},
		{
			name: "scalar values replacement",
			current: map[string]interface{}{
				"minimum-stability": "dev",
			},
			target: map[string]interface{}{
				"minimum-stability": "stable",
			},
			key: "minimum-stability",
			expected: map[string]interface{}{
				"minimum-stability": "stable",
			},
		},
		{
			name:    "key missing in current",
			current: map[string]interface{}{},
			target: map[string]interface{}{
				"require-dev": map[string]interface{}{
					"test/runner": "9.0",
				},
			},
			key: "require-dev",
			expected: map[string]interface{}{
				"require-dev": map[string]interface{}{
					"test/runner": "9.0",
				},
			},
		},
		{
			name: "key missing in target",
			current: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0",
				},
			},
			target: map[string]interface{}{},
			key:    "require",
			expected: map[string]interface{}{
				"require": map[string]interface{}{
					"sample/package": "1.0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			magento2.MergeComposerMapKeysForTest(tt.current, tt.target, tt.key)
			if !reflect.DeepEqual(tt.current, tt.expected) {
				t.Errorf("MergeComposerMapKeysForTest() mismatch.\nGot:  %v\nWant: %v", tt.current, tt.expected)
			}
		})
	}
}

// runMagentoUpgradeBehindFakeDocker drives the real magento2.RunUpgrade with
// a fake docker on PATH that answers every call with exit 0, prints a minimal
// composer.json for the `cat` calls, and records each invocation on its own
// line. The environment update and setup:upgrade are skipped, so the run goes
// straight from the composer.json merge to the composer update.
func runMagentoUpgradeBehindFakeDocker(t *testing.T, config engine.Config) []string {
	t.Helper()
	return runMagentoUpgradeWithComposerJSON(t, config, `{"require":{"magento/product-community-edition":"2.4.6"},"config":{"platform":{"php":"8.1"}}}`)
}

// runMagentoUpgradeWithComposerJSON is runMagentoUpgradeBehindFakeDocker with
// the project's own composer.json chosen by the caller.
func runMagentoUpgradeWithComposerJSON(t *testing.T, config engine.Config, composerJSON string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shim targets POSIX sh")
	}
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "composer.json"), []byte(composerJSON), 0o644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}
	shimDir := t.TempDir()
	dockerLog := filepath.Join(shimDir, "docker.log")
	script := fmt.Sprintf(`#!/bin/sh
echo "$*" >> %s
case "$*" in
*"cat temp_upgrade_source/composer.json"*|*"cat composer.json"*)
	echo '{"require":{"magento/product-community-edition":"2.4.8"}}'
	;;
esac
exit 0
`, dockerLog)
	if err := os.WriteFile(filepath.Join(shimDir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := magento2.RunUpgrade(context.Background(), config, engine.UpgradeOptions{
		ProjectName:   "sample-project",
		ProjectDir:    projectDir,
		TargetVersion: "2.4.8",
		NoEnvUpdate:   true,
		NoInteraction: true,
		NoDBUpgrade:   true,
		Stdout:        io.Discard,
		Stderr:        io.Discard,
	}, magento2.UpgradeVariantForTest())
	if err != nil {
		t.Fatalf("RunUpgrade() error = %v", err)
	}
	data, err := os.ReadFile(dockerLog)
	if err != nil {
		t.Fatalf("read fake docker log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func indexOfCommand(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

// TestMagentoUpgradeRefreshesThePlatformPinBeforeTheUpdate asserts the
// upgrade rewrites composer's platform.php to the runtime PHP before it runs
// composer update, so a pin written for the old PHP does not outlive it.
func TestMagentoUpgradeRefreshesThePlatformPinBeforeTheUpdate(t *testing.T) {
	config := engine.Config{Framework: "magento2", FrameworkVersion: "2.4.8"}
	config.Stack.PHPVersion = "8.4"
	lines := runMagentoUpgradeBehindFakeDocker(t, config)

	pin := indexOfCommand(lines, "composer config platform.php 8.4")
	update := indexOfCommand(lines, "composer update")
	if pin < 0 {
		t.Fatalf("expected `composer config platform.php 8.4`, docker log:\n%s", strings.Join(lines, "\n"))
	}
	if update < 0 || pin > update {
		t.Fatalf("expected the platform pin (line %d) before composer update (line %d), docker log:\n%s", pin, update, strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[pin], "exec -w /var/www/html sample-project-php-1 composer config platform.php 8.4") {
		t.Fatalf("pin ran in the wrong container or directory: %s", lines[pin])
	}
}

// TestMagentoUpgradeSkipsThePlatformPinWhenItCannotApply asserts the docker
// calls are unchanged when the runtime PHP is unknown or Composer cannot honor
// the pin (Composer 1, or 2.0/2.1 without the ext-* wildcard).
func TestMagentoUpgradeSkipsThePlatformPinWhenItCannotApply(t *testing.T) {
	cases := []struct {
		name     string
		php      string
		composer string
	}{
		{"unknown PHP", "", ""},
		{"Composer 1", "7.4", "1"},
		{"Composer 2.1", "8.1", "2.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := engine.Config{Framework: "magento2", FrameworkVersion: "2.4.8"}
			config.Stack.PHPVersion = tc.php
			config.Stack.ComposerVersion = tc.composer
			lines := runMagentoUpgradeBehindFakeDocker(t, config)
			if indexOfCommand(lines, "composer update") < 0 {
				t.Fatalf("the run never reached composer update, docker log:\n%s", strings.Join(lines, "\n"))
			}
			if i := indexOfCommand(lines, "platform.php"); i >= 0 {
				t.Fatalf("did not expect a platform pin, got %s", lines[i])
			}
		})
	}
}

// TestMagentoUpgradeLeavesAProjectWithoutAPlatformPinAlone asserts upgrade only
// refreshes an existing config.platform.php. composer update runs with
// --ignore-platform-reqs, so the pin plays no part in resolving the upgrade, and
// writing one would change a committed composer.json (and the platform check of
// a production composer install) as a side effect. The run says how to add one.
func TestMagentoUpgradeLeavesAProjectWithoutAPlatformPinAlone(t *testing.T) {
	var captured bytes.Buffer
	pterm.SetDefaultOutput(&captured)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	config := engine.Config{Framework: "magento2", FrameworkVersion: "2.4.8"}
	config.Stack.PHPVersion = "8.4"
	lines := runMagentoUpgradeWithComposerJSON(t, config, `{"require":{"magento/product-community-edition":"2.4.6"},"config":{"sort-packages":true}}`)

	if indexOfCommand(lines, "composer update") < 0 {
		t.Fatalf("the run never reached composer update, docker log:\n%s", strings.Join(lines, "\n"))
	}
	if i := indexOfCommand(lines, "composer config platform.php"); i >= 0 {
		t.Fatalf("did not expect a platform pin to be written, got %s", lines[i])
	}
	out := ansiEscape.ReplaceAllString(captured.String(), "")
	if !strings.Contains(out, "no platform pin") || !strings.Contains(out, "govard tool composer config platform.php <version>") {
		t.Fatalf("expected one line naming the missing pin and the command to add it, got:\n%s", out)
	}
}
