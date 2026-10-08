package cmd

import (
	"fmt"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// dbInfoHost returns the host to print: a remote or container that reaches its
// database over localhost reports an empty host, which must not render as a
// blank value.
func dbInfoHost(host, fallback string) string {
	if strings.TrimSpace(host) == "" {
		return fallback
	}
	return host
}

func runDBInfo(cmd *cobra.Command, config engine.Config, options dbCommandOptions) error {
	return runDBHooks(config, engine.HookPreDBConnect, engine.HookPostDBConnect, cmd, func() error {
		fmt.Fprintln(cmd.OutOrStdout(), "Database Connection Info")
		fmt.Fprintln(cmd.OutOrStdout(), "========================")

		if options.Environment == "local" {
			containerName := dbContainerName(config)
			if err := ensureLocalDBRunning(containerName); err != nil {
				return err
			}

			credentials := resolveLocalDBCredentials(config, containerName)

			fmt.Fprintf(cmd.OutOrStdout(), "Environment:  local\n")
			fmt.Fprintf(cmd.OutOrStdout(), "Container:    %s\n", containerName)
			fmt.Fprintf(cmd.OutOrStdout(), "Host:         %s\n", dbInfoHost(credentials.Host, "localhost (inside container)"))
			fmt.Fprintf(cmd.OutOrStdout(), "Port:         %d\n", credentials.Port)
			if credentials.Port == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Port:         %d (default)\n", conventions.MySQLPort)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Username:     %s\n", credentials.Username)
			fmt.Fprintf(cmd.OutOrStdout(), "Database:     %s\n", credentials.Database)
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "To connect:")
			fmt.Fprintf(cmd.OutOrStdout(), "  govard db connect\n")
		} else {
			remoteCfg, err := resolveDBRemote(config, options.Environment, false)
			if err != nil {
				return err
			}
			credentials, probeErr := resolveRemoteDBCredentials(config, options.Environment, remoteCfg)
			if probeErr != nil {
				pterm.Warning.Println(formatRemoteDBProbeWarning(options.Environment, probeErr))
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Environment:  %s\n", options.Environment)
			fmt.Fprintf(cmd.OutOrStdout(), "Host:         %s\n", dbInfoHost(credentials.Host, "localhost (or internal container hostname)"))
			fmt.Fprintf(cmd.OutOrStdout(), "Port:         %d\n", credentials.Port)
			if credentials.Port == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Port:         %d (default)\n", conventions.MySQLPort)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Username:     %s\n", credentials.Username)
			fmt.Fprintf(cmd.OutOrStdout(), "Database:     %s\n", credentials.Database)
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "To connect:")
			fmt.Fprintf(cmd.OutOrStdout(), "  govard db connect --environment %s\n", options.Environment)
		}
		return nil
	})
}
