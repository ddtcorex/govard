package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/conventions"

	"github.com/pterm/pterm"
)

// ProfileShiftInfo holds structured data about a detected environment profile change.
type ProfileShiftInfo struct {
	Shifted         bool
	Reason          string
	PreviousPHP     string
	CurrentPHP      string
	PreviousProfile string
	CurrentProfile  string
	PreviousVersion string
	CurrentVersion  string
	IsInitial       bool
}

// DetectProfileShift detects whether the current config represents a
// profile change compared to the last known state (lock file or registry).
// This is intended to be called early in the pipeline, before containers start.
func DetectProfileShift(config Config) ProfileShiftInfo {
	cwd, _ := os.Getwd()
	previousPHP, previousProfile, previousVersion := "", "", ""
	// currentProfile will be set from registry when PreviousProfile exists,
	// otherwise from config at the end. Default to "default" for empty profile.
	currentProfile := ""
	isInitial := false

	// Check registry first for previous_profile (set during profile switch)
	if entry, ok := GetProjectRegistryEntry(cwd); ok {
		prevProfile := strings.TrimSpace(entry.PreviousProfile)
		if prevProfile != "" {
			// Previous profile was saved during switch - use it
			previousProfile = prevProfile
			previousPHP = strings.TrimSpace(entry.PHPVersion)
			previousVersion = strings.TrimSpace(entry.FrameworkVersion)
			// When using PreviousProfile, currentProfile should come from registry
			// (config file may not have profile field, causing false shift detection)
			currentProfile = strings.TrimSpace(entry.Profile)
			if currentProfile == "" {
				currentProfile = "default"
			}
			if previousProfile == "" {
				previousProfile = "default"
			}
		} else {
			// No previous_profile, check current profile in registry
			// for potential PHP version or profile change
			currentRegProfile := strings.TrimSpace(entry.Profile)
			configProfile := strings.TrimSpace(config.Profile)
			if currentRegProfile != "" {
				// Only detect a profile change if config has a non-empty profile.
				// Empty profile means "use profile default" → NOT a change.
				if configProfile != "" && currentRegProfile != configProfile {
					// Profile changed
					previousProfile = currentRegProfile
				}
			}
			// Always capture PHP version and framework version from registry for comparison.
			// This ensures we can detect PHP/version changes even when there's no profile in registry.
			previousPHP = strings.TrimSpace(entry.PHPVersion)
			previousVersion = strings.TrimSpace(entry.FrameworkVersion)
		}
	}

	// Fallback to lock file only if registry doesn't have previous info
	if previousPHP == "" && previousProfile == "" {
		lockFile, err := ReadLockFile(LockFilePath(cwd))
		if err == nil {
			previousPHP = strings.TrimSpace(lockFile.Stack.PHPVersion)
			previousProfile = strings.TrimSpace(lockFile.Project.Profile)
			previousVersion = strings.TrimSpace(lockFile.Project.FrameworkVersion)
		} else {
			isInitial = true
		}
	}

	currentPHP := strings.TrimSpace(config.Stack.PHPVersion)
	// Only override currentProfile from config if not already set from registry
	if currentProfile == "" {
		currentProfile = strings.TrimSpace(config.Profile)
	}
	currentVersion := strings.TrimSpace(config.FrameworkVersion)

	// Check for any change: profile, PHP version, or framework version.
	// Empty current values mean "use profile default" → NOT a shift.
	// For framework version, only compare base version (major.minor.patch),
	// ignore patch suffix (e.g., p9 vs p10 are the same base version).
	// Note: "Initial configuration" (no previous info) does NOT trigger shift.
	// User can run `govard config auto` manually if needed.
	reason := ""
	shifted := false

	if previousPHP != "" || previousProfile != "" {
		if previousPHP != "" && currentPHP != "" && previousPHP != currentPHP {
			shifted = true
			reason = fmt.Sprintf("PHP version changed: %s -> %s", previousPHP, currentPHP)
		}
		if previousProfile != "" && currentProfile != "" && previousProfile != currentProfile {
			shifted = true
			reason = fmt.Sprintf("Profile changed: %q -> %q", previousProfile, currentProfile)
		}
		if previousVersion != "" && currentVersion != "" && baseVersion(previousVersion) != baseVersion(currentVersion) {
			shifted = true
			reason = fmt.Sprintf("Version changed: %s -> %s", previousVersion, currentVersion)
		}
	}
	// No previous info = no shift detected. User runs `govard config auto` manually if needed.

	return ProfileShiftInfo{
		Shifted:         shifted,
		Reason:          reason,
		PreviousPHP:     previousPHP,
		CurrentPHP:      currentPHP,
		PreviousProfile: previousProfile,
		CurrentProfile:  currentProfile,
		PreviousVersion: previousVersion,
		CurrentVersion:  currentVersion,
		IsInitial:       isInitial,
	}
}

