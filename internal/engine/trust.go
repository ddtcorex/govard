package engine

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"govard/internal/conventions"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

const (
	govardRootCANickname = "Govard Local CA"
)

type TrustOptions struct {
	ImportBrowsers         bool
	ContinueOnBrowserError bool
}

func TrustCA() error {
	return TrustCAWithOptions(TrustOptions{
		ImportBrowsers:         true,
		ContinueOnBrowserError: true,
	})
}

func TrustCAWithOptions(options TrustOptions) error {
	pterm.Info.Println("Attempting to trust Govard Root CA...")
	certInfo, err := exportRootCA()
	if err != nil {
		return err
	}

	if certInfo.Updated {
		pterm.Info.Printf("Exported Govard Root CA to %s\n", certInfo.Path)
	} else {
		pterm.Debug.Printf("Govard Root CA already up-to-date at %s\n", certInfo.Path)
	}

	switch runtime.GOOS {
	case "linux":
		return trustLinux(certInfo, options)
	case "darwin":
		return trustDarwin(certInfo, options)
	default:
		return fmt.Errorf("unsupported operating system for automated trust: %s", runtime.GOOS)
	}
}

type rootCAExport struct {
	Path        string
	Fingerprint string
	HomeDir     string
	Updated     bool
}

func trustLinux(certInfo rootCAExport, options TrustOptions) error {
	pterm.Info.Println("On Linux, this requires sudo privileges to update /usr/local/share/ca-certificates/")

	systemCertPath := "/usr/local/share/ca-certificates/govard.crt"
	systemFingerprint, err := certificateFingerprint(systemCertPath)
	if err == nil && systemFingerprint == certInfo.Fingerprint {
		pterm.Info.Println("System trust store already contains current Govard Root CA.")
	} else {
		if err := runCommand("sudo", "cp", certInfo.Path, systemCertPath); err != nil {
			return fmt.Errorf("failed to copy cert to system store (sudo required): %w", err)
		}
		if err := runCommand("sudo", "update-ca-certificates"); err != nil {
			return fmt.Errorf("failed to refresh system trust store: %w", err)
		}
		pterm.Success.Println("System trust store updated with Govard Root CA.")
	}

	if options.ImportBrowsers {
		if err := importRootCAToBrowsers(certInfo.Path, certInfo.HomeDir); err != nil {
			if options.ContinueOnBrowserError {
				pterm.Warning.Printf("Could not import Govard Root CA into browser stores automatically: %v\n", err)
				pterm.Info.Println("You can retry after installing certutil (libnss3-tools) with `govard doctor trust`.")
			} else {
				return err
			}
		}
	}

	return nil
}

func trustDarwin(certInfo rootCAExport, options TrustOptions) error {
	output, err := exec.Command(
		"sudo",
		"security",
		"add-trusted-cert",
		"-d",
		"-r",
		"trustRoot",
		"-k",
		"/Library/Keychains/System.keychain",
		certInfo.Path,
	).CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		// Re-trusting can report existing item depending on keychain state.
		if !strings.Contains(strings.ToLower(trimmed), "already exists") {
			return fmt.Errorf("failed to trust certificate in macOS keychain: %v (%s)", err, trimmed)
		}
	}
	pterm.Success.Println("System keychain updated with Govard Root CA.")

	if options.ImportBrowsers {
		if err := importRootCAToBrowsers(certInfo.Path, certInfo.HomeDir); err != nil {
			if options.ContinueOnBrowserError {
				pterm.Warning.Printf("Could not import Govard Root CA into browser stores automatically: %v\n", err)
			} else {
				return err
			}
		}
	}

	return nil
}

