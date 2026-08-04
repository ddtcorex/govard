package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/frameworks"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the application",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := loadFullConfig()
		if err != nil {
			pterm.Error.Println(err)
			return
		}
		if err := engine.RunHooks(config, engine.HookPreDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			pterm.Error.Printf("Pre-deploy hooks failed: %v\n", err)
			return
		}

		locales, _ := cmd.Flags().GetString("locales")
		if strings.TrimSpace(locales) == "" {
			detected := detectFrameworkLocales(config)
			if len(detected) > 0 {
				locales = strings.Join(detected, " ")
				pterm.Info.Printf("Auto-detected locales: %s\n", locales)
			}
		}

		if strings.TrimSpace(locales) != "" {
			pterm.Info.Printf("Deploying static content for locales: %s\n", locales)
		} else {
			pterm.Info.Println("Deploying (strategy: native)")
		}

		if err := engine.RunHooks(config, engine.HookPostDeploy, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			pterm.Error.Printf("Post-deploy hooks failed: %v\n", err)
			return
		}
	},
}

func init() {
	deployCmd.Flags().String("strategy", "native", "Deployment strategy (native or deployer)")
	deployCmd.Flags().Bool("deployer", false, "Use Deployer strategy")
	deployCmd.Flags().String("deployer-config", "", "Path to Deployer config")
	deployCmd.Flags().StringP("locales", "l", "", "Space-separated locales to deploy (e.g. \"en_US fr_FR\"). Auto-detected from DB when not set.")

	rootCmd.AddCommand(deployCmd)
}

// detectFrameworkLocales queries framework-owned locale metadata.
// It returns a deduplicated, sorted list that always includes "en_US".
// Falls back silently on any error.
func detectFrameworkLocales(config engine.Config) []string {
	definition, ok := frameworks.Get(config.Framework)
	if !ok || definition.BuildDeployLocalesQuery == nil {
		return nil
	}
	containerName := dbContainerName(config)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	credentials := resolveLocalDBCredentials(config, containerName)
	credentials = credentials.withDefaults()
	query := definition.BuildDeployLocalesQuery(credentials.TablePrefix)

	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args,
		containerName,
		"sh", "-lc",
		fmt.Sprintf(
			`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else exit 1; fi && "$DB_CLI" -u %s -N -e %s %s`,
			engine.ShellQuote(credentials.Username),
			engine.ShellQuote(query),
			engine.ShellQuote(credentials.Database),
		),
	)

	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil
	}

	localeSet := map[string]struct{}{"en_US": {}}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		locale := strings.TrimSpace(line)
		if locale != "" {
			localeSet[locale] = struct{}{}
		}
	}

	locales := make([]string, 0, len(localeSet))
	for l := range localeSet {
		locales = append(locales, l)
	}
	sort.Strings(locales)
	return locales
}

// DeployCommand exposes the deploy command for tests.
func DeployCommand() *cobra.Command {
	return deployCmd
}
