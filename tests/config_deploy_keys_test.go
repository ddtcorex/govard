package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// The deploy block is a first-class part of the project configuration, and a
// reader who cannot ask the CLI what a deploy will use ends up re-deriving the
// defaults by hand — which is how `deploy.settings.php_bin` came to look
// unsupported while the deploy used it.
func TestConfigGetReadsTheDeployBlock(t *testing.T) {
	config := engine.Config{Deploy: engine.DeployConfig{
		KeepReleases:       5,
		CommandTimeout:     "30m",
		MaintenanceTimeout: "15m",
		LockStaleAfter:     "2h",
		ArtifactDir:        "artifacts",
		DBBackup:           true,
		Verify:             engine.DeployVerifyConfig{URL: "https://example.test/", Timeout: "30s"},
		Settings: map[string]any{
			"php_bin":     "php8.3",
			"shared_dirs": []string{"var/log", "var/cache"},
		},
	}}

	want := map[string]string{
		"deploy.keep_releases":        "5",
		"deploy.command_timeout":      "30m",
		"deploy.maintenance_timeout":  "15m",
		"deploy.lock_stale_after":     "2h",
		"deploy.artifact_dir":         "artifacts",
		"deploy.db_backup":            "true",
		"deploy.verify.url":           "https://example.test/",
		"deploy.verify.timeout":       "30s",
		"deploy.settings.php_bin":     "php8.3",
		"deploy.settings.shared_dirs": "var/log,var/cache",
	}
	for key, expected := range want {
		got, ok := cmd.GetConfigValueForTest(config, key)
		if !ok {
			t.Errorf("%s is not readable", key)
			continue
		}
		if got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}

	// A setting the project did not write reads as empty rather than as an
	// unknown key: the key exists and its value is "nothing configured".
	if got, ok := cmd.GetConfigValueForTest(config, "deploy.settings.mage_mode"); !ok || got != "" {
		t.Errorf("an unset deploy setting must read as empty, got %q ok=%v", got, ok)
	}
	// The block itself is not a single value, and a key that is not a key is a
	// typo rather than an empty answer.
	if _, ok := cmd.GetConfigValueForTest(config, "deploy"); ok {
		t.Error("the whole deploy block is not a single value")
	}
	if _, ok := cmd.GetConfigValueForTest(config, "deploy.not_a_key"); ok {
		t.Error("an unknown deploy key must be refused")
	}
}

func TestConfigSetWritesTheDeployBlock(t *testing.T) {
	config := engine.Config{}

	for key, value := range map[string]string{
		"deploy.keep_releases":    "7",
		"deploy.command_timeout":  "45m",
		"deploy.artifact_dir":     "artifacts",
		"deploy.verify.url":       "https://shop.example.test/",
		"deploy.verify.timeout":   "45s",
		"deploy.db_backup":        "true",
		"deploy.settings.php_bin": "php8.3",
	} {
		ok, err := cmd.SetConfigValueForTest(&config, key, value)
		if err != nil || !ok {
			t.Fatalf("set %s: ok=%v err=%v", key, ok, err)
		}
	}

	if config.Deploy.KeepReleases != 7 {
		t.Errorf("keep_releases = %d, want 7", config.Deploy.KeepReleases)
	}
	if config.Deploy.Verify.URL != "https://shop.example.test/" {
		t.Errorf("verify.url = %q", config.Deploy.Verify.URL)
	}
	if !config.Deploy.DBBackup {
		t.Error("db_backup must be true")
	}
	if got := config.Deploy.Settings["php_bin"]; got != "php8.3" {
		t.Errorf("settings.php_bin = %v", got)
	}

	// A value that cannot be the type it claims is a usage error, not a silent
	// zero: a deploy with keep_releases 0 would prune everything but the live
	// release.
	if _, err := cmd.SetConfigValueForTest(&config, "deploy.keep_releases", "many"); err == nil {
		t.Error("a non-numeric keep_releases must be refused")
	}
	if _, err := cmd.SetConfigValueForTest(&config, "deploy.db_backup", "maybe"); err == nil {
		t.Error("a non-boolean db_backup must be refused")
	}
}

// A list setting stays a list. Writing `shared_dirs` as one comma-separated
// argument would otherwise replace a list the deploy reads with a single string
// it cannot read.
func TestConfigSetKeepsAListSettingAList(t *testing.T) {
	config := engine.Config{Deploy: engine.DeployConfig{
		Settings: map[string]any{"shared_dirs": []string{"var/log"}},
	}}

	if _, err := cmd.SetConfigValueForTest(&config, "deploy.settings.shared_dirs", "var/log,var/cache"); err != nil {
		t.Fatalf("set a list setting: %v", err)
	}
	got, ok := config.Deploy.Settings["shared_dirs"].([]string)
	if !ok {
		t.Fatalf("shared_dirs became %T, want []string", config.Deploy.Settings["shared_dirs"])
	}
	if strings.Join(got, ",") != "var/log,var/cache" {
		t.Errorf("shared_dirs = %v", got)
	}

	// A setting that is a plain string stays one, even when the value contains
	// a comma: `frontend_command` is a command line, not a list.
	config.Deploy.Settings["frontend_command"] = "npm ci && npm run build"
	if _, err := cmd.SetConfigValueForTest(&config, "deploy.settings.frontend_command", "npm ci, npm run build"); err != nil {
		t.Fatalf("set a string setting: %v", err)
	}
	if got, ok := config.Deploy.Settings["frontend_command"].(string); !ok || got != "npm ci, npm run build" {
		t.Errorf("frontend_command = %#v, want one string", config.Deploy.Settings["frontend_command"])
	}
}
