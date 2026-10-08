package cmd

import (
	"compress/gzip"
	"fmt"
	"govard/internal/conventions"
	"io"
	"os"
	"os/exec"
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
			pterm.Success.Println(remoteDumpSuccessMessage(options.Environment, remoteFilePath))
			return nil
		}

		targetFile := options.File
		if targetFile == "" {
			suffix := "sql.gz"
			timestamp := time.Now().Format("20060102T150405")
			targetFile = filepath.Join("var", fmt.Sprintf("%s_%s-%s.%s", config.ProjectName, options.Environment, timestamp, suffix))
		}

		targetPath := filepath.Clean(targetFile)
		pterm.Info.Printf("Writing database dump to %s...\n", targetPath)
		if err := writeDumpFile(dumpCommand, targetPath, cmd.ErrOrStderr()); err != nil {
			return err
		}
		pterm.Success.Printf("Database dump saved to %s (on this machine).\n", targetPath)
		return nil
	})
}

// remoteDumpSuccessMessage says where a remote-side dump file lives, so a
// path such as /tmp/x.sql.gz is not mistaken for a file on this machine.
func remoteDumpSuccessMessage(environment string, remotePath string) string {
	return fmt.Sprintf("Database dump written on %s: %s (on the remote, not on this machine). Add --local to save it here instead.", environment, remotePath)
}

// RemoteDumpSuccessMessageForTest exposes remoteDumpSuccessMessage for tests.
func RemoteDumpSuccessMessageForTest(environment string, remotePath string) string {
	return remoteDumpSuccessMessage(environment, remotePath)
}

// writeDumpFile streams dumpCommand into targetPath through a sibling
// ".partial" file that is renamed into place only after the dump succeeded and
// the file (and its gzip trailer) is fully written. A failed dump therefore
// never leaves a stub that looks like a dump, and an earlier file at targetPath
// survives a failed run.
func writeDumpFile(dumpCommand *exec.Cmd, targetPath string, stderr io.Writer) (err error) {
	if err := os.MkdirAll(filepath.Dir(targetPath), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create dump directory: %w", err)
	}
	// A device or pipe target (/dev/null, /dev/stdout) cannot be renamed into
	// place, so it is written directly.
	partialPath := targetPath + ".partial"
	direct := false
	if info, statErr := os.Stat(targetPath); statErr == nil && !info.Mode().IsRegular() {
		partialPath = targetPath
		direct = true
	}
	fileWriter, err := createPrivateDumpFile(partialPath)
	if err != nil {
		return fmt.Errorf("create dump file: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = fileWriter.Close()
		}
		if err != nil && !direct {
			_ = os.Remove(partialPath)
		}
	}()

	var writer io.Writer = fileWriter
	var gzWriter *gzip.Writer
	if strings.HasSuffix(targetPath, ".gz") {
		gzWriter = gzip.NewWriter(fileWriter)
		writer = gzWriter
	}

	if runErr := runDumpToWriter(dumpCommand, writer, true, stderr); runErr != nil {
		return fmt.Errorf("db dump failed: %w", runErr)
	}
	if gzWriter != nil {
		if err := gzWriter.Close(); err != nil {
			return fmt.Errorf("finish dump compression: %w", err)
		}
	}
	closed = true
	if err := fileWriter.Close(); err != nil {
		return fmt.Errorf("close dump file: %w", err)
	}
	if direct {
		return nil
	}
	if err := os.Rename(partialPath, targetPath); err != nil {
		return fmt.Errorf("move dump into place: %w", err)
	}
	return nil
}

// WriteDumpFileForTest exposes writeDumpFile for tests.
func WriteDumpFileForTest(dumpCommand *exec.Cmd, targetPath string) error {
	return writeDumpFile(dumpCommand, targetPath, io.Discard)
}

// createPrivateDumpFile is the dump-file constructor shared with the snapshot
// code in internal/engine.
func createPrivateDumpFile(path string) (*os.File, error) {
	return engine.CreatePrivateDumpFile(path)
}
