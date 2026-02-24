package engine

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/pterm/pterm"
)

func TrustCA() error {
	// In a real scenario, we'd pull the cert from the Caddy container or a shared volume.
	// For this implementation, we assume the CA cert is available or we show how to get it.

	pterm.Info.Println("Attempting to trust Govard Root CA...")

	switch runtime.GOOS {
	case "linux":
		return trustLinux()
	case "darwin":
		return trustDarwin()
	default:
		return fmt.Errorf("unsupported operating system for automated trust: %s", runtime.GOOS)
	}
}

func trustLinux() error {
	pterm.Info.Println("On Linux, this requires sudo privileges to update /usr/local/share/ca-certificates/")

	localCertPath, err := extractCAToUserDir()
	if err != nil {
		return err
	}

	systemCertPath := "/usr/local/share/ca-certificates/govard.crt"

	// Copy to system trust store
	cmd := exec.Command("sudo", "cp", localCertPath, systemCertPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy cert to system store (sudo required): %v", err)
	}

	// Update trust store
	cmd = exec.Command("sudo", "update-ca-certificates")
	return cmd.Run()
}

func trustDarwin() error {
	localCertPath, err := extractCAToUserDir()
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", localCertPath)
	return cmd.Run()
}

func extractCAToUserDir() (string, error) {
	proxyContainer := "proxy-caddy-1"

	homeDir, uid, gid, err := resolveUserHomeAndOwnership()
	if err != nil {
		return "", err
	}

	sslDir := filepath.Join(homeDir, ".govard", "ssl")
	if err := os.MkdirAll(sslDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create ssl directory %s: %w", sslDir, err)
	}

	localCertPath := filepath.Join(sslDir, "root.crt")

	// Extract cert from Caddy container to global govard storage
	pterm.Debug.Printf("Extracting CA from %s to %s...\n", proxyContainer, localCertPath)
	cmd := exec.Command("docker", "cp", proxyContainer+":/data/caddy/pki/authorities/local/root.crt", localCertPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to extract CA from container: %v, output: %s", err, string(output))
	}

	// Ensure readable by user for browser import (especially if created as root)
	if err := os.Chmod(localCertPath, 0644); err != nil {
		return "", fmt.Errorf("failed to set permissions on %s: %w", localCertPath, err)
	}

	// Set ownership if running under sudo
	if uid != -1 && gid != -1 {
		if err := os.Chown(sslDir, uid, gid); err != nil {
			return "", fmt.Errorf("failed to set ownership on %s: %w", sslDir, err)
		}
		if err := os.Chown(localCertPath, uid, gid); err != nil {
			return "", fmt.Errorf("failed to set ownership on %s: %w", localCertPath, err)
		}
	}

	return localCertPath, nil
}

// resolveUserHomeAndOwnership returns homeDir, uid, gid.
// uid/gid are -1 if not running under sudo or if resolution fails (in which case we rely on default permissions).
func resolveUserHomeAndOwnership() (string, int, int, error) {
	homeDir := os.Getenv("HOME")
	uid, gid := -1, -1

	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err := user.Lookup(sudoUser)
		if err == nil {
			homeDir = u.HomeDir

			uID, convErr := strconv.Atoi(u.Uid)
			if convErr == nil {
				uid = uID
			}
			gID, convErr := strconv.Atoi(u.Gid)
			if convErr == nil {
				gid = gID
			}
		}
	}

	if homeDir == "" {
		// Fallback to current user if SUDO_USER not set or lookup failed but we need a home dir
		u, err := user.Current()
		if err != nil {
			return "", -1, -1, fmt.Errorf("failed to determine user home directory: %w", err)
		}
		homeDir = u.HomeDir
	}

	return homeDir, uid, gid, nil
}
