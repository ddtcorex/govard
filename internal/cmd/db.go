package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var dbCmd = &cobra.Command{
	Use:   "db [connect|import|dump]",
	Short: "Interact with the database container",
	Long: `Manage your project's database. Supports connecting to the container shell,
importing SQL dumps, and creating backups. Works for both local and remote environments.

Case Studies:
- Remote Debugging: Connect directly to the staging database to inspect live data.
- Quick Backup: Create a local SQL dump before performing risky operations.
- Data Refresh: Stream a database dump from production directly into your local DB.
- Sanitized Backup: Use --exclude-sensitive-data to remove DEFINER and GTID from dumps.`,
	Example: `  # Open an interactive MySQL shell locally
  govard db connect

  # Connect to the staging database via SSH tunnel
  govard db connect --environment staging

  # Import a local SQL file
  govard db import --file backup.sql

  # Stream a dump from production into your local database (Wipe local DB first)
  govard db import --stream-db --environment prod

  # Create a database dump with routines and triggers
  govard db dump --full --file my_backup.sql`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		subcommand := strings.ToLower(strings.TrimSpace(args[0]))
		if err := runDBSubcommand(cmd, subcommand); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	dbCmd.Flags().StringP("environment", "e", "local", "Target environment (local, staging, prod, etc.)")
	dbCmd.Flags().StringP("file", "f", "", "Database dump file (import or dump output)")
	dbCmd.Flags().Bool("stream-db", false, "For import: stream dump from remote environment into local database")
	dbCmd.Flags().Bool("full", false, "For dump: include routines, events, and triggers")
	dbCmd.Flags().Bool("exclude-sensitive-data", false, "Apply SQL sanitization pipeline (DEFINER/GTID cleanup)")
}

type DBCommandOptions struct {
	Environment          string
	File                 string
	StreamDB             bool
	Full                 bool
	ExcludeSensitiveData bool
}

type dbCommandOptions = DBCommandOptions

var mysqlDatabaseNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateDBCommandOptions validates DB command option combinations.
func ValidateDBCommandOptions(subcommand string, options DBCommandOptions) error {
	return validateDBCommandOptions(subcommand, options)
}

func runDBSubcommand(cmd *cobra.Command, subcommand string) (err error) {
	startedAt := time.Now()
	operationStatus := engine.OperationStatusFailure
	operationCategory := ""
	operationMessage := ""
	operationConfig := engine.Config{}
	operationSource := ""
	operationDestination := ""
	defer func() {
		if err != nil && operationMessage == "" {
			operationMessage = err.Error()
		}
		if err == nil && operationStatus == engine.OperationStatusFailure {
			operationStatus = engine.OperationStatusSuccess
		}
		if err != nil && operationCategory == "" {
			operationCategory = classifyCommandError(err)
		}
		writeOperationEventBestEffort(
			"db."+subcommand,
			operationStatus,
			operationConfig,
			operationSource,
			operationDestination,
			operationMessage,
			operationCategory,
			time.Since(startedAt),
		)
		if err == nil {
			cwd, _ := os.Getwd()
			trackProjectRegistryBestEffort(operationConfig, cwd, "db-"+subcommand)
		}
	}()

	auditStatus := remote.RemoteAuditStatusFailure
	auditCategory := ""
	auditMessage := ""
	auditRemote := ""
	auditSource := ""
	auditDestination := ""

	options, err := readDBCommandOptions(cmd)
	if err != nil {
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "db." + subcommand,
			Status:     remote.RemoteAuditStatusFailure,
			Category:   "validation",
			DurationMS: time.Since(startedAt).Milliseconds(),
			Message:    err.Error(),
		})
		return err
	}
	if err := validateDBCommandOptions(subcommand, options); err != nil {
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "db." + subcommand,
			Status:     remote.RemoteAuditStatusFailure,
			Category:   "validation",
			Remote:     options.Environment,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Message:    err.Error(),
		})
		return err
	}
	shouldAudit := options.Environment != "local" || options.StreamDB
	if shouldAudit {
		if options.StreamDB {
			auditSource = options.Environment
			auditDestination = "local"
			operationSource = options.Environment
			operationDestination = "local"
		} else {
			auditRemote = options.Environment
			operationDestination = options.Environment
		}
		defer func() {
			if err != nil && auditMessage == "" {
				auditMessage = err.Error()
			}
			if err == nil && auditStatus == remote.RemoteAuditStatusFailure {
				auditStatus = remote.RemoteAuditStatusSuccess
			}
			if err != nil && auditCategory == "" {
				auditCategory = classifyCommandError(err)
			}
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:   "db." + subcommand,
				Status:      auditStatus,
				Category:    auditCategory,
				Remote:      auditRemote,
				Source:      auditSource,
				Destination: auditDestination,
				DurationMS:  time.Since(startedAt).Milliseconds(),
				Message:     auditMessage,
			})
		}()
	}

	config := loadFullConfig()
	operationConfig = config
	switch subcommand {
	case "connect":
		err = runDBConnect(cmd, config, options)
		if err == nil {
			auditStatus = remote.RemoteAuditStatusSuccess
			auditMessage = "db connect completed"
			operationStatus = engine.OperationStatusSuccess
			operationMessage = "db connect completed"
		}
		return err
	case "dump":
		err = runDBDump(cmd, config, options)
		if err == nil {
			auditStatus = remote.RemoteAuditStatusSuccess
			auditMessage = "db dump completed"
			operationStatus = engine.OperationStatusSuccess
			operationMessage = "db dump completed"
		}
		return err
	case "import":
		err = runDBImport(cmd, config, options)
		if err == nil {
			auditStatus = remote.RemoteAuditStatusSuccess
			if options.StreamDB {
				auditMessage = "db stream import completed"
				operationMessage = "db stream import completed"
			} else {
				auditMessage = "db import completed"
				operationMessage = "db import completed"
			}
			operationStatus = engine.OperationStatusSuccess
		}
		return err
	default:
		return fmt.Errorf("unknown db subcommand: %s", subcommand)
	}
}

