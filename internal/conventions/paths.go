package conventions

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// Filenames
	BaseConfigFile             = ".govard.yml"
	LocalConfigFile            = ".govard.local.yml"
	LockFileName               = "govard.lock"
	ProjectComposeOverridePath = ".govard/docker-compose.override.yml"

	// Directories
	ProjectExtensionsDir   = ".govard"
	ProjectCommandsDir     = ".govard/commands"
	ProjectHooksDir        = ".govard/hooks"
	ProjectNginxCustomDir  = ".govard/nginx/custom"
	ProjectApacheCustomDir = ".govard/apache/custom"

	// Complex Paths
	ProjectLocalConfigPath = ".govard/.govard.local.yml"

	// Environment Variables
	// Moved to env.go: EnvGovardLock, EnvGovardHome
)

const (
	PrestaShopParametersFile = "app/config/parameters.php"
)

// GetGovardHome returns the absolute path to the Govard home directory (~/.govard by default).
// It respects the GOVARD_HOME_DIR environment variable if set.
func GetGovardHome() string {
	if override := os.Getenv(EnvGovardHome); override != "" {
		return filepath.Clean(override)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".govard")
}

// ShellQuote wraps a string in single quotes and escapes existing single quotes
// so it is safe to use in a shell command.
func ShellQuote(raw string) string {
	if raw == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(raw, "'", `'"'"'`) + "'"
}