// baseVersion extracts the base version without patch suffix.
// e.g., "2.4.7-p9" -> "2.4.7", "2.4.8" -> "2.4.8"
func baseVersion(version string) string {
	if idx := strings.Index(version, "-"); idx > 0 {
		return version[:idx]
	}
	return version
}

// PrepareInfraForShift handles infrastructure cleanup BEFORE containers
// are started. This must be called before `docker compose up` when a profile
// shift is detected, to avoid issues like Redis RDB version incompatibility.
func PrepareInfraForShift(projectName string, config Config) {
	// Force remove Redis/Valkey container to avoid RDB version conflicts (e.g. 7.2 -> 7.0)
	if config.Stack.Features.Cache || config.Stack.Services.Cache != "none" {
		redisContainer := fmt.Sprintf("%s%s", projectName, conventions.RedisSuffix)
		pterm.Info.Println("Removing stale cache container for clean profile start...")
		_ = exec.Command("docker", "rm", "-f", redisContainer).Run()
	}
}

// ResolveEffectiveProfile resolves the effective profile for a project.
// Priority: 1. explicit profile (--profile flag), 2. project registry (last-used), 3. empty (default)
//
// The literal name "default" is treated as the empty/no-profile sentinel,
// matching `config profile switch default` (internal/cmd/profile.go), so
// `--profile default` never diverges into its own "-default"-suffixed
// compose file, volumes, or containers. It must return immediately rather
// than fall through to the registry lookup below: that lookup exists for
// "no --profile flag given at all", and an explicit "default" has to win
// over whatever profile the registry has saved, not be treated as absent.
func ResolveEffectiveProfile(projectPath, explicitProfile string) string {
	explicitProfile = strings.TrimSpace(explicitProfile)
	if explicitProfile == "default" {
		return ""
	}
	if explicitProfile != "" {
		return explicitProfile
	}

	// Fall back to last-used profile from project registry
	if entry, ok := GetProjectRegistryEntry(projectPath); ok {
		profile := strings.TrimSpace(entry.Profile)
		if profile != "" {
			return profile
		}
	}

	return ""
}

func CheckProfileShiftCleanup(config Config) (bool, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, ""
	}

	previousPHP := ""
	previousProfile := ""
	previousVersion := ""

	lockFile, err := ReadLockFile(LockFilePath(cwd))
	if err == nil {
		previousPHP = strings.TrimSpace(lockFile.Stack.PHPVersion)
		previousProfile = strings.TrimSpace(lockFile.Project.Profile)
		previousVersion = strings.TrimSpace(lockFile.Project.FrameworkVersion)
	} else {
		// Fallback to project registry
		if entry, ok := GetProjectRegistryEntry(cwd); ok {
			previousPHP = strings.TrimSpace(entry.PHPVersion)
			previousProfile = strings.TrimSpace(entry.Profile)
			previousVersion = strings.TrimSpace(entry.FrameworkVersion)
		}
	}

	// Normalize: treat empty strings as "unset" (use profile default).
	// When current is empty but previous exists, user likely wants the
	// default PHP version for the profile → NOT a shift.
	currentPHP := strings.TrimSpace(config.Stack.PHPVersion)
	currentProfile := strings.TrimSpace(config.Profile)
	currentVersion := strings.TrimSpace(config.FrameworkVersion)

	prevPHP := strings.TrimSpace(previousPHP)
	if prevPHP != "" && currentPHP != "" && prevPHP != currentPHP {
		return true, fmt.Sprintf("PHP version changed: %s -> %s", prevPHP, currentPHP)
	}

	prevProfile := strings.TrimSpace(previousProfile)
	if prevProfile != "" && currentProfile != "" && prevProfile != currentProfile {
		return true, fmt.Sprintf("Profile changed: %q -> %q", prevProfile, currentProfile)
	}

	prevVersion := strings.TrimSpace(previousVersion)
	if prevVersion != "" && currentVersion != "" && baseVersion(prevVersion) != baseVersion(currentVersion) {
		return true, fmt.Sprintf("Version changed: %s -> %s", prevVersion, currentVersion)
	}

	return false, ""
}
