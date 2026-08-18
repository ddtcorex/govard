package wordpress

import (
	"fmt"
	"os/exec"
	"regexp"
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

// recommendedWPCliVersion returns the WP-CLI version govard pins for a
// WordPress major version, or "" when no specific version is recommended
// (an unknown major — or one newer than the map — falls back to the latest
// WP-CLI, just like resolveWPCliURL).
func recommendedWPCliVersion(wpMajor int) string {
	return wpCLIVersionMap[wpMajor]
}

// wpCliVersionPattern matches the numeric X.Y.Z release in WP-CLI's
// `wp --version` output (e.g. "WP-CLI 2.8.1").
var wpCliVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// parseWPCliVersion extracts the active WP-CLI version (X.Y.Z) from
// `wp --version` output, or "" when none is present.
func parseWPCliVersion(raw string) string {
	return wpCliVersionPattern.FindString(raw)
}

// wpVersionMatches reports whether the active WP-CLI version (from
// `wp --version`) equals the expected one.
func wpVersionMatches(raw, want string) bool {
	if want == "" {
		return false
	}
	return parseWPCliVersion(raw) == want
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
	if config.Framework != frameworkName {
		return nil
	}

	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)

	// Detect the WordPress version and the WP-CLI version pinned for it.
	wpMajor := detectWordPressVersion(containerName)
	pinned := recommendedWPCliVersion(wpMajor)

	// Fast path — mirrors the Composer fast path (see ensureSpecificComposerVersion
	// in internal/engine/composer_compatibility.go): skip the download when the
	// container already runs the recommended WP-CLI version, instead of
	// overwriting the phar on every env up. This is also what lets the pinned
	// version actually be enforced: the previous existence-only check would keep
	// an outdated WP-CLI once the recommendation for a WordPress major changed.
	// When nothing is pinned (WordPress major undetectable, or no recommendation
	// for it), keep the original behavior and only install when wp is absent.
	if pinned != "" {
		if active := getWPVersion(containerName); wpVersionMatches(active, pinned) {
			pterm.Info.Printf("WP-CLI %s is already active, skipping download.\n", pinned)
			return nil
		}
	} else if wpExists(containerName) {
		return nil
	}

	pterm.Info.Println("Installing WP-CLI in WordPress container...")
	wpCLIURL := resolveWPCliURL(wpMajor)
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

// RecommendedWPCliVersionForTest exposes recommendedWPCliVersion to the external /tests package.
func RecommendedWPCliVersionForTest(wpMajor int) string {
	return recommendedWPCliVersion(wpMajor)
}

// ParseWPCliVersionForTest exposes parseWPCliVersion to the external /tests package.
func ParseWPCliVersionForTest(raw string) string {
	return parseWPCliVersion(raw)
}

// WPCliVersionMatchesForTest exposes wpVersionMatches to the external /tests package.
func WPCliVersionMatchesForTest(raw, want string) bool {
	return wpVersionMatches(raw, want)
}
