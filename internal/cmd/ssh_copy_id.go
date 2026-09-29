package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
)

// resolvePublicKeyForRemote finds the best SSH public key to copy for a remote.
// If explicitKeyPath is provided, it normalizes and uses that. Otherwise, it
// resolves from config, then falls back to default key file probing.
func resolvePublicKeyForRemote(remoteName string, remoteCfg engine.RemoteConfig, explicitKeyPath string) string {
	pubKeyPath := ""

	if explicitKeyPath != "" {
		pubKeyPath = strings.TrimSuffix(explicitKeyPath, ".pub") + ".pub"
	} else {
		privateKeyPath, _ := remote.ResolveSSHKeyPath(remoteName, remoteCfg)
		if privateKeyPath != "" {
			pubKeyPath = strings.TrimSuffix(privateKeyPath, ".pub") + ".pub"
		}
	}

	if pubKeyPath != "" && fileExists(pubKeyPath) {
		return pubKeyPath
	}

	// Default fallback: probe well-known key types
	candidates := []string{"~/.ssh/id_ed25519.pub", "~/.ssh/id_ecdsa.pub", "~/.ssh/id_rsa.pub"}
	for _, c := range candidates {
		resolved := remote.NormalizePath(c)
		if fileExists(resolved) {
			return resolved
		}
	}

	return ""
}

// copySSHKeyToRemote copies the given public key to the remote server's
// authorized_keys. It uses ssh-copy-id when available, falling back to
// a manual append via SSH.
func copySSHKeyToRemote(remoteName string, remoteCfg engine.RemoteConfig, pubKeyPath string) error {
	// Prefer ssh-copy-id if available
	if sshCopyIdBin, err := exec.LookPath("ssh-copy-id"); err == nil {
		args := []string{"-i", pubKeyPath}
		if remoteCfg.Port > 0 {
			args = append(args, "-p", fmt.Sprintf("%d", remoteCfg.Port))
		}
		args = append(args, remote.RemoteTarget(remoteCfg))
		cmd := exec.Command(sshCopyIdBin, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Fallback for systems without ssh-copy-id
	pubKeyContent, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	setupCmd := fmt.Sprintf(
		"mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '%s' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys",
		strings.TrimSpace(string(pubKeyContent)),
	)
	sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, false, setupCmd)
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	return sshCmd.Run()
}

// shouldOfferSSHKeyCopy is the whole decision, kept pure so it can be asserted
// without a terminal, a key on disk or an SSH connection. A write-protected
// remote is out on the same grounds as every other writing helper in the tree:
// govard does not touch one unless a caller asked for a write explicitly, and
// a key copy is a write to the remote's authorized_keys.
func shouldOfferSSHKeyCopy(remoteName string, remoteCfg engine.RemoteConfig, interactive bool, pubKeyPath string) bool {
	if !interactive {
		return false
	}
	if blocked, _ := engine.RemoteWriteBlocked(remoteName, remoteCfg); blocked {
		return false
	}
	return pubKeyPath != ""
}

// sshKeyCopyConfirmPrinter builds the prompt. pterm's own default is No; this
// used to override it to Yes, so a single Enter copied a public key onto a
// remote nobody had asked govard to write to. The default now takes pterm's,
// and the text names the key so a Yes is still informed.
func sshKeyCopyConfirmPrinter() *pterm.InteractiveConfirmPrinter {
	return pterm.DefaultInteractiveConfirm.WithDefaultValue(false)
}

// ShouldOfferSSHKeyCopyForTest exposes shouldOfferSSHKeyCopy for tests.
func ShouldOfferSSHKeyCopyForTest(remoteName string, remoteCfg engine.RemoteConfig, interactive bool, pubKeyPath string) bool {
	return shouldOfferSSHKeyCopy(remoteName, remoteCfg, interactive, pubKeyPath)
}

// SSHKeyCopyConfirmPrinterForTest exposes sshKeyCopyConfirmPrinter for tests.
func SSHKeyCopyConfirmPrinterForTest() *pterm.InteractiveConfirmPrinter {
	return sshKeyCopyConfirmPrinter()
}

// OfferSSHKeyCopyOnAuthFailureForTest exposes offerSSHKeyCopyOnAuthFailure for tests.
func OfferSSHKeyCopyOnAuthFailureForTest(remoteName string, remoteCfg engine.RemoteConfig) error {
	return offerSSHKeyCopyOnAuthFailure(remoteName, remoteCfg)
}

// offerSSHKeyCopyOnAuthFailure probes SSH auth for the given remote and, if
// key-based authentication fails, interactively offers to copy the local
// SSH public key before the caller proceeds to the full SSH connection.
//
// Returns an error only for non-auth failures (network, host key, etc.).
// Auth failures are handled by offering copy-id; if the user declines,
// nil is returned so the caller can fall through to password-based SSH.
//
// The write-protected check runs before remote.ProbeSSHAuth on purpose: this
// helper must not open a session to a remote the project has declared off-limits
// for writes, even a read-only `ssh <host> true` probe. Key setup for such a
// remote is a request the user makes, not a repair govard performs.
func offerSSHKeyCopyOnAuthFailure(remoteName string, remoteCfg engine.RemoteConfig) error {
	if blocked, reason := engine.RemoteWriteBlocked(remoteName, remoteCfg); blocked {
		pterm.Info.Printf(
			"Remote '%s' is write-protected (%s): govard will not probe it or copy a key into its authorized_keys. Set key authentication up explicitly with `govard remote copy-id %s`.\n",
			remoteName,
			reason,
			remoteName,
		)
		return nil
	}

	probeErr := remote.ProbeSSHAuth(remoteName, remoteCfg)
	if probeErr == nil {
		return nil // Auth OK, nothing to do
	}

	if !remote.IsAuthFailure(probeErr) {
		// Network, host key, or other non-auth error — bubble up
		return fmt.Errorf("SSH connection failed: %w", probeErr)
	}

	// Auth-specific failure — offer to copy key if we're in a terminal
	if !stdinIsTerminal() {
		return nil // Non-interactive, let SSH handle it
	}

	pubKeyPath := resolvePublicKeyForRemote(remoteName, remoteCfg, "")
	if pubKeyPath == "" {
		pterm.Warning.Println("SSH key authentication failed and no local public key found.")
		pterm.Warning.Println("SSH will ask for your password. To set up key auth later, run: govard remote copy-id " + remoteName)
		return nil
	}

	// Belt and braces: the remote and the key are re-checked through the single
	// predicate that decides, so reordering or dropping either condition above
	// cannot quietly re-open the offer. `interactive` is the tty gate above,
	// already answered, so it is passed as the `true` it is.
	if !shouldOfferSSHKeyCopy(remoteName, remoteCfg, true, pubKeyPath) {
		return nil
	}

	confirmed, _ := sshKeyCopyConfirmPrinter().
		Show(fmt.Sprintf(
			"SSH key auth failed for '%s'. Copy your public key (%s) to the remote server?",
			remoteName,
			filepath.Base(pubKeyPath),
		))

	if !confirmed {
		return nil // User declined, let SSH ask for password
	}

	pterm.Info.Printf("Copying public key '%s' to remote '%s' (%s)...\n", pubKeyPath, remoteName, remote.RemoteTarget(remoteCfg))

	if err := copySSHKeyToRemote(remoteName, remoteCfg, pubKeyPath); err != nil {
		pterm.Warning.Printf("Failed to copy SSH key: %v\n", err)
		pterm.Warning.Println("Continuing with password authentication...")
		return nil
	}

	pterm.Success.Printf("SSH key copied to '%s'. Future connections will use key authentication.\n", remoteName)
	return nil
}
