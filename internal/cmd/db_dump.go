package cmd

import (
	"compress/gzip"
	"fmt"
	"govard/internal/conventions"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runDBDump(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	return runDBHooks(config, engine.HookPreDBDump, engine.HookPostDBDump, cmd, func() error {
		dumpCommand, remoteFilePath, err := buildDBDumpCommand(config, options)
		if err != nil {
			return err
		}

		// Determination if we are dumping to a local file or remote file
		// If Environment is remote and !Local, we are dumping on the remote server
		isRemoteStorage := options.Environment != "local" && !options.Local

		if isRemoteStorage {
			pterm.Info.Printf("Executing database dump on remote environment '%s'...\n", options.Environment)
			output, err := dumpCommand.CombinedOutput()
			if err != nil {
				return fmt.Errorf("remote db dump failed: %w\nOutput: %s", err, string(output))
			}
			// We don't have the final filename easily here if it was defaulted in buildDBDumpCommand
			// but we can at least show success.
			// Actually, let's fix buildDBDumpCommand to return the filename or just rely on Warden-like patterns.
			pterm.Success.Printf("Database dump completed on remote environment '%s' at '%s'.\n", options.Environment, remoteFilePath)
			return nil
		}

		var writer io.Writer
		var fileWriter *os.File

		targetFile := options.File
		if targetFile == "" {
			suffix := "sql.gz"
			timestamp := time.Now().Format("20060102T150405")
			targetFile = filepath.Join("var", fmt.Sprintf("%s_%s-%s.%s", config.ProjectName, options.Environment, timestamp, suffix))
		}

		targetPath := filepath.Clean(targetFile)
		// Ensure the directory exists (e.g. var/)
		if err := os.MkdirAll(filepath.Dir(targetPath), conventions.DefaultDirPerm); err != nil {
			return fmt.Errorf("create dump directory: %w", err)
		}

		fileWriter, err = createPrivateDumpFile(targetPath)
		if err != nil {
			return fmt.Errorf("create dump file: %w", err)
		}
		defer fileWriter.Close()

		writer = fileWriter

		var gzWriter *gzip.Writer
		if strings.HasSuffix(targetPath, ".gz") {
			gzWriter = gzip.NewWriter(fileWriter)
			defer func() { _ = gzWriter.Close() }()
			writer = gzWriter
		}
		pterm.Info.Printf("Writing database dump to %s...\n", targetPath)
		options.File = targetPath // Update for the success message below

		if err := runDumpToWriter(dumpCommand, writer, true, cmd.ErrOrStderr()); err != nil {
			return fmt.Errorf("db dump failed: %w", err)
		}

		if options.File != "" {
			pterm.Success.Printf("Database dump saved to %s.\n", filepath.Clean(options.File))
		}
		return nil
	})
}

// createPrivateDumpFile creates (or truncates) a local dump file that only its
// owner can read. A dump holds the whole database, so it is created 0600 rather
// than with the umask-dependent default of os.Create, and a pre-existing file
// that is broader than that is tightened before anything is written to it.
func createPrivateDumpFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("restrict dump file permissions: %w", err)
		}
	}
	return file, nil
}
