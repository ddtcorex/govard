package tests

import (
	"testing"

	"govard/internal/updater"
)

func TestUpgradeHintRefusesNpmSource(t *testing.T) {
	hint := updater.UpgradeHint(updater.InstallSourceNpm)
	if hint == "" {
		t.Fatal("UpgradeHint(npm) is empty, want the npm upgrade command")
	}
}

func TestUpgradeHintEmptyForUnmanaged(t *testing.T) {
	if hint := updater.UpgradeHint(updater.InstallSourceUnmanaged); hint != "" {
		t.Fatalf("UpgradeHint(unmanaged) = %q, want empty", hint)
	}
}

func TestUpgradeHintCoversEveryManagedSource(t *testing.T) {
	managed := []string{
		updater.InstallSourceNpm, updater.InstallSourceBrew, updater.InstallSourceApt,
		updater.InstallSourceYum, updater.InstallSourceChoco, updater.InstallSourceScoop,
		updater.InstallSourceWinget, updater.InstallSourceDocker,
	}
	for _, source := range managed {
		if hint := updater.UpgradeHint(source); hint == "" {
			t.Errorf("UpgradeHint(%q) is empty, want a channel upgrade command", source)
		}
	}
}