func readDBCommandOptions(cmd *cobra.Command) (dbCommandOptions, error) {
	environment, err := cmd.Flags().GetString("environment")
	if err != nil {
		return dbCommandOptions{}, err
	}
	file, err := cmd.Flags().GetString("file")
	if err != nil {
		return dbCommandOptions{}, err
	}
	streamDB, err := cmd.Flags().GetBool("stream-db")
	if err != nil {
		return dbCommandOptions{}, err
	}
	full, err := cmd.Flags().GetBool("full")
	if err != nil {
		return dbCommandOptions{}, err
	}
	excludeSensitiveData, err := cmd.Flags().GetBool("exclude-sensitive-data")
	if err != nil {
		return dbCommandOptions{}, err
	}

	return dbCommandOptions{
		Environment:          strings.ToLower(strings.TrimSpace(environment)),
		File:                 strings.TrimSpace(file),
		StreamDB:             streamDB,
		Full:                 full,
		ExcludeSensitiveData: excludeSensitiveData,
	}, nil
}

func validateDBCommandOptions(subcommand string, options dbCommandOptions) error {
	if options.Environment == "" {
		return errors.New("environment cannot be empty")
	}

	switch subcommand {
	case "connect":
		if options.File != "" || options.StreamDB || options.Full || options.ExcludeSensitiveData {
			return errors.New("connect does not support --file, --stream-db, --full, or --exclude-sensitive-data")
		}
	case "dump":
		if options.StreamDB {
			return errors.New("--stream-db is only supported by db import")
		}
	case "import":
		if options.Full {
			return errors.New("--full is only supported by db dump")
		}
		if options.StreamDB && options.Environment == "local" {
			return errors.New("--stream-db requires a remote --environment source")
		}
	default:
		return fmt.Errorf("unknown db subcommand: %s", subcommand)
	}
	return nil
}

