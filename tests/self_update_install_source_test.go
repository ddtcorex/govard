package tests

import (
	"strings"
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
		updater.InstallSourceWinget, updater.InstallSourceDocker, updater.InstallSourceSnap,
	}
	for _, source := range managed {
		if hint := updater.UpgradeHint(source); hint == "" {
			t.Errorf("UpgradeHint(%q) is empty, want a channel upgrade command", source)
		}
	}
}

// The update notice on `govard up` must name the command that owns the install:
// telling a snap user to run `govard self-update` sends them to a command that
// refuses and prints a second hint.
func TestUpdateCommandHintNamesTheOwningChannel(t *testing.T) {
	cases := map[string]string{
		updater.InstallSourceSnap:      "sudo snap refresh govard",
		updater.InstallSourceBrew:      updater.UpgradeHint(updater.InstallSourceBrew),
		updater.InstallSourceUnmanaged: "govard self-update",
	}
	for source, want := range cases {
		if got := updater.UpdateCommandHint(source); got != want {
			t.Errorf("UpdateCommandHint(%q) = %q, want %q", source, got, want)
		}
	}
}

// --force exists to override a package manager's claim, but a snap is a
// read-only squashfs: there is nothing to override, and the replace would fail
// late with EROFS after downloading the release.
func TestForceRefusalBlocksTheReadOnlySnap(t *testing.T) {
	err := updater.ForceRefusal(updater.InstallSourceSnap)
	if err == nil {
		t.Fatal("ForceRefusal(snap) = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "sudo snap refresh govard") {
		t.Errorf("the refusal must name the working command, got %q", err)
	}
	for _, source := range []string{updater.InstallSourceUnmanaged, updater.InstallSourceNpm, updater.InstallSourceBrew} {
		if err := updater.ForceRefusal(source); err != nil {
			t.Errorf("ForceRefusal(%q) = %v, want nil: --force stays available where the binary is writable", source, err)
		}
	}
}

func TestUpgradeHintForSnapUsesSnapRefresh(t *testing.T) {
	if hint := updater.UpgradeHint(updater.InstallSourceSnap); hint != "sudo snap refresh govard" {
		t.Fatalf("UpgradeHint(snap) = %q, want %q", hint, "sudo snap refresh govard")
	}
}
