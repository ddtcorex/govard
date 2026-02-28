package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runDBImport(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	return runDBHooks(config, engine.HookPreDBImport, engine.HookPostDBImport, cmd, func() error {
		if options.StreamDB {
			return runStreamDBImport(cmd, config, options)
		}
		return runDirectDBImport(cmd, config, options)
	})
}

func runStreamDBImport(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	remoteCfg, err := resolveDBRemote(config, options.Environment, false)
	if err != nil {
		return err
	}

	containerName := dbContainerName(config)
	if err := ensureLocalDBRunning(containerName); err != nil {
		return err
	}

	remoteCredentials, probeErr := resolveRemoteDBCredentials(config, options.Environment, remoteCfg)
	if probeErr != nil {
		pterm.Warning.Println(formatRemoteDBProbeWarning(options.Environment, probeErr))
	}
	localCredentials := resolveLocalDBCredentials(containerName)
	if err := resetLocalDatabase(containerName, localCredentials.Database); err != nil {
		return err
	}

	sourceDumpCmd := remote.BuildSSHExecCommand(options.Environment, remoteCfg, true, buildRemoteMySQLDumpCommandString(remoteCredentials, options.Full))
	destinationImportCmd := buildLocalDBImportCommand(containerName, localCredentials)
	sanitizeStreamDump := options.ExcludeSensitiveData || options.StreamDB

	if options.File != "" {
		targetPath := filepath.Clean(options.File)
		dumpFile, err := os.Create(targetPath)
		if err != nil {
			return fmt.Errorf("create stream dump file: %w", err)
		}
		if err := runDumpToWriter(sourceDumpCmd, dumpFile, sanitizeStreamDump, cmd.ErrOrStderr()); err != nil {
			_ = dumpFile.Close()
			return fmt.Errorf("stream-db dump step failed: %w", err)
		}
		if err := dumpFile.Close(); err != nil {
			return fmt.Errorf("close stream dump file: %w", err)
		}

		fileReader, err := os.Open(targetPath)
		if err != nil {
			return fmt.Errorf("open stream dump file: %w", err)
		}
		defer fileReader.Close()

		if err := RunImportFromReader(destinationImportCmd, fileReader, false, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("stream-db local import step failed: %w", err)
		}

		pterm.Success.Printf("stream import completed from remote '%s' via file %s.\n", options.Environment, targetPath)
		return nil
	}

	if err := RunDumpToImport(sourceDumpCmd, destinationImportCmd, sanitizeStreamDump, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("stream-db import failed: %w", err)
	}
	pterm.Success.Printf("stream import completed from remote '%s' into local database.\n", options.Environment)
	return nil
}

func resetLocalDatabase(containerName string, database string) error {
	name := normalizeDatabaseName(database)
	script, err := buildLocalDBResetScript(name)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resetCmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "sh", "-lc", script)
	output, err := resetCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out resetting local database %s", name)
		}
		return fmt.Errorf("failed to reset local database %s (%v): %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func normalizeDatabaseName(database string) string {
	name := strings.TrimSpace(database)
	if name != "" {
		return name
	}
	return "magento"
}

func validateDatabaseName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("invalid database name: value cannot be empty")
	}
	if len(trimmed) > 64 {
		return fmt.Errorf("invalid database name %q: exceeds 64 characters", trimmed)
	}
	if !mysqlDatabaseNamePattern.MatchString(trimmed) {
		return fmt.Errorf("invalid database name %q: only letters, numbers, underscore, and hyphen are allowed", trimmed)
	}
	return nil
}

func buildLocalDBResetScript(database string) (string, error) {
	name := normalizeDatabaseName(database)
	if err := validateDatabaseName(name); err != nil {
		return "", err
	}

	// Kill any sessions using the target DB so DROP DATABASE does not block on metadata locks.
	killSQL := fmt.Sprintf("SELECT id FROM information_schema.processlist WHERE db='%s' AND id<>CONNECTION_ID()", strings.ReplaceAll(name, "'", "''"))
	resetSQL := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s`;", name, name)
	return strings.Join([]string{
		"set -e",
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else echo "mysql client not found (mysql/mariadb)" >&2; exit 127; fi`,
		"IDS=$($DB_CLI -uroot -proot -N -e " + shellQuote(killSQL) + " 2>/dev/null || true)",
		`for id in $IDS; do $DB_CLI -uroot -proot -e "KILL $id" 2>/dev/null || true; done`,
		"$DB_CLI -uroot -proot -e " + shellQuote(resetSQL),
	}, " && "), nil
}

func BuildLocalDBResetScriptForTest(database string) (string, error) {
	return buildLocalDBResetScript(database)
}

func runDirectDBImport(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	importCommand, err := buildDBImportCommand(config, options)
	if err != nil {
		return err
	}

	reader, closer, err := resolveDBImportReader(options)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}

	if reader == os.Stdin {
		pterm.Description.Println("Tip: cat backup.sql | govard db import")
	}

	if err := RunImportFromReader(importCommand, reader, options.ExcludeSensitiveData, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("db import failed: %w", err)
	}
	pterm.Success.Println("Database import completed.")
	return nil
}
