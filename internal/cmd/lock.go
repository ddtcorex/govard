package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var lockDependencies = engine.LockDependencies{}

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Manage project lock file",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var lockGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate govard.lock from current environment",
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		startedAt := time.Now()
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		cwd, _ := os.Getwd()
		status := engine.OperationStatusFailure
		message := ""
		category := ""
		defer func() {
			if err != nil && message == "" {
				message = err.Error()
			}
			if err == nil {
				status = engine.OperationStatusSuccess
				if message == "" {
					message = "lock file generated"
				}
			} else {
				category = classifyCommandError(err)
			}
			writeOperationEventBestEffort(
				"lock.generate",
				status,
				config,
				"",
				"",
				message,
				category,
				time.Since(startedAt),
			)
			if err == nil {
				trackProjectRegistryBestEffort(config, cwd, "lock-generate")
			}
		}()

		lockPath, err := resolveLockPathFromFlag(cmd, cwd)
		if err != nil {
			return err
		}

		lockfile, err := engine.BuildLockFileFromConfig(cwd, config, Version, lockDependencies)
		if err != nil {
			return err
		}
		if err := engine.WriteLockFile(lockPath, lockfile); err != nil {
			return err
		}

		pterm.Success.Printf("Generated lock file: %s\n", lockPath)
		message = "lock file generated"
		return nil
	},
}

var lockCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Compare current environment with govard.lock",
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		startedAt := time.Now()
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		cwd, _ := os.Getwd()
		status := engine.OperationStatusFailure
		message := ""
		category := ""
		defer func() {
			if err != nil && message == "" {
				message = err.Error()
			}
			if err == nil {
				status = engine.OperationStatusSuccess
				if message == "" {
					message = "lock check passed"
				}
			} else {
				category = classifyCommandError(err)
			}
			writeOperationEventBestEffort(
				"lock.check",
				status,
				config,
				"",
				"",
				message,
				category,
				time.Since(startedAt),
			)
			if err == nil {
				trackProjectRegistryBestEffort(config, cwd, "lock-check")
			}
		}()

		lockPath, err := resolveLockPathFromFlag(cmd, cwd)
		if err != nil {
			return err
		}

		expected, err := engine.ReadLockFile(lockPath)
		if err != nil {
			return err
		}
		current, err := engine.BuildLockFileFromConfig(cwd, config, Version, lockDependencies)
		if err != nil {
			return err
		}

		warnings := buildUpLockWarnings(expected, current)
		if len(warnings) == 0 {
			pterm.Success.Printf("Lock check passed: %s\n", lockPath)
			message = "lock check passed"
			return nil
		}

		for _, warning := range warnings {
			pterm.Warning.Println(warning)
		}
		message = fmt.Sprintf("lock check found %d mismatch(es)", len(warnings))
		return errors.New(message)
	},
}

func init() {
	lockGenerateCmd.Flags().String("file", "", "Path to lock file (default: ./govard.lock)")
	lockCheckCmd.Flags().String("file", "", "Path to lock file (default: ./govard.lock)")

	lockCmd.AddCommand(lockGenerateCmd)
	lockCmd.AddCommand(lockCheckCmd)
}

func resolveLockPathFromFlag(cmd *cobra.Command, cwd string) (string, error) {
	if cmd == nil {
		return engine.LockFilePath(cwd), nil
	}
	path, err := cmd.Flags().GetString("file")
	if err != nil {
		return "", err
	}
	if path == "" {
		return engine.LockFilePath(cwd), nil
	}
	return path, nil
}

// SetLockDependenciesForTest swaps lock dependencies and returns a restore callback.
func SetLockDependenciesForTest(deps engine.LockDependencies) func() {
	previous := lockDependencies
	lockDependencies = deps
	return func() {
		lockDependencies = previous
	}
}
