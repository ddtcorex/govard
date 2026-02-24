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

	proxyContainer := "proxy-caddy-1"

	// Get the actual user's home directory even if running under sudo
	homeDir := os.Getenv("HOME")
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			homeDir = u.HomeDir
		}
	}

	sslDir := filepath.Join(homeDir, ".govard", "ssl")
	if err := os.MkdirAll(sslDir, 0755); err != nil {
		return fmt.Errorf("failed to create ssl directory %s: %w", sslDir, err)
	}

	localCertPath := filepath.Join(sslDir, "root.crt")
	systemCertPath := "/usr/local/share/ca-certificates/govard.crt"

	// 1. Extract cert from Caddy container to global govard storage
	pterm.Debug.Printf("Extracting CA from %s to %s...\n", proxyContainer, localCertPath)
	cmd := exec.Command("docker", "cp", proxyContainer+":/data/caddy/pki/authorities/local/root.crt", localCertPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract CA from container: %v, output: %s", err, string(output))
	}

	// Ensure readable by user for browser import (especially if created as root)
	if err := os.Chmod(localCertPath, 0644); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", localCertPath, err)
	}
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		// Set ownership back to the original user
		if u, err := user.Lookup(sudoUser); err == nil {
			uid, convErr := strconv.Atoi(u.Uid)
			if convErr != nil {
				return fmt.Errorf("failed to parse uid for %s: %w", sudoUser, convErr)
			}
			gid, convErr := strconv.Atoi(u.Gid)
			if convErr != nil {
				return fmt.Errorf("failed to parse gid for %s: %w", sudoUser, convErr)
			}
			if err := os.Chown(sslDir, uid, gid); err != nil {
				return fmt.Errorf("failed to set ownership on %s: %w", sslDir, err)
			}
			if err := os.Chown(localCertPath, uid, gid); err != nil {
				return fmt.Errorf("failed to set ownership on %s: %w", localCertPath, err)
			}
		}
	}

	// 2. Copy to system trust store
	cmd = exec.Command("sudo", "cp", localCertPath, systemCertPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy cert to system store (sudo required): %v", err)
	}

	// 3. Update trust store
	cmd = exec.Command("sudo", "update-ca-certificates")
	return cmd.Run()
}

func trustDarwin() error {
	certPath := "/tmp/govard-ca.crt"
	cmd := exec.Command("sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certPath)
	return cmd.Run()
}
