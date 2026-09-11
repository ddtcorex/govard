package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"govard/internal/runtime"
)

const openSupportedTargets = "admin, db, mail, mftf, portainer, shell, sftp, elasticsearch, opensearch"

var openEnvironment string
var openPma bool
var openClient bool

var openCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "open [target]",
	Short: "Open common service URLs",
	Long: strings.TrimSpace(`
Quickly open web interfaces for various services in your default browser.
Supported targets: admin, db (PMA/DB Client), mail (Mailpit), mftf, portainer, shell, sftp, elasticsearch, opensearch.

Targets:
- admin: The web application's admin panel.
- db: PHPMyAdmin (local) or local DB client (remote).
- mail: Mailpit web UI for inspecting outgoing emails.
- mftf: Magento Functional Testing Framework web endpoint.
- shell: Open or print shell access details for the target environment.
- sftp: SFTP connection details (remote).
- elasticsearch/opensearch: Search engine endpoint info.

Case Studies:
- Fast Login: Use 'govard open admin' to jump straight to the login page.
- QA Access: Use 'govard open mftf' to reach MFTF services when enabled.
- Remote Debugging: Open SFTP details for a remote environment to check logs or config files.
- Database Inspection: Quickly open PHPMyAdmin locally or get connection strings for remote DBs.
`),
	Example: `  # Open the local admin panel
  govard open admin

  # Open Mailpit to check emails
  govard open mail

  # Open the staging admin panel
  govard open admin --environment staging

  # Open local database in Desktop Client
  govard open db --client
  
  # Open local database in PHPMyAdmin explicitly
  govard open db --pma`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		target := strings.ToLower(strings.TrimSpace(args[0]))

		switch target {
		case "admin":
			return runOpenAdminTarget(config, openEnvironment)
		case "db":
			return runOpenDBTarget(config, openEnvironment, openPma, openClient)
		case "mail":
			return runOpenMailTarget(config, openEnvironment)
		case "mftf":
			return runOpenMFTFTarget(config, openEnvironment)
		case "portainer":
			return runOpenPortainerTarget(config, openEnvironment)
		case "shell":
			return runOpenShellTarget(config, openEnvironment)
		case "sftp":
			return runOpenSFTPTarget(config, openEnvironment)
		case "elasticsearch", "opensearch":
			return runOpenSearchTarget(config, target, openEnvironment)
		default:
			return fmt.Errorf("unknown target %q. Supported: %s", target, openSupportedTargets)
		}
	},
}

func init() {
	openCmd.Flags().StringVarP(
		&openEnvironment,
		"environment",
		"e",
		"",
		"Environment for open targets (local or configured remote name/env)",
	)
	openCmd.Flags().BoolVar(&openPma, "pma", false, "Open PHPMyAdmin (for db target)")
	openCmd.Flags().BoolVar(&openClient, "client", false, "Open local DB client like Beekeeper Studio (for db target)")
}

// ResetOpenFlagsForTest resets open command flag-backed globals for tests.
func ResetOpenFlagsForTest() {
	openEnvironment = ""
	openPma = false
	openClient = false
	openCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	})
}
