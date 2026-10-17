package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/conventions"
	"govard/internal/desktop"
)

// seedToolRunnerProject writes a minimal govard project config with the given
// framework and returns its path. ListToolPresets accepts a direct path, so
// no registry or container state is needed and the test stays hermetic.
func seedToolRunnerProject(t *testing.T, framework string) string {
	t.Helper()
	projectPath := filepath.Join(t.TempDir(), "sample-project")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := "project_name: sample-project\ndomain: sample-project.test\nframework: " + framework + "\n"
	if err := os.WriteFile(filepath.Join(projectPath, conventions.BaseConfigFile), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return projectPath
}

func presetIDs(presets []desktop.ToolPreset) map[string]desktop.ToolPreset {
	byID := make(map[string]desktop.ToolPreset, len(presets))
	for _, p := range presets {
		byID[p.ID] = p
	}
	return byID
}

func TestToolRunnerMagentoPresets(t *testing.T) {
	projectPath := seedToolRunnerProject(t, "magento2")
	presets, err := desktop.ListToolPresets(projectPath)
	if err != nil {
		t.Fatalf("ListToolPresets() error: %v", err)
	}
	byID := presetIDs(presets)

	flush, ok := byID["magento-cache-flush"]
	if !ok {
		t.Fatal("expected magento-cache-flush preset for a magento2 project")
	}
	if flush.Display != "bin/magento cache:flush" {
		t.Errorf("magento-cache-flush.Display = %q, want %q", flush.Display, "bin/magento cache:flush")
	}
	if _, ok := byID["magento-setup-di-compile"]; !ok {
		t.Error("expected magento-setup-di-compile preset for a magento2 project")
	}
	// Generic presets ride along with framework ones.
	if _, ok := byID["composer-install"]; !ok {
		t.Error("expected generic composer-install preset alongside magento2 presets")
	}
}

func TestToolRunnerLaravelPresets(t *testing.T) {
	projectPath := seedToolRunnerProject(t, "laravel")
	presets, err := desktop.ListToolPresets(projectPath)
	if err != nil {
		t.Fatalf("ListToolPresets() error: %v", err)
	}
	byID := presetIDs(presets)

	migrate, ok := byID["laravel-migrate"]
	if !ok {
		t.Fatal("expected laravel-migrate preset for a laravel project")
	}
	if migrate.Display != "php artisan migrate" {
		t.Errorf("laravel-migrate.Display = %q, want %q", migrate.Display, "php artisan migrate")
	}
	if _, ok := byID["laravel-cache-clear"]; !ok {
		t.Error("expected laravel-cache-clear preset for a laravel project")
	}
	if _, ok := byID["magento-cache-flush"]; ok {
		t.Error("magento-cache-flush must not be listed for a laravel project")
	}
}

func TestToolRunnerGenericFallback(t *testing.T) {
	projectPath := seedToolRunnerProject(t, "custom")
	presets, err := desktop.ListToolPresets(projectPath)
	if err != nil {
		t.Fatalf("ListToolPresets() error: %v", err)
	}
	byID := presetIDs(presets)
	if _, ok := byID["composer-install"]; !ok {
		t.Error("expected generic composer-install preset for an unknown framework")
	}
	if _, ok := byID["npm-install"]; !ok {
		t.Error("expected generic npm-install preset for an unknown framework")
	}
	if _, ok := byID["magento-cache-flush"]; ok {
		t.Error("magento-cache-flush must not be listed for an unknown framework")
	}
	if _, ok := byID["laravel-migrate"]; ok {
		t.Error("laravel-migrate must not be listed for an unknown framework")
	}
}

func TestToolRunnerUnresolvableProjectSurfacesError(t *testing.T) {
	if _, err := desktop.ListToolPresets(""); err == nil {
		t.Error("ListToolPresets(\"\") = nil error, want the resolution failure surfaced")
	}
	if _, err := desktop.ListToolPresets("   "); err == nil {
		t.Error("ListToolPresets(whitespace) = nil error, want the resolution failure surfaced")
	}
}

// Every preset must carry exec-ready argv with no shell in it: non-empty,
// and no element may contain whitespace or a shell metacharacter, so Task 2
// can pass Argv straight to exec.Command.
func TestToolRunnerPresetsHaveSafeArgv(t *testing.T) {
	for _, framework := range []string{"magento2", "laravel", "custom", ""} {
		for _, preset := range desktop.ListToolPresetsForFramework(framework) {
			if preset.Display == "" {
				t.Errorf("preset %q has an empty Display", preset.ID)
			}
			if len(preset.Argv) == 0 {
				t.Errorf("preset %q has an empty Argv", preset.ID)
				continue
			}
			for _, element := range preset.Argv {
				if element == "" {
					t.Errorf("preset %q has an empty Argv element", preset.ID)
				}
				if strings.ContainsAny(element, " \t\n;|&$()<>'\"\\*?~#`!^={}[]") {
					t.Errorf("preset %q Argv element %q contains shell-unsafe characters", preset.ID, element)
				}
			}
		}
	}
}

func TestToolRunnerValidateArgsAcceptsValid(t *testing.T) {
	if err := desktop.ValidateToolArgs("magento-cache-flush", map[string]string{"type": "full_page"}); err != nil {
		t.Errorf("ValidateToolArgs(valid type) error: %v", err)
	}
	if err := desktop.ValidateToolArgs("magento-cache-flush", map[string]string{}); err != nil {
		t.Errorf("ValidateToolArgs(omitted optional arg) error: %v", err)
	}
	if err := desktop.ValidateToolArgs("laravel-migrate", map[string]string{"database": "mysql", "force": "true"}); err != nil {
		t.Errorf("ValidateToolArgs(valid database+force) error: %v", err)
	}
	// A flag arg present without a value counts as set.
	if err := desktop.ValidateToolArgs("laravel-migrate", map[string]string{"force": ""}); err != nil {
		t.Errorf("ValidateToolArgs(empty flag value) error: %v", err)
	}
}

func TestToolRunnerValidateArgsRejectsInjection(t *testing.T) {
	injections := map[string]string{
		"semicolon":         "; rm -rf /",
		"chain":             "full_page && reboot",
		"pipe":              "full_page | tee /tmp/x",
		"subshell":          "$(reboot)",
		"backtick":          "`reboot`",
		"redirect":          "full_page > /tmp/x",
		"newline":           "full_page\nrm -rf /",
		"dollar-var":        "$HOME",
		"single-quote":      "full_'page",
		"double-quote":      "full_\"page",
		"backslash":         `full_\page`,
		"hash":              "full_page # comment",
		"tilde":             "~/x",
		"glob":              "full_*",
		"null-byte":         "full_\x00page",
		"unicode-semicolon": "full_\uFF1Bpage",
	}
	for name, value := range injections {
		t.Run(name, func(t *testing.T) {
			// Custom per-arg pattern branch.
			if err := desktop.ValidateToolArgs("magento-cache-flush", map[string]string{"type": value}); err == nil {
				t.Errorf("ValidateToolArgs(type=%q) = nil, want rejection", value)
			}
			// Default safe-pattern branch (database has no per-arg Pattern).
			if err := desktop.ValidateToolArgs("laravel-migrate", map[string]string{"database": value}); err == nil {
				t.Errorf("ValidateToolArgs(database=%q) = nil, want rejection", value)
			}
		})
	}
}

func TestToolRunnerValidateArgsLengthBoundary(t *testing.T) {
	atLimit := strings.Repeat("a", 256)
	if err := desktop.ValidateToolArgs("magento-cache-flush", map[string]string{"type": atLimit}); err != nil {
		t.Errorf("ValidateToolArgs(256-char value) error: %v, want accepted at the boundary", err)
	}
	overLimit := strings.Repeat("a", 257)
	if err := desktop.ValidateToolArgs("magento-cache-flush", map[string]string{"type": overLimit}); err == nil {
		t.Error("ValidateToolArgs(257-char value) = nil, want rejection")
	}
}

func TestToolRunnerValidateArgsRejectsUnknownArg(t *testing.T) {
	if err := desktop.ValidateToolArgs("laravel-cache-clear", map[string]string{"force": "true"}); err == nil {
		t.Error("ValidateToolArgs(undeclared arg) = nil, want rejection")
	}
}

func TestToolRunnerValidateArgsRejectsUnknownPreset(t *testing.T) {
	if _, ok := desktop.FindToolPreset("no-such-tool"); ok {
		t.Fatal("FindToolPreset(no-such-tool) found, want not found")
	}
	// Membership resolves inside ValidateToolArgs: no preset struct can be
	// smuggled past the allowlist.
	if err := desktop.ValidateToolArgs("no-such-tool", map[string]string{}); err == nil {
		t.Error("ValidateToolArgs(unknown id) = nil, want rejection")
	}
	if err := desktop.ValidateToolArgs("", map[string]string{}); err == nil {
		t.Error("ValidateToolArgs(empty id) = nil, want rejection")
	}
}
