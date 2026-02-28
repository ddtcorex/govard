package cmd

import (
	"fmt"
	"os"

	"govard/internal/engine"
)

func evaluateUpLockPolicy(cwd string, config engine.Config) ([]string, error) {
	lockPath := engine.LockFilePath(cwd)
	expected, err := engine.ReadLockFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			if !config.Lock.Strict {
				return nil, nil
			}
			warning := fmt.Sprintf("Lock strict mode: required lock file missing at %s", lockPath)
			return []string{warning}, fmt.Errorf("lock strict mode enabled: missing %s (run `govard lock generate`)", lockPath)
		}
		warning := fmt.Sprintf("Lock check skipped: %v", err)
		if !config.Lock.Strict {
			return []string{warning}, nil
		}
		return []string{warning}, fmt.Errorf("lock strict mode enabled: %w", err)
	}

	current, err := engine.BuildLockFileFromConfig(cwd, config, Version, lockDependencies)
	if err != nil {
		warning := fmt.Sprintf("Lock check skipped: %v", err)
		if !config.Lock.Strict {
			return []string{warning}, nil
		}
		return []string{warning}, fmt.Errorf("lock strict mode enabled: %w", err)
	}
	warnings := buildUpLockWarnings(expected, current)
	if len(warnings) == 0 {
		return nil, nil
	}
	if !config.Lock.Strict {
		return warnings, nil
	}
	return warnings, fmt.Errorf("lock strict mode enabled: found %d mismatch(es); run `govard lock check`", len(warnings))
}

func buildUpLockWarnings(expected engine.LockFile, current engine.LockFile) []string {
	result := engine.CompareLockFile(expected, current)
	if result.Compliant {
		return nil
	}
	warnings := make([]string, 0, len(result.Mismatches))
	for _, mismatch := range result.Mismatches {
		warnings = append(warnings, "Lockfile mismatch: "+mismatch)
	}
	return warnings
}

// BuildUpLockWarningsForTest exposes lock warning rendering for tests.
func BuildUpLockWarningsForTest(expected engine.LockFile, current engine.LockFile) []string {
	return buildUpLockWarnings(expected, current)
}

// EvaluateUpLockPolicyForTest exposes lock policy evaluation for tests.
func EvaluateUpLockPolicyForTest(cwd string, config engine.Config) ([]string, error) {
	return evaluateUpLockPolicy(cwd, config)
}
