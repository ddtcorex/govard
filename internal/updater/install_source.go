package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Install sources recognized by Govard. Anything else normalizes to Unmanaged.
const (
	InstallSourceUnmanaged = "unmanaged"
	InstallSourceNpm       = "npm"
	InstallSourceBrew      = "brew"
	InstallSourceApt       = "apt"
	InstallSourceYum       = "yum"
	InstallSourceChoco     = "choco"
	InstallSourceScoop     = "scoop"
	InstallSourceWinget    = "winget"
	InstallSourceDocker    = "docker"
	InstallSourceSnap      = "snap"

	installSourceEnvVar   = "GOVARD_INSTALL_SOURCE"
	installSourceFileName = ".install-source"

	// snapName is the Snap Store name; snapd exports it as SNAP_NAME.
	snapName = "govard"
)

// InstallSource reports how this binary was installed, for ownership decisions
// (self-update must defer to managed channels). Never returns "".
func InstallSource() string {
	execPath, err := os.Executable()
	marker := ""
	if err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
			execPath = resolved
		}
		if data, readErr := os.ReadFile(filepath.Join(filepath.Dir(execPath), installSourceFileName)); readErr == nil {
			marker = string(data)
		}
	}
	return detectInstallSource(os.Getenv(installSourceEnvVar), os.Getenv("SNAP"), os.Getenv("SNAP_NAME"), execPath, marker)
}

// detectInstallSource lets snap ownership outrank the marker file: the snap is
// a read-only squashfs that ships no marker, and snapd owns its refreshes.
func detectInstallSource(env, snapDir, snapEnvName, execPath, marker string) string {
	if isSnapInstall(snapDir, snapEnvName, execPath) {
		marker = InstallSourceSnap
	}
	return normalizeInstallSource(env, marker)
}

// isSnapInstall requires the executable to live inside $SNAP, not just the
// snapd environment: SNAP/SNAP_NAME leak into every child process, so a host
// binary started from a snap's shell must not be mistaken for the snap.
func isSnapInstall(snapDir, snapEnvName, execPath string) bool {
	if snapEnvName != snapName || strings.TrimSpace(snapDir) == "" || execPath == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(snapDir), filepath.Clean(execPath))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
}

func normalizeInstallSource(env, marker string) string {
	for _, candidate := range []string{env, marker} {
		switch normalized := strings.ToLower(strings.TrimSpace(candidate)); normalized {
		case InstallSourceNpm, InstallSourceBrew, InstallSourceApt,
			InstallSourceYum, InstallSourceChoco, InstallSourceScoop,
			InstallSourceWinget, InstallSourceDocker, InstallSourceSnap:
			return normalized
		}
	}
	return InstallSourceUnmanaged
}

// UpgradeHint returns the channel-correct upgrade command for a managed source,
// or "" for unmanaged installs (self-update handles those itself).
func UpgradeHint(source string) string {
	switch normalizeInstallSource(source, "") {
	case InstallSourceNpm:
		return "npm i -g @ddtcorex/govard@latest"
	case InstallSourceBrew:
		return "brew upgrade govard"
	case InstallSourceApt:
		return "sudo apt update && sudo apt upgrade govard"
	case InstallSourceYum:
		return "sudo dnf upgrade govard"
	case InstallSourceChoco:
		return "choco upgrade govard"
	case InstallSourceScoop:
		return "scoop update govard"
	case InstallSourceWinget:
		return "winget upgrade govard"
	case InstallSourceDocker:
		return "docker pull ghcr.io/ddtcorex/govard:latest"
	case InstallSourceSnap:
		return "sudo snap refresh govard"
	default:
		return ""
	}
}

// UpdateCommandHint names the command that upgrades this install: the owning
// channel's own command for a managed source, `govard self-update` otherwise.
func UpdateCommandHint(source string) string {
	if hint := UpgradeHint(source); hint != "" {
		return hint
	}
	return "govard self-update"
}

// ForceRefusal reports why `self-update --force` cannot override source, or nil
// when it can. The snap is a read-only squashfs, so a forced replace would only
// fail with EROFS after the release was downloaded.
func ForceRefusal(source string) error {
	if source == InstallSourceSnap {
		return fmt.Errorf("govard runs from its snap, which is read-only; --force cannot replace it. Upgrade with: %s", UpgradeHint(source))
	}
	return nil
}

// InstallSourceForTest exposes detection with an explicit marker value for tests in /tests.
func InstallSourceForTest(marker string) string {
	return normalizeInstallSource(os.Getenv(installSourceEnvVar), marker)
}

// InstallSourceForSnapTest exposes detection with explicit snapd env and executable path for tests in /tests.
func InstallSourceForSnapTest(snapDir, snapEnvName, execPath, marker string) string {
	return detectInstallSource(os.Getenv(installSourceEnvVar), snapDir, snapEnvName, execPath, marker)
}
