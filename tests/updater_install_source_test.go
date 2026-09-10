package tests

import (
	"testing"

	"govard/internal/updater"
)

func TestInstallSourceDefaultsToUnmanaged(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_INSTALL_SOURCE", "")

	if got := updater.InstallSourceForTest(""); got != updater.InstallSourceUnmanaged {
		t.Fatalf("InstallSource() = %q, want %q", got, updater.InstallSourceUnmanaged)
	}
}

func TestInstallSourcePrefersEnvOverMarker(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_INSTALL_SOURCE", "npm")

	if got := updater.InstallSourceForTest("brew"); got != updater.InstallSourceNpm {
		t.Fatalf("InstallSource() = %q, want %q", got, updater.InstallSourceNpm)
	}
}

func TestInstallSourceRejectsUnknownMarker(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_INSTALL_SOURCE", "")

	if got := updater.InstallSourceForTest("not-a-real-source"); got != updater.InstallSourceUnmanaged {
		t.Fatalf("InstallSource() = %q, want %q", got, updater.InstallSourceUnmanaged)
	}
}
