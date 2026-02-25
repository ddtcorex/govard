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

	localCertPath, err := extractRootCA("proxy-caddy-1")
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
	pterm.Info.Println("On macOS, this requires sudo privileges to update the System Keychain.")

	certPath, err := extractRootCA("proxy-caddy-1")
	if err != nil {
		return err
	}

	cmd := exec.Command("sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certPath)
	return cmd.Run()
}

func extractRootCA(proxyContainer string) (string, error) {
	homeDir, err := getRealHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	govardDir := filepath.Join(homeDir, ".govard")
	sslDir := filepath.Join(govardDir, "ssl")

	if err := os.MkdirAll(sslDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create ssl directory %s: %w", sslDir, err)
	}

	// Fix ownership of govardDir if created as root
	// This ensures that ~/.govard is not root-owned, preventing future permission issues
	if err := fixOwnership(govardDir); err != nil {
		return "", err
	}

	// Fix ownership of sslDir if created as root
	if err := fixOwnership(sslDir); err != nil {
		return "", err
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

	// Fix ownership of the cert file
	if err := fixOwnership(localCertPath); err != nil {
		return "", err
	}

	return localCertPath, nil
}

func getRealHomeDir() (string, error) {
	// Get the actual user's home directory even if running under sudo
	homeDir := os.Getenv("HOME")
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			homeDir = u.HomeDir
		} else {
			return "", fmt.Errorf("failed to lookup sudo user %s: %w", sudoUser, err)
		}
	}
	return homeDir, nil
}

func fixOwnership(path string) error {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		return nil
	}

	u, err := user.Lookup(sudoUser)
	if err != nil {
		return fmt.Errorf("failed to lookup sudo user %s: %w", sudoUser, err)
	}

	uid, convErr := strconv.Atoi(u.Uid)
	if convErr != nil {
		return fmt.Errorf("failed to parse uid for %s: %w", sudoUser, convErr)
	}
	gid, convErr := strconv.Atoi(u.Gid)
	if convErr != nil {
		return fmt.Errorf("failed to parse gid for %s: %w", sudoUser, convErr)
	}

	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("failed to set ownership on %s: %w", path, err)
	}

	return nil
}