func exportRootCA() (rootCAExport, error) {
	homeDir, uid, gid, hasOwnership, err := resolveHostUser()
	if err != nil {
		return rootCAExport{}, err
	}

	sslDir := filepath.Join(homeDir, ".govard", "ssl")
	if err := os.MkdirAll(sslDir, conventions.DefaultDirPerm); err != nil {
		return rootCAExport{}, fmt.Errorf("failed to create ssl directory %s: %w", sslDir, err)
	}
	localCertPath := filepath.Join(sslDir, "root.crt")
	tempCertPath := localCertPath + ".tmp"

	if hasOwnership {
		if err := os.Chown(sslDir, uid, gid); err != nil {
			return rootCAExport{}, fmt.Errorf("failed to set ownership on %s: %w", sslDir, err)
		}
	}

	extractErr := extractRootCAFromContainer(tempCertPath)
	if extractErr != nil {
		warmupCaddyCA()
		if retryErr := extractRootCAFromContainer(tempCertPath); retryErr != nil {
			return rootCAExport{}, fmt.Errorf("failed to extract Govard Root CA: %w", extractErr)
		}
	}

	if err := os.Chmod(tempCertPath, conventions.DefaultFilePerm); err != nil {
		return rootCAExport{}, fmt.Errorf("failed to set permissions on %s: %w", tempCertPath, err)
	}
	if hasOwnership {
		if err := os.Chown(tempCertPath, uid, gid); err != nil {
			return rootCAExport{}, fmt.Errorf("failed to set ownership on %s: %w", tempCertPath, err)
		}
	}

	newFingerprint, err := certificateFingerprint(tempCertPath)
	if err != nil {
		return rootCAExport{}, fmt.Errorf("failed to inspect exported CA certificate: %w", err)
	}

	existingFingerprint, existingErr := certificateFingerprint(localCertPath)
	updated := true

	if existingErr == nil && existingFingerprint == newFingerprint {
		updated = false
		if removeErr := os.Remove(tempCertPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return rootCAExport{}, fmt.Errorf("failed to clean temp CA file: %w", removeErr)
		}
	} else {
		if err := os.Rename(tempCertPath, localCertPath); err != nil {
			return rootCAExport{}, fmt.Errorf("failed to move CA certificate into place: %w", err)
		}
		if hasOwnership {
			if err := os.Chown(localCertPath, uid, gid); err != nil {
				return rootCAExport{}, fmt.Errorf("failed to set ownership on %s: %w", localCertPath, err)
			}
		}
	}

	return rootCAExport{
		Path:        localCertPath,
		Fingerprint: newFingerprint,
		HomeDir:     homeDir,
		Updated:     updated,
	}, nil
}

func extractRootCAFromContainer(destinationPath string) error {
	_ = os.Remove(destinationPath)
	candidates := []string{
		"govard-proxy-caddy",
		"proxy-caddy-1",
	}
	attemptErrors := make([]string, 0, len(candidates))

	for _, containerName := range candidates {
		output, err := exec.Command(
			"docker",
			"cp",
			fmt.Sprintf("%s:/data/caddy/pki/authorities/local/root.crt", containerName),
			destinationPath,
		).CombinedOutput()
		if err == nil {
			return nil
		}
		attemptErrors = append(
			attemptErrors,
			fmt.Sprintf("%s: %s", containerName, strings.TrimSpace(string(output))),
		)
	}

	return fmt.Errorf("tried containers %s", strings.Join(attemptErrors, " | "))
}

// warmupCaddyCA makes a single TLS handshake against the local Caddy proxy to
// force it to mint its root CA / leaf cert on first run, before that CA
// exists anywhere for us to verify against (extractRootCAFromContainer calls
// this only after an initial extraction attempt fails). The dial target is
// hardcoded to loopback, no data is sent or read, and the connection is
// closed immediately after the handshake, so InsecureSkipVerify carries no
// meaningful risk here — there is nothing yet to verify against.
func warmupCaddyCA() {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", "127.0.0.1:443", &tls.Config{
		InsecureSkipVerify: true, // local bootstrap probe, see doc comment above
		ServerName:         "mail.govard.test",
	})
	if err != nil {
		return
	}
	_ = conn.Close()
}

func resolveHostUser() (homeDir string, uid int, gid int, hasOwnership bool, err error) {
	homeDir, err = os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = os.Getenv("HOME")
	}
	if strings.TrimSpace(homeDir) == "" {
		return "", 0, 0, false, fmt.Errorf("could not resolve home directory")
	}

	sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER"))
	if sudoUser == "" {
		return homeDir, 0, 0, false, nil
	}

	u, err := user.Lookup(sudoUser)
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("lookup sudo user %s: %w", sudoUser, err)
	}

	parsedUID, err := strconv.Atoi(u.Uid)
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("parse uid for %s: %w", sudoUser, err)
	}
	parsedGID, err := strconv.Atoi(u.Gid)
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("parse gid for %s: %w", sudoUser, err)
	}

	return u.HomeDir, parsedUID, parsedGID, true, nil
}

