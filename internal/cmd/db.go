package cmd

import (
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
	Use:   "db [connect|import|dump|query|info]",
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
  govard db dump --full --file my_backup.sql

  # Execute a SQL query
  govard db query "SELECT * FROM core_config_data LIMIT 5"

  # Show database connection info
  govard db info`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires at least one argument (subcommand)")
		}
		subcommand := strings.ToLower(strings.TrimSpace(args[0]))
		if subcommand == "query" && len(args) < 2 {
			return errors.New("query subcommand requires a SQL query argument")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		subcommand := strings.ToLower(strings.TrimSpace(args[0]))
		if err := runDBSubcommand(cmd, subcommand, args[1:]); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	dbCmd.Flags().StringP("environment", "e", "local", "Target environment (local, staging, prod, etc.)")
	dbCmd.Flags().StringP("file", "f", "", "Database dump file (import or dump output)")
	dbCmd.Flags().String("profile", "", "Environment scope (profile) to use")
	dbCmd.Flags().Bool("stream-db", false, "For import: stream dump from remote environment into local database")
	dbCmd.Flags().Bool("full", false, "For dump: include routines, events, and triggers")
	dbCmd.Flags().Bool("exclude-sensitive-data", false, "Apply SQL sanitization pipeline (DEFINER/GTID cleanup)")
}

type DBCommandOptions struct {
	Environment          string
	File                 string
	Profile              string
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

func runDBSubcommand(cmd *cobra.Command, subcommand string, extraArgs []string) (err error) {
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

	config, err := loadFullConfigWithProfile(options.Profile)
	if err != nil {

		return err
	}
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
	case "query":
		err = runDBQuery(cmd, config, options, extraArgs)
		if err == nil {
			auditStatus = remote.RemoteAuditStatusSuccess
			auditMessage = "db query completed"
			operationStatus = engine.OperationStatusSuccess
			operationMessage = "db query completed"
		}
		return err
	case "info":
		err = runDBInfo(cmd, config, options)
		if err == nil {
			auditStatus = remote.RemoteAuditStatusSuccess
			auditMessage = "db info completed"
			operationStatus = engine.OperationStatusSuccess
			operationMessage = "db info completed"
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
	profile, _ := cmd.Flags().GetString("profile")
	return dbCommandOptions{
		Environment:          strings.ToLower(strings.TrimSpace(environment)),
		File:                 strings.TrimSpace(file),
		Profile:              profile,
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
	case "query", "info":
		if options.File != "" || options.StreamDB || options.Full || options.ExcludeSensitiveData {
			return errors.New("query and info do not support --file, --stream-db, --full, or --exclude-sensitive-data")
		}
	default:
		return fmt.Errorf("unknown db subcommand: %s", subcommand)
	}
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
	if !forWrite {
		if blocked, reason := engine.RemoteWriteBlocked(name, remoteCfg); blocked {
			return engine.RemoteConfig{}, fmt.Errorf("remote environment '%s' is write-protected: %s", name, reason)
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
