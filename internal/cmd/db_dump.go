package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

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
