package cmd

import (
	"fmt"
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

			credentials := resolveLocalDBCredentials(containerName)
			pterm.Info.Printf("Executing query on %s...\n", containerName)

			queryCmd := buildLocalDBQueryCommand(containerName, credentials, query)
			queryCmd.Stdout = cmd.OutOrStdout()
			queryCmd.Stderr = cmd.ErrOrStderr()
			return queryCmd.Run()
		}

		remoteCfg, err := resolveDBRemote(config, options.Environment, false)
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
