package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"govard/internal/engine"

	"github.com/pterm/pterm"
)

// describeLockReadError turns the raw read failure into the instruction the
// operator needs. The exit code stays non-zero on purpose: a check that could
// not check must not pass in CI, but "read lock file: open ... no such file or
// directory" leaves out the one thing they have to do next.
func describeLockReadError(lockPath string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no govard.lock in %s; run 'govard lock generate' to create one", filepath.Dir(lockPath))
	}
	return err
}

func evaluateUpLockPolicy(cwd string, config engine.Config, update bool) ([]string, error) {
	lockPath := engine.LockFilePath(cwd)
	expected, err := engine.ReadLockFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if !config.Lock.Strict {
				return nil, nil
			}
			warning := fmt.Sprintf("Lock strict mode: required lock file missing at %s", lockPath)
			return []string{warning}, fmt.Errorf("lock strict mode enabled: missing %s (run `govard lock generate`)", lockPath)
		}
		// Any other error (e.g. permission or corruption) should still be a warning/error.
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
	warnings := buildUpLockWarnings(expected, current, config.Lock.IgnoreFields)
	if len(warnings) == 0 {
		return nil, nil
	}
	if !update {
		if !config.Lock.Strict {
			return warnings, nil
		}
		return warnings, fmt.Errorf("lock strict mode enabled: found %d mismatch(es); run `govard lock diff` for details or use `govard env up --update-lock` to sync", len(warnings))
	}

	pterm.Info.Println("Auto-updating lock file due to --update-lock flag...")
	if err := engine.WriteLockFile(lockPath, current); err != nil {
		return warnings, fmt.Errorf("failed to auto-update lock file: %w", err)
	}
	pterm.Success.Println("Lock file updated.")
	return nil, nil
}

func buildUpLockWarnings(expected engine.LockFile, current engine.LockFile, ignoreFields []string) []string {
	result := engine.CompareLockFile(expected, current, ignoreFields)
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
func BuildUpLockWarningsForTest(expected engine.LockFile, current engine.LockFile, ignoreFields []string) []string {
	return buildUpLockWarnings(expected, current, ignoreFields)
}

// EvaluateUpLockPolicyForTest exposes lock policy evaluation for tests.
func EvaluateUpLockPolicyForTest(cwd string, config engine.Config, update bool) ([]string, error) {
	return evaluateUpLockPolicy(cwd, config, update)
}