func runDBConnect(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	return runDBHooks(config, engine.HookPreDBConnect, engine.HookPostDBConnect, cmd, func() error {
		if options.Environment == "local" {
			containerName := dbContainerName(config)
			if err := ensureLocalDBRunning(containerName); err != nil {
				return err
			}

			credentials := resolveLocalDBCredentials(containerName)
			pterm.Info.Printf("Connecting to database on %s...\n", containerName)
			connectCmd := buildLocalDBConnectCommand(containerName, credentials)
			connectCmd.Stdin, connectCmd.Stdout, connectCmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return connectCmd.Run()
		}

		remoteCfg, err := resolveDBRemote(config, options.Environment, false)
		if err != nil {
			return err
		}
		credentials, probeErr := resolveRemoteDBCredentials(config, options.Environment, remoteCfg)
		if probeErr != nil {
			pterm.Warning.Println(formatRemoteDBProbeWarning(options.Environment, probeErr))
		}
		connectCmd := remote.BuildSSHExecCommand(options.Environment, remoteCfg, true, buildRemoteMySQLConnectCommandString(credentials))
		connectCmd.Stdin, connectCmd.Stdout, connectCmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return connectCmd.Run()
	})
}

func runDBDump(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	return runDBHooks(config, engine.HookPreDBDump, engine.HookPostDBDump, cmd, func() error {
		dumpCommand, err := buildDBDumpCommand(config, options)
		if err != nil {
			return err
		}

		writer := cmd.OutOrStdout()
		var fileWriter *os.File
		if options.File != "" {
			targetPath := filepath.Clean(options.File)
			fileWriter, err = os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("create dump file: %w", err)
			}
			defer fileWriter.Close()
			writer = fileWriter
			pterm.Info.Printf("Writing database dump to %s...\n", targetPath)
		} else {
			pterm.Info.Println("Dumping database to stdout...")
		}

		if err := runDumpToWriter(dumpCommand, writer, options.ExcludeSensitiveData, cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("db dump failed: %w", err)
		}

		if options.File != "" {
			pterm.Success.Printf("Database dump saved to %s.\n", filepath.Clean(options.File))
		}
		return nil
	})
}

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

func runDBHooks(config engine.Config, pre string, post string, cmd *cobra.Command, action func() error) error {
	if err := engine.RunHooks(config, pre, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("%s hooks failed: %w", pre, err)
	}
	if err := action(); err != nil {
		return err
	}
	if err := engine.RunHooks(config, post, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("%s hooks failed: %w", post, err)
	}
	return nil
}

func resolveDBImportReader(options dbCommandOptions) (io.Reader, io.Closer, error) {
	if options.File != "" {
		path := filepath.Clean(options.File)
		file, err := os.Open(path)
		if err != nil {
			return nil, nil, fmt.Errorf("open import file: %w", err)
		}
		return file, file, nil
	}

	if stdinIsTerminal() {
		return nil, nil, errors.New("no import input provided; use --file or pipe SQL via stdin")
	}
	return os.Stdin, nil, nil
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func buildDBDumpCommand(config engine.Config, options dbCommandOptions) (*exec.Cmd, error) {
	if options.Environment == "local" {
		containerName := dbContainerName(config)
		if err := ensureLocalDBRunning(containerName); err != nil {
			return nil, err
		}
		credentials := resolveLocalDBCredentials(containerName)
		return buildLocalDBDumpCommand(containerName, credentials, options.Full), nil
	}

	remoteCfg, err := resolveDBRemote(config, options.Environment, false)
	if err != nil {
		return nil, err
	}
	credentials, probeErr := resolveRemoteDBCredentials(config, options.Environment, remoteCfg)
	if probeErr != nil {
		pterm.Warning.Println(formatRemoteDBProbeWarning(options.Environment, probeErr))
	}
	return remote.BuildSSHExecCommand(options.Environment, remoteCfg, true, buildRemoteMySQLDumpCommandString(credentials, options.Full)), nil
}

func buildDBImportCommand(config engine.Config, options dbCommandOptions) (*exec.Cmd, error) {
	if options.Environment == "local" {
		containerName := dbContainerName(config)
		if err := ensureLocalDBRunning(containerName); err != nil {
			return nil, err
		}
		return buildLocalDBImportCommand(containerName, resolveLocalDBCredentials(containerName)), nil
	}

	remoteCfg, err := resolveDBRemote(config, options.Environment, true)
	if err != nil {
		return nil, err
	}
	credentials, probeErr := resolveRemoteDBCredentials(config, options.Environment, remoteCfg)
	if probeErr != nil {
		pterm.Warning.Println(formatRemoteDBProbeWarning(options.Environment, probeErr))
	}
	return remote.BuildSSHExecCommand(options.Environment, remoteCfg, true, buildRemoteMySQLImportCommandString(credentials)), nil
}

func buildMySQLDumpCommandArgs(full bool) []string {
	return buildMySQLDumpCommandArgsWithCredentials(defaultDBCredentials(), full)
}

func buildMySQLDumpCommandString(full bool) string {
	return buildRemoteMySQLDumpCommandString(defaultDBCredentials(), full)
}

func dbContainerName(config engine.Config) string {
	return fmt.Sprintf("%s-db-1", config.ProjectName)
}

func ensureLocalDBRunning(containerName string) error {
	check := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	output, err := check.Output()
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		return fmt.Errorf("database container %s is not running", containerName)
	}
	return nil
}

