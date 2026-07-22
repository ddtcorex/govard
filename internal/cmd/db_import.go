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

	"govard/internal/conventions"
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
	localCredentials := resolveLocalDBCredentials(config, containerName)
	localPoller := &finalizePoller{config: config, remoteName: "local", credentials: localCredentials, noNoise: options.NoNoise, noPII: options.NoPII}
	if options.Drop {
		if !options.AssumeYes {
			if !stdinIsTerminal() {
				return errors.New("confirmation required to drop and recreate the local database; use -y to assume yes in non-interactive environments")
			}
			confirmed, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).WithDefaultText("Are you sure you want to drop and recreate the local database?").Show()
			if !confirmed {
				return errors.New("stream-db import cancelled by user")
			}
		}

		if err := resetLocalDatabase(containerName, localCredentials); err != nil {
			return err
		}
		pterm.Success.Println("Local database reset successful.")
	}

	sourceDumpCmd := remote.BuildSSHExecCommand(options.Environment, remoteCfg, true, buildRemoteMySQLDumpCommandString(remoteCredentials, options.NoNoise, options.NoPII, config.Framework, true))
	destinationImportCmd := buildLocalDBImportCommand(containerName, localCredentials)
	sanitizeStreamDump := options.StreamDB
	var totalSize int64
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("Fetching remote database size for '%s'...", options.Environment))
	if size, sizeErr := GetDatabaseSize(config, options.Environment, remoteCfg, remoteCredentials, options.NoNoise, options.NoPII); sizeErr == nil {
		totalSize = size
		spinner.Success()
	} else {
		spinner.Warning(fmt.Sprintf("could not determine remote database size: %v", sizeErr))
	}

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

		if err := RunImportFromReaderWithProgress(destinationImportCmd, fileReader, totalSize, false, cmd.OutOrStdout(), cmd.ErrOrStderr(), localPoller.size); err != nil {
			return fmt.Errorf("stream-db local import step failed: %w", err)
		}

		pterm.Success.Printf("stream import completed from remote '%s' via file %s.\n", options.Environment, targetPath)
		return nil
	}

	if err := RunDumpToImportWithProgress(sourceDumpCmd, destinationImportCmd, totalSize, sanitizeStreamDump, cmd.OutOrStdout(), cmd.ErrOrStderr(), localPoller.size); err != nil {
		return fmt.Errorf("stream-db import failed: %w", err)
	}
	pterm.Success.Printf("stream import completed from remote '%s' into local database.\n", options.Environment)
	return nil
}

func resetLocalDatabase(containerName string, credentials dbCredentials) error {
	script, err := buildLocalDBResetScript(credentials)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resetCmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "sh", "-lc", script)
	output, err := resetCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timed out resetting local database %s", credentials.Database)
		}
		return fmt.Errorf("failed to reset local database %s (%v): %s", credentials.Database, err, strings.TrimSpace(string(output)))
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

func buildLocalDBResetScript(credentials dbCredentials) (string, error) {
	if credentials.Engine == conventions.ServicePostgreSQL {
		return buildLocalPostgresResetScript(credentials)
	}

	name := normalizeDatabaseName(credentials.Database)
	if err := validateDatabaseName(name); err != nil {
		return "", err
	}

	killSQL := fmt.Sprintf("SELECT id FROM information_schema.processlist WHERE db='%s' AND id<>CONNECTION_ID()", strings.ReplaceAll(name, "'", "''"))
	resetSQL := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s`;", name, name)
	return strings.Join([]string{
		"set -e",
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else echo "mysql client not found (mysql/mariadb)" >&2; exit 127; fi`,
		"IDS=$($DB_CLI -uroot -proot -N -e " + engine.ShellQuote(killSQL) + " 2>/dev/null || true)",
		`for id in $IDS; do $DB_CLI -uroot -proot -e "KILL $id" 2>/dev/null || true; done`,
		"$DB_CLI -uroot -proot -e " + engine.ShellQuote(resetSQL),
	}, " && "), nil
}

func BuildLocalDBResetScriptForTest(database string) (string, error) {
	return buildLocalDBResetScript(dbCredentials{Database: database})
}

func runDirectDBImport(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	if options.Drop {
		if !options.AssumeYes {
			if !stdinIsTerminal() {
				return errors.New("confirmation required to drop and recreate the database; use -y to assume yes in non-interactive environments")
			}
			confirmed, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).WithDefaultText("Are you sure you want to drop and recreate the database?").Show()
			if !confirmed {
				return errors.New("database import cancelled by user")
			}
		}

		containerName := dbContainerName(config)
		if err := ensureLocalDBRunning(containerName); err != nil {
			return err
		}
		credentials := resolveLocalDBCredentials(config, containerName)
		script, err := buildLocalDBResetScript(credentials)
		if err != nil {
			return err
		}

		pterm.Info.Printf("Resetting database '%s'...\n", credentials.Database)
		out, err := exec.Command("docker", "exec", containerName, "sh", "-c", script).CombinedOutput()
		if err != nil {
			return fmt.Errorf("database reset failed: %w\nOutput: %s", err, string(out))
		}
		pterm.Success.Println("Database reset successful.")
	}

	importCommand, poller, err := buildDBImportCommand(config, options)
	if err != nil {
		return err
	}

	reader, closer, totalSize, err := resolveDBImportReader(options)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}

	if reader == os.Stdin {
		pterm.Description.Println("Tip: cat backup.sql | govard db import")
	}

	if err := RunImportFromReaderWithProgress(importCommand, reader, totalSize, false, cmd.OutOrStdout(), cmd.ErrOrStderr(), poller.size); err != nil {
		return fmt.Errorf("db import failed: %w", err)
	}
	pterm.Success.Println("Database import completed.")
	return nil
}
