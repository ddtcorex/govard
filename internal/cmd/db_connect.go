package cmd

import (
	"os"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func runDBConnect(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	return runDBHooks(config, engine.HookPreDBConnect, engine.HookPostDBConnect, cmd, func() error {
		if options.Environment == "local" {
			containerName := dbContainerName(config)
			if err := ensureLocalDBRunning(containerName); err != nil {
				return err
			}

			credentials := resolveLocalDBCredentials(config, containerName)
			pterm.Info.Printf("Connecting to database on %s...\n", containerName)
			connectCmd := buildLocalDBConnectCommand(containerName, credentials)
			connectCmd.Stdin, connectCmd.Stdout, connectCmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return connectCmd.Run()
		}

		// An interactive mysql client with the terminal attached can DROP, UPDATE
		// or import just as easily as it can SELECT, so this is a write against
		// the target. forWrite=true is what refuses it against a protected
		// remote; the escape stays `protected: false` in that remote's config,
		// exactly as for db import.
		remoteCfg, err := resolveDBRemote(config, options.Environment, true)
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
