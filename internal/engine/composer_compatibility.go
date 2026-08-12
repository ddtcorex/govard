package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"govard/internal/conventions"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// FixComposerCompatibility ensures the container has the correct Composer version
// and necessary configurations (like plugin allowance and audit bypass) for the project.
func FixComposerCompatibility(config Config) error {
	targetVer := config.Stack.ComposerVersion
	if targetVer == "" {
		profileResult, err := ResolveRuntimeProfile(config.Framework, config.FrameworkVersion)
		if err == nil && profileResult.Profile.ComposerVersion != "" {
			targetVer = profileResult.Profile.ComposerVersion
		}
	}

	if targetVer == "" || targetVer == "latest" {
		// Even if version is latest, we still want to ensure plugin allowance and audit bypass
		// for consistent behavior across all projects.
		return ensureComposerConfig(config)
	}

	pterm.Info.Printf("Ensuring Composer version %s as configured...\n", targetVer)
	if err := ensureSpecificComposerVersion(config, targetVer); err != nil {
		return err
	}

	return ensureComposerConfig(config)
}

func ensureComposerConfig(config Config) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	// Fix: Composer 2.2+ blocks plugins by default. Enable them globally in the container for bootstrap.
	// We also disable audit blocking to prevent failures on older versions with known vulnerabilities.
	// Fix: .composer directory might be owned by root (e.g. if created during image build).
	// We ensure it's owned by the www-data user so composer config -g works.
	fixOwnership := "chown -R www-data:www-data ~/.composer 2>/dev/null || true"
	_ = exec.Command("docker", "exec", "-u", "root", containerName, "sh", "-c", fixOwnership).Run()

	globalScript := "composer config -g allow-plugins true && composer config -g audit.block-insecure false 2>/dev/null || true"
	_ = exec.Command("docker", "exec", containerName, "sh", "-c", globalScript).Run()

	return nil
}

func ensureSpecificComposerVersion(config Config, version string) error {
	pterm.Info.Printf("Ensuring Composer version %s is installed in container...\n", version)

	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)

	// Dynamically resolve minor versions like "2.7" to the latest patch release (e.g. "2.7.9")
	if isMinorComposerVersion(version) && version != "2.2" {
		resolved, err := resolveComposerPatchVersion(version)
		if err == nil && resolved != "" {
			pterm.Info.Printf("Resolved Composer minor version %s to patch release %s\n", version, resolved)
			version = resolved
		} else {
			pterm.Warning.Printf("Could not dynamically resolve Composer version %s: %v\n", version, err)
		}
	}

	downloadUrl := "https://getcomposer.org/composer-stable.phar"
	if version != "latest" {
		if version == "2" {
			downloadUrl = "https://getcomposer.org/composer-2.phar"
		} else if version == "1" {
			downloadUrl = "https://getcomposer.org/composer-1.phar"
		} else if version == "2.2" {
			downloadUrl = "https://getcomposer.org/download/latest-2.2.x/composer.phar"
		} else if strings.Contains(version, ".") {
			downloadUrl = fmt.Sprintf("https://getcomposer.org/download/%s/composer.phar", version)
		}
	}

	script := buildComposerEnsureScript(version, downloadUrl)

	cmd := exec.Command("docker", "exec", "-u", "root", containerName, "sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("composer setup failed (%s): %w: %s", version, err, string(out))
	}

	trimmedOut := strings.TrimSpace(string(out))
	if trimmedOut != "" {
		pterm.Info.Println(trimmedOut)
	}

	pterm.Success.Printf("Composer %s is now active.\n", version)
	return nil
}

// buildComposerEnsureScript builds the in-container shell script that makes
// the given Composer version active. It skips the network download when a
// pre-baked binary already matches, or when the currently active `composer`
// already reports the exact target version — otherwise `env up` would
// re-download composer.phar from getcomposer.org on every single run.
func buildComposerEnsureScript(version, downloadUrl string) string {
	return fmt.Sprintf(`
		case "%[1]s" in
			1)   [ -f /usr/local/bin/composer1 ]   && echo "Using pre-baked Composer version 1..." && ln -sf /usr/local/bin/composer1 $(which composer) && exit 0 ;;
			2)   [ -f /usr/local/bin/composer2 ]   && echo "Using pre-baked Composer version 2..." && ln -sf /usr/local/bin/composer2 $(which composer) && exit 0 ;;
			2.2) [ -f /usr/local/bin/composer2lts ] && echo "Using pre-baked Composer version 2.2..." && ln -sf /usr/local/bin/composer2lts $(which composer) && exit 0 ;;
		esac
		CURRENT_VERSION=$(composer --version 2>/dev/null | sed -nE 's/.*version ([0-9]+\.[0-9]+\.[0-9]+).*/\1/p')
		if [ "$CURRENT_VERSION" = "%[1]s" ]; then
			echo "Composer %[1]s is already active, skipping download."
			exit 0
		fi
		echo "Ensuring Composer version %[1]s (downloading from %[2]s)..."
		curl -sSfL %[2]s -o /tmp/composer.phar && chmod +x /tmp/composer.phar && mv /tmp/composer.phar $(which composer)
	`, version, downloadUrl)
}

// BuildComposerEnsureScriptForTest exposes buildComposerEnsureScript for tests.
func BuildComposerEnsureScriptForTest(version, downloadUrl string) string {
	return buildComposerEnsureScript(version, downloadUrl)
}

func isMinorComposerVersion(version string) bool {
	return len(strings.Split(version, ".")) == 2
}

func resolveComposerPatchVersion(minorVersion string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://repo.packagist.org/p2/composer/composer.json", nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Packages struct {
			Composer []struct {
				Version string `json:"version"`
			} `json:"composer/composer"`
		} `json:"packages"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	for _, c := range result.Packages.Composer {
		if strings.HasPrefix(c.Version, minorVersion+".") && !strings.Contains(strings.ToLower(c.Version), "-") {
			return c.Version, nil
		}
	}
	return "", fmt.Errorf("no stable patch versions found for %s", minorVersion)
}