func resolveDBRemote(config engine.Config, name string, forWrite bool) (engine.RemoteConfig, error) {
	remoteCfg, err := ensureRemoteKnown(config, name)
	if err != nil {
		return engine.RemoteConfig{}, err
	}
	if !engine.RemoteCapabilityEnabled(remoteCfg, engine.RemoteCapabilityDB) {
		return engine.RemoteConfig{}, fmt.Errorf(
			"remote '%s' does not allow db operations (capabilities: %s)",
			name,
			strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
		)
	}
	if forWrite {
		if blocked, reason := engine.RemoteWriteBlocked(remoteCfg); blocked {
			return engine.RemoteConfig{}, fmt.Errorf("remote '%s' blocks db write operations: %s", name, reason)
		}
	}
	return remoteCfg, nil
}

func runDumpToWriter(dumpCmd *exec.Cmd, writer io.Writer, sanitize bool, stderr io.Writer) error {
	stdout, err := dumpCmd.StdoutPipe()
	if err != nil {
		return err
	}
	dumpCmd.Stderr = stderr
	if err := dumpCmd.Start(); err != nil {
		return err
	}

	var copyErr error
	if sanitize {
		copyErr = engine.SanitizeSQLDump(stdout, writer)
	} else {
		_, copyErr = io.Copy(writer, stdout)
	}

	waitErr := dumpCmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	return waitErr
}

func RunImportFromReader(importCmd *exec.Cmd, reader io.Reader, sanitize bool, stdout io.Writer, stderr io.Writer) error {
	stdin, err := importCmd.StdinPipe()
	if err != nil {
		return err
	}
	importCmd.Stdout = stdout
	importCmd.Stderr = stderr
	if err := importCmd.Start(); err != nil {
		return err
	}

	var copyErr error
	if sanitize {
		copyErr = engine.SanitizeSQLDump(reader, stdin)
	} else {
		_, copyErr = io.Copy(stdin, reader)
	}

	closeErr := stdin.Close()
	waitErr := importCmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return waitErr
}

func RunDumpToImport(dumpCmd *exec.Cmd, importCmd *exec.Cmd, sanitize bool, stdout io.Writer, stderr io.Writer) error {
	dumpStdout, err := dumpCmd.StdoutPipe()
	if err != nil {
		return err
	}
	importStdin, err := importCmd.StdinPipe()
	if err != nil {
		return err
	}

	dumpCmd.Stderr = stderr
	importCmd.Stdout = stdout
	importCmd.Stderr = stderr

	if err := dumpCmd.Start(); err != nil {
		return err
	}
	if err := importCmd.Start(); err != nil {
		if dumpCmd.Process != nil {
			_ = dumpCmd.Process.Kill()
		}
		_ = dumpCmd.Wait()
		return err
	}

	var copyErr error
	if sanitize {
		copyErr = engine.SanitizeSQLDump(dumpStdout, importStdin)
	} else {
		_, copyErr = io.Copy(importStdin, dumpStdout)
	}

	closeErr := importStdin.Close()
	dumpErr := dumpCmd.Wait()
	importErr := importCmd.Wait()

	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if dumpErr != nil {
		return dumpErr
	}
	return importErr
}

func classifyCommandError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unknown remote"),
		strings.Contains(message, "requires"),
		strings.Contains(message, "does not support"),
		strings.Contains(message, "does not allow"),
		strings.Contains(message, "blocks db write operations"),
		strings.Contains(message, "environment cannot be empty"),
		strings.Contains(message, "database container"):
		return "validation"
	default:
		return remote.ClassifyFailure(err, message).Category
	}
}
