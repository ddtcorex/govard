package wordpress

import (
	"fmt"
	"os/exec"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"

	"github.com/pterm/pterm"
)

// wpCLIVersionMap maps WordPress major versions to recommended WP-CLI versions.
// WP-CLI 2.x is required for WordPress 5+, 1.x for older versions.
var wpCLIVersionMap = map[int]string{
	4: "2.4.0",
	5: "2.8.1",
	6: "2.10.0",
}

const (
	// wpCLIBaseURL is the official WP-CLI phar download URL
	wpCLIBaseURL = "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar"
	// wpCLIPharName is the main phar file name (not latest)
	wpCLIPharName = "wp-cli.phar"
	// wpCLISystemPath is the full system path where wp binary will be installed
	wpCLISystemPath = "/usr/local/bin/wp"
)

// FixWordPressCompatibility ensures the PHP container has WP-CLI (wp) installed.
// It downloads the WP-CLI phar directly from the official builds repository,
// selecting the version based on the detected WordPress version.
func FixWordPressCompatibility(config engine.Config) error {
	if config.Framework != conventions.FrameworkWordPress {
		return nil
	}

	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)

	// Check if wp is already available in PATH
	if wpExists(containerName) {
		return nil
	}

	pterm.Info.Println("Installing WP-CLI in WordPress container...")

	// Detect WordPress version to select appropriate WP-CLI
	wpVersion := detectWordPressVersion(containerName)
	wpCLIURL := resolveWPCliURL(wpVersion)
	pterm.Info.Printf("Downloading WP-CLI from %s\n", wpCLIURL)

	// Download and install WP-CLI phar
	// We create a wrapper script that automatically adds --allow-root
	// The phar is stored in a persistent location, not /tmp
	script := fmt.Sprintf(`
		set -e

		# Download WP-CLI phar to persistent location
		curl -sSfL %s -o /usr/local/bin/wp-cli.phar

		# Check file size to ensure download succeeded
		if [ ! -s /usr/local/bin/wp-cli.phar ]; then
			echo "ERROR: Downloaded file is empty"
			exit 1
		fi

		# Create wrapper script that includes --allow-root
		cat > %s << 'WRAPPER_EOF'
#!/bin/sh
# WP-CLI wrapper - automatically adds --allow-root for root execution
exec php /usr/local/bin/wp-cli.phar --allow-root "$@"
WRAPPER_EOF

		# Make both executable
		chmod +x /usr/local/bin/wp-cli.phar
		chmod +x %s

		# Verify installation
		%s --version
	`, wpCLIURL, wpCLISystemPath, wpCLISystemPath, wpCLISystemPath)

	installArgs := []string{"exec", "-u", "root", containerName, "sh", "-c", script}
	out, err := exec.Command("docker", installArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("WP-CLI installation failed: %w: %s", err, string(out))
	}

	// Log success
	if version := getWPVersion(containerName); version != "" {
		pterm.Success.Printf("WP-CLI %s is now available.\n", version)
	} else {
		pterm.Success.Println("WP-CLI installed successfully.")
	}

	return nil
}

// wpExists checks if wp CLI is available in the container.
func wpExists(containerName string) bool {
	script := `command -v wp >/dev/null 2>&1 && wp --version 2>/dev/null | head -1 || echo "not_found"`
	args := []string{"exec", "-u", "root", containerName, "sh", "-c", script}
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err == nil {
		version := strings.TrimSpace(string(out))
		if version != "" && version != "not_found" {
			pterm.Debug.Printf("WP-CLI already available: %s\n", version)
			return true
		}
	}
	return false
}

// getWPVersion returns the installed WP-CLI version.
func getWPVersion(containerName string) string {
	script := `wp --version 2>/dev/null | head -1 || echo ""`
	args := []string{"exec", "-u", "root", containerName, "sh", "-c", script}
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// detectWordPressVersion detects the installed WordPress version.
func detectWordPressVersion(containerName string) int {
	// Try common locations for wp-includes/version.php
	paths := []string{
		fmt.Sprintf("%s/wp-includes/version.php", conventions.DefaultWorkDir),
		fmt.Sprintf("%s/wordpress/wp-includes/version.php", conventions.DefaultWorkDir),
		fmt.Sprintf("%s/web/wp-includes/version.php", conventions.DefaultWorkDir),
	}

	for _, path := range paths {
		script := fmt.Sprintf(`php -r 'include "%s"; echo $wp_version;' 2>/dev/null || echo ""`, path)
		args := []string{"exec", "-u", "root", containerName, "sh", "-c", script}
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err == nil {
			version := strings.TrimSpace(string(out))
			// Parse major version
			if len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
				dotIdx := strings.Index(version, ".")
				if dotIdx > 0 {
					major := version[:dotIdx]
					for _, c := range major {
						if c < '0' || c > '9' {
							return 0
						}
					}
					if majorNum := stringToInt(major); majorNum > 0 {
						pterm.Debug.Printf("Detected WordPress major version: %d\n", majorNum)
						return majorNum
					}
				}
			}
		}
	}

	pterm.Debug.Println("Could not detect WordPress version, using latest WP-CLI")
	return 0
}

// resolveWPCliURL returns the appropriate WP-CLI phar URL based on WordPress version.
func resolveWPCliURL(wpMajorVersion int) string {
	// Check version map
	if version, ok := wpCLIVersionMap[wpMajorVersion]; ok {
		url := fmt.Sprintf("%s/wp-cli-%s.phar", wpCLIBaseURL, version)
		pterm.Debug.Printf("Using WP-CLI %s for WordPress %d.x\n", version, wpMajorVersion)
		return url
	}

	// Use the main wp-cli.phar for unknown versions (always available)
	pterm.Debug.Println("Using latest WP-CLI (version not detected or >= 7)")
	return fmt.Sprintf("%s/%s", wpCLIBaseURL, wpCLIPharName)
}

// stringToInt converts a string to int safely.
func stringToInt(s string) int {
	result := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
	}
	return result
}
