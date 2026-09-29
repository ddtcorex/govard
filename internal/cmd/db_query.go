package cmd

import (
	"fmt"
	"os"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runDBQuery(cmd *cobra.Command, config engine.Config, options dbCommandOptions, extraArgs []string) error {
	return runDBHooks(config, engine.HookPreDBConnect, engine.HookPostDBConnect, cmd, func() error {
		// Get the query from positional args passed to the command
		query := strings.Join(extraArgs, " ")

		// If still empty, check if there are any non-flag arguments
		if query == "" {
			return fmt.Errorf("query argument is required. Usage: govard db query \"<SQL_QUERY>\"")
		}

		if options.Environment == "local" {
			containerName := dbContainerName(config)
			if err := ensureLocalDBRunning(containerName); err != nil {
				return err
			}

			credentials := resolveLocalDBCredentials(config, containerName)
			// stderr, not stdout: a notice on stdout would be ingested as data by
			// `govard db query ... | while read` loops.
			fmt.Fprintf(os.Stderr, "Executing query on %s...\n", containerName)

			queryCmd := buildLocalDBQueryCommand(containerName, credentials, query)
			queryCmd.Stdout = cmd.OutOrStdout()
			queryCmd.Stderr = cmd.ErrOrStderr()
			return queryCmd.Run()
		}

		// The operator's SQL is interpolated verbatim into `mysql -e '<SQL>'`
		// ON THE REMOTE, so this is a write against the target whatever the
		// statement happens to be. forWrite=true is what refuses it against a
		// protected remote; the escape stays `protected: false` in that
		// remote's config, exactly as for db import.
		remoteCfg, err := resolveDBRemote(config, options.Environment, true)
		if err != nil {
			return err
		}
		credentials, probeErr := resolveRemoteDBCredentials(config, options.Environment, remoteCfg)
		if probeErr != nil {
			pterm.Warning.Println(formatRemoteDBProbeWarning(options.Environment, probeErr))
		}

		queryCmd := remote.BuildSSHExecCommand(options.Environment, remoteCfg, false, buildRemoteMySQLQueryCommandString(credentials, query))
		queryCmd.Stdout = cmd.OutOrStdout()
		queryCmd.Stderr = cmd.ErrOrStderr()
		return queryCmd.Run()
	})
}