func importRootCAToBrowsers(certPath string, homeDir string) error {
	if _, err := exec.LookPath("certutil"); err != nil {
		return fmt.Errorf("certutil not found in PATH")
	}

	dbs := discoverNSSDatabases(homeDir)
	if len(dbs) == 0 {
		pterm.Info.Println("No browser NSS databases found; skipping browser certificate import.")
		return nil
	}

	imported := 0
	errorsFound := make([]string, 0)

	for _, db := range dbs {
		if err := importRootCAToNSSDB(certPath, db.Path, db.AllowCreate); err != nil {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: %v", db.Path, err))
			continue
		}
		imported++
	}

	if imported > 0 {
		pterm.Success.Printf("Imported Govard Root CA into %d browser profile store(s).\n", imported)
		pterm.Info.Println("Restart browsers to apply updated trust.")
	}
	if len(errorsFound) > 0 {
		return errors.New(strings.Join(errorsFound, " | "))
	}
	return nil
}

type nssDatabase struct {
	Path        string
	AllowCreate bool
}

func discoverNSSDatabases(homeDir string) []nssDatabase {
	defaultNSSPath := filepath.Join(homeDir, ".pki", "nssdb")
	candidates := []nssDatabase{
		{Path: defaultNSSPath, AllowCreate: true},
		{Path: filepath.Join(homeDir, "snap", "chromium", "current", ".pki", "nssdb"), AllowCreate: true},
		{Path: filepath.Join(homeDir, "snap", "brave", "current", ".pki", "nssdb"), AllowCreate: true},
		{Path: filepath.Join(homeDir, "snap", "microsoft-edge", "current", ".pki", "nssdb"), AllowCreate: true},
	}

	firefoxRoot := filepath.Join(homeDir, ".mozilla", "firefox")
	entries, err := os.ReadDir(firefoxRoot)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			profilePath := filepath.Join(firefoxRoot, entry.Name())
			candidates = append(candidates, nssDatabase{
				Path:        profilePath,
				AllowCreate: false,
			})
		}
	}

	seen := make(map[string]bool)
	filtered := make([]nssDatabase, 0, len(candidates))
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate.Path)
		if seen[cleaned] {
			continue
		}
		if candidate.AllowCreate && cleaned != defaultNSSPath {
			if _, err := os.Stat(cleaned); err != nil {
				continue
			}
		}
		if !candidate.AllowCreate {
			if !looksLikeNSSDatabase(cleaned) {
				continue
			}
		}
		seen[cleaned] = true
		filtered = append(filtered, nssDatabase{
			Path:        cleaned,
			AllowCreate: candidate.AllowCreate,
		})
	}

	return filtered
}

func looksLikeNSSDatabase(path string) bool {
	if _, err := os.Stat(filepath.Join(path, "cert9.db")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "cert8.db")); err == nil {
		return true
	}
	return false
}

func importRootCAToNSSDB(certPath string, dbPath string, allowCreate bool) error {
	if allowCreate {
		if err := ensureNSSDatabase(dbPath); err != nil {
			return err
		}
	} else if !looksLikeNSSDatabase(dbPath) {
		return fmt.Errorf("not an NSS database")
	}

	// Delete previous nickname first to keep trust attributes and CA in sync.
	_, _ = exec.Command(
		"certutil",
		"-D",
		"-d",
		"sql:"+dbPath,
		"-n",
		govardRootCANickname,
	).CombinedOutput()

	if err := runCommand(
		"certutil",
		"-A",
		"-d",
		"sql:"+dbPath,
		"-n",
		govardRootCANickname,
		"-t",
		"C,,",
		"-i",
		certPath,
	); err != nil {
		return err
	}

	return nil
}

func ensureNSSDatabase(path string) error {
	if looksLikeNSSDatabase(path) {
		return nil
	}
	if err := os.MkdirAll(path, conventions.SecretDirPerm); err != nil {
		return fmt.Errorf("create NSS db directory: %w", err)
	}

	output, err := exec.Command(
		"certutil",
		"-N",
		"-d",
		"sql:"+path,
		"--empty-password",
	).CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if !strings.Contains(strings.ToLower(trimmed), "already exists") {
			return fmt.Errorf("initialize NSS db: %v (%s)", err, trimmed)
		}
	}
	return nil
}

func runCommand(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func certificateFingerprint(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	der := data
	if block, _ := pem.Decode(data); block != nil {
		der = block.Bytes
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}
