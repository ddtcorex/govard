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
	"govard/internal/ui"

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
	if err := checkPrivacyFilter(config.Framework, remoteCredentials.TablePrefix, options.NoNoise, options.NoPII, false); err != nil {
		return err
	}
	localCredentials := resolveLocalDBCredentials(config, containerName)
	localPoller := &finalizePoller{config: config, remoteName: "local", credentials: localCredentials, noNoise: options.NoNoise, noPII: options.NoPII}
	sourceDumpCmd := sshScriptCommand(options.Environment, remoteCfg, buildRemoteMySQLDumpCommandString(remoteCredentials, options.NoNoise, options.NoPII, config.Framework, true))
	destinationImportCmd := buildLocalDBImportCommand(containerName, localCredentials)
	sanitizeStreamDump := options.StreamDB
	var totalSize int64
	spinner := ui.StartSpinner(cmd.ErrOrStderr(), fmt.Sprintf("Fetching remote database size for '%s'...", options.Environment))
	if size, sizeErr := GetDatabaseSize(config, options.Environment, remoteCfg, remoteCredentials, options.NoNoise, options.NoPII); sizeErr == nil {
		totalSize = size
		spinner.Success()
	} else {
		spinner.Warning(fmt.Sprintf("could not determine remote database size: %v", sizeErr))
	}

	// A reset destroys the database this import is about to fill, and --file
	// overwrites a backup. Neither may depend on a dump that has not arrived
	// yet, so both are staged through a file: the dump completes first, then the
	// target file or the reset touches local data, then the import reads it.
	if !options.Drop && options.File == "" {
		if err := RunDumpToImportWithProgress(sourceDumpCmd, destinationImportCmd, totalSize, sanitizeStreamDump, cmd.OutOrStdout(), cmd.ErrOrStderr(), localPoller.size); err != nil {
			return fmt.Errorf("stream-db import failed: %w", err)
		}
		pterm.Success.Printf("stream import completed from remote '%s' into local database.\n", options.Environment)
		return nil
	}

	// The prompt comes before the download: asking after a full transfer spends
	// the transfer on an operator who may then say no.
	if options.Drop && !options.AssumeYes {
		if !stdinIsTerminal() {
			return errors.New("confirmation required to drop and recreate the local database; use -y to assume yes in non-interactive environments")
		}
		confirmed, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).WithDefaultText("Are you sure you want to drop and recreate the local database?").Show()
		if !confirmed {
			return errors.New("stream-db import cancelled by user")
		}
	}

	targetPath := filepath.Clean(options.File)
	temporary := options.File == ""
	if temporary {
		// The staged dump holds a full database, so it goes under govard's own
		// temporary directory rather than the OS temp: on most Linux hosts /tmp
		// is a tmpfs, and a database written there is RAM.
		tempDir := filepath.Join(engine.GovardHomeDir(), "tmp")
		if err := os.MkdirAll(tempDir, conventions.SecretDirPerm); err != nil {
			return fmt.Errorf("create stream dump directory: %w", err)
		}
		sweepStaleStreamDumps(tempDir, staleStreamDumpAge)
		tempFile, tempErr := os.CreateTemp(tempDir, "stream-db-*.sql")
		if tempErr != nil {
			return fmt.Errorf("create temporary stream dump file: %w", tempErr)
		}
		targetPath = tempFile.Name()
		if closeErr := tempFile.Close(); closeErr != nil {
			return fmt.Errorf("close temporary stream dump file: %w", closeErr)
		}
		defer os.Remove(targetPath)
	}

	// A staged dump left behind by a killed run is harmless and short lived: the
	// next run truncates its own .partial before writing, and the target file keeps
	// its previous contents until a dump completes.
	if err := stageDumpThenReset(
		func() error { return writeDumpFile(sourceDumpCmd, targetPath, cmd.ErrOrStderr()) },
		func() error {
			if !options.Drop {
				return nil
			}
			if err := resetLocalDatabase(containerName, localCredentials); err != nil {
				return err
			}
			pterm.Success.Println("Local database reset successful.")
			return nil
		},
	); err != nil {
		return err
	}

	fileReader, err := os.Open(targetPath)
	if err != nil {
		return fmt.Errorf("open stream dump file: %w", err)
	}
	defer fileReader.Close()

	if err := RunImportFromReaderWithProgress(destinationImportCmd, fileReader, totalSize, false, cmd.OutOrStdout(), cmd.ErrOrStderr(), localPoller.size); err != nil {
		return fmt.Errorf("stream-db local import step failed: %w", err)
	}

	if temporary {
		pterm.Success.Printf("stream import completed from remote '%s' into local database.\n", options.Environment)
		return nil
	}
	pterm.Success.Printf("stream import completed from remote '%s' via file %s.\n", options.Environment, targetPath)
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
	return conventions.DefaultDBName
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
	// The write-protection gate answers before anything destructive runs. The
	// --drop block below resets the LOCAL database, so a protected remote has to
	// be refused first: otherwise the command destroys local data and only then
	// reports that it was never going to import, and --drop's confirmation reads
	// as consent to a reset that had no business happening.
	if options.Environment != "local" {
		if _, err := resolveDBRemote(config, options.Environment, true); err != nil {
			return err
		}
	}

	// The reader comes before the command: buildDBImportCommand probes the local
	// container (or the remote credentials), and probing first meant a typo in
	// --file surfaced only after the database had already been dropped.
	reader, closer, totalSize, err := resolveDBImportReader(options)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}

	importCommand, poller, err := buildDBImportCommand(config, options)
	if err != nil {
		return err
	}

	if reader == os.Stdin {
		pterm.Description.Println("Tip: cat backup.sql | govard db import")
	}

	// The reset runs last, after every check that can fail without side effects:
	// the input file is open, the import command and its credentials are built.
	// Anything that can fail before this point does so with the database intact.
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

	if err := RunImportFromReaderWithProgress(importCommand, reader, totalSize, false, cmd.OutOrStdout(), cmd.ErrOrStderr(), poller.size); err != nil {
		return fmt.Errorf("db import failed: %w", err)
	}
	pterm.Success.Println("Database import completed.")
	return nil
}

// stageDumpThenReset encodes the one ordering rule of the stream path: the dump
// completes before anything destructive runs, so a failed or dropped dump leaves
// the existing database untouched. reset may be nil, which is the --file case
// without --drop.
func stageDumpThenReset(dump func() error, reset func() error) error {
	if err := dump(); err != nil {
		return err
	}
	if reset == nil {
		return nil
	}
	return reset()
}

// staleStreamDumpAge is how old a staged stream dump must be before the next
// stream import treats it as an orphan. A live import finishes in minutes; a day
// leaves room for the largest databases without keeping a dead run's dump for long.
const staleStreamDumpAge = 24 * time.Hour

// sweepStaleStreamDumps removes staged stream dumps older than olderThan from dir.
// A normal exit and an error return remove their own dump through a defer, but a
// process that dies without running defers (SIGHUP, SIGKILL, power loss) leaves a
// full database behind under a random name no later run would reuse. Best effort:
// a file that cannot be removed is left for the next run.
func sweepStaleStreamDumps(dir string, olderThan time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		name := entry.Name()
		stagedDump := strings.HasPrefix(name, "stream-db-") &&
			(strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".sql.partial"))
		if entry.IsDir() || !stagedDump {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}
