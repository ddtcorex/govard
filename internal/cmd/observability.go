package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"
)

func trackProjectRegistry(config engine.Config, cwd string, command string) error {
	projectRoot := strings.TrimSpace(cwd)
	if projectRoot == "" {
		value, err := os.Getwd()
		if err == nil {
			projectRoot = value
		}
	}
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}

	// previous_profile is only ever set by `config profile switch`/`clear`; preserve it here
	// so unrelated commands (sync, lock, db, bootstrap, remote, tunnel) don't erase the
	// bookkeeping that lets `env up` detect and warn about a profile shift.
	previousProfile := ""
	if existing, ok := engine.GetProjectRegistryEntry(projectRoot); ok {
		previousProfile = existing.PreviousProfile
	}

	entry := engine.ProjectRegistryEntry{
		Path:             projectRoot,
		ProjectName:      normalizeProjectName(config.ProjectName, projectRoot),
		Domain:           strings.TrimSpace(config.Domain),
		ExtraDomains:     config.ExtraDomains,
		Framework:        strings.TrimSpace(config.Framework),
		LastCommand:      strings.TrimSpace(command),
		LastSeenAt:       time.Now().UTC(),
		PHPVersion:       strings.TrimSpace(config.Stack.PHPVersion),
		Profile:          strings.TrimSpace(config.Profile),
		PreviousProfile:  previousProfile,
		FrameworkVersion: strings.TrimSpace(config.FrameworkVersion),
	}
	return engine.UpsertProjectRegistryEntry(entry)
}

func writeOperationEvent(
	operation string,
	status engine.OperationStatus,
	config engine.Config,
	source string,
	destination string,
	message string,
	category string,
	duration time.Duration,
) error {
	event := engine.OperationEvent{
		Operation:   strings.TrimSpace(operation),
		Status:      status,
		Project:     strings.TrimSpace(config.ProjectName),
		Source:      strings.TrimSpace(source),
		Destination: strings.TrimSpace(destination),
		Category:    strings.TrimSpace(category),
		Message:     strings.TrimSpace(message),
	}
	if event.Project == "" {
		if cwd, err := os.Getwd(); err == nil {
			event.Project = filepath.Base(cwd)
		}
	}
	if duration > 0 {
		event.DurationMS = duration.Milliseconds()
	}
	return engine.WriteOperationEvent(event)
}

func trackProjectRegistryBestEffort(config engine.Config, cwd string, command string) {
	_ = trackProjectRegistry(config, cwd, command)
}

func writeOperationEventBestEffort(
	operation string,
	status engine.OperationStatus,
	config engine.Config,
	source string,
	destination string,
	message string,
	category string,
	duration time.Duration,
) {
	_ = writeOperationEvent(operation, status, config, source, destination, message, category, duration)
}

func normalizeProjectName(projectName string, projectRoot string) string {
	value := strings.TrimSpace(projectName)
	if value != "" {
		return value
	}
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

// TrackProjectRegistryForTest exposes project registry tracking for tests.
func TrackProjectRegistryForTest(config engine.Config, cwd string, command string) error {
	return trackProjectRegistry(config, cwd, command)
}

// WriteOperationEventForTest exposes operation event writing for tests.
func WriteOperationEventForTest(
	operation string,
	status engine.OperationStatus,
	config engine.Config,
	source string,
	destination string,
	message string,
	category string,
	duration time.Duration,
) error {
	return writeOperationEvent(operation, status, config, source, destination, message, category, duration)
}
