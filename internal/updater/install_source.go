package updater

import (
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

	installSourceEnvVar   = "GOVARD_INSTALL_SOURCE"
	installSourceFileName = ".install-source"
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
	return normalizeInstallSource(os.Getenv(installSourceEnvVar), marker)
}

func normalizeInstallSource(env, marker string) string {
	for _, candidate := range []string{env, marker} {
		switch normalized := strings.ToLower(strings.TrimSpace(candidate)); normalized {
		case InstallSourceNpm, InstallSourceBrew, InstallSourceApt,
			InstallSourceYum, InstallSourceChoco, InstallSourceScoop,
			InstallSourceWinget, InstallSourceDocker:
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
	default:
		return ""
	}
}

// InstallSourceForTest exposes detection with an explicit marker value for tests in /tests.
func InstallSourceForTest(marker string) string {
	return normalizeInstallSource(os.Getenv(installSourceEnvVar), marker)
}
