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

func TestInstallSourceDetectsGovardSnap(t *testing.T) {
	cases := []struct {
		name     string
		snapDir  string
		snapName string
		execPath string
		want     string
	}{
		{"binary inside the govard snap", "/snap/govard/42", "govard", "/snap/govard/42/bin/govard", updater.InstallSourceSnap},
		{"not running under snapd", "", "", "/usr/local/bin/govard", updater.InstallSourceUnmanaged},
		{"launched from another snap's environment", "/snap/code/7", "code", "/usr/local/bin/govard", updater.InstallSourceUnmanaged},
		{"snap env inherited by a host binary", "/snap/govard/42", "govard", "/usr/local/bin/govard", updater.InstallSourceUnmanaged},
		{"sibling path sharing the snap prefix", "/snap/govard/42", "govard", "/snap/govard/420/bin/govard", updater.InstallSourceUnmanaged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOVARD_INSTALL_SOURCE", "")
			if got := updater.InstallSourceForSnapTest(tc.snapDir, tc.snapName, tc.execPath, ""); got != tc.want {
				t.Fatalf("InstallSource() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInstallSourceEnvOverridesSnapDetection(t *testing.T) {
	t.Setenv("GOVARD_INSTALL_SOURCE", "unmanaged-by-choice")
	// An unknown override value is ignored, so snap detection still applies.
	if got := updater.InstallSourceForSnapTest("/snap/govard/42", "govard", "/snap/govard/42/bin/govard", ""); got != updater.InstallSourceSnap {
		t.Fatalf("InstallSource() = %q, want %q", got, updater.InstallSourceSnap)
	}
	t.Setenv("GOVARD_INSTALL_SOURCE", "npm")
	if got := updater.InstallSourceForSnapTest("/snap/govard/42", "govard", "/snap/govard/42/bin/govard", ""); got != updater.InstallSourceNpm {
		t.Fatalf("InstallSource() = %q, want %q", got, updater.InstallSourceNpm)
	}
}
