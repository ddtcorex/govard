package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var remoteCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: "ssh,rsync",
	},
	Use:     "remote",
	Aliases: []string{"rmt"},
	Short:   "Manage remote environments",
}

var remoteAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add or update a remote environment",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		startedAt := time.Now()
		operationStatus := engine.OperationStatusFailure
		operationCategory := ""
		operationMessage := ""
		configForObservability := engine.Config{}
		defer func() {
			writeOperationEventBestEffort(
				"remote.add",
				operationStatus,
				configForObservability,
				"",
				"",
				operationMessage,
				operationCategory,
				time.Since(startedAt),
			)
			if operationStatus == engine.OperationStatusSuccess {
				cwd, _ := os.Getwd()
				trackProjectRegistryBestEffort(configForObservability, cwd, "remote-add")
			}
		}()
		config, err := loadWritableConfig()
		if err != nil {

			return err
		}
		configForObservability = config

		name := ""
		if len(args) > 0 {
			name = strings.TrimSpace(args[0])
		} else if stdinIsTerminal() {
			name, _ = pterm.DefaultInteractiveTextInput.Show("Enter remote name (e.g. staging)")
		}
		name = strings.ToLower(strings.TrimSpace(name))

		if name == "" {
			return fmt.Errorf("remote name is required")
		}

		host, _ := cmd.Flags().GetString("host")
		user, _ := cmd.Flags().GetString("user")
		path, _ := cmd.Flags().GetString("path")
		port, _ := cmd.Flags().GetInt("port")
		protectedSet := cmd.Flags().Changed("protected")
		protected, _ := cmd.Flags().GetBool("protected")
		capabilitiesRaw, _ := cmd.Flags().GetString("capabilities")
		authMethodRaw, _ := cmd.Flags().GetString("auth-method")
		keyPathRaw, _ := cmd.Flags().GetString("key-path")
		strictHostKey, _ := cmd.Flags().GetBool("strict-host-key")
		knownHostsFile, _ := cmd.Flags().GetString("known-hosts-file")

		if host == "" && stdinIsTerminal() {
			host, _ = pterm.DefaultInteractiveTextInput.WithDefaultText(host).Show("Remote host (e.g. example.com)")
		}
		if user == "" && stdinIsTerminal() {
			user, _ = pterm.DefaultInteractiveTextInput.WithDefaultText(user).Show("Remote SSH user")
		}
		if port == 22 && !cmd.Flags().Changed("port") && stdinIsTerminal() {
			portStr, _ := pterm.DefaultInteractiveTextInput.WithDefaultText("22").Show("Remote SSH port")
			if portStr != "" {
				if p, err := strconv.Atoi(portStr); err == nil {
					port = p
				}
			}
		}
		if path == "" && stdinIsTerminal() {
			path, _ = pterm.DefaultInteractiveTextInput.WithDefaultText(path).Show("Remote project path on target host (e.g. /var/www/app or ~/public_html)")
		}

		if host == "" || user == "" || path == "" {
			err := fmt.Errorf("host, user, and path are required")
			operationCategory = "validation"
			operationMessage = "missing required flags: host/user/path"
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.add",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   "validation",
				Remote:     name,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Message:    "missing required flags: host/user/path",
			})
			return err
		}

		// environment is now derived from name
		capabilities, err := engine.ParseRemoteCapabilitiesCSV(capabilitiesRaw)
		if err != nil {
			operationCategory = "validation"
			operationMessage = err.Error()
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.add",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   "validation",
				Remote:     name,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Message:    err.Error(),
			})
			return err
		}
		authMethod := remote.NormalizeAuthMethod(authMethodRaw)
		if !remote.IsSupportedAuthMethod(authMethod) {
			err := fmt.Errorf("unsupported auth method '%s' (allowed: keychain, ssh-agent, keyfile)", authMethodRaw)
			operationCategory = "validation"
			operationMessage = err.Error()
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.add",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   "validation",
				Remote:     name,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Message:    err.Error(),
			})
			return err
		}

		keyPath := strings.TrimSpace(keyPathRaw)
		if keyPath != "" && authMethod == remote.AuthMethodKeychain {
			if err := remote.PersistSSHKeyPath(name, keyPath); err != nil {
				pterm.Warning.Printf("Could not persist key path in auth store (%v); falling back to config auth.key_path.\n", err)
			} else {
				pterm.Info.Printf("Stored SSH key path for remote '%s' in auth store.\n", name)
				keyPath = ""
			}
		}

		if knownHostsFile != "" && !strictHostKey {
			strictHostKey = true
			pterm.Warning.Println("known-hosts-file specified: enabling strict host key checking.")
		}

		if config.Remotes == nil {
			config.Remotes = map[string]engine.RemoteConfig{}
		}

		var protectedPtr *bool
		if protectedSet {
			protectedPtr = engine.BoolPtr(protected)
		}

		config.Remotes[name] = engine.RemoteConfig{
			Host:         host,
			User:         user,
			Path:         path,
			Port:         port,
			Protected:    protectedPtr,
			Capabilities: capabilities,
			Auth: engine.RemoteAuth{
				Method:         authMethod,
				KeyPath:        keyPath,
				StrictHostKey:  strictHostKey,
				KnownHostsFile: knownHostsFile,
			},
		}

		saveConfig(config)
		configForObservability = config
		effectiveProtected, _ := engine.RemoteWriteBlocked(name, config.Remotes[name])
		pterm.Success.Printf(
			"Remote '%s' saved (capabilities=%s, auth=%s, protected=%t, strict_host_key=%t).\n",
			name,
			strings.Join(engine.RemoteCapabilityList(config.Remotes[name]), ","),
			authMethod,
			effectiveProtected,
			strictHostKey,
		)
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "remote.add",
			Status:     remote.RemoteAuditStatusSuccess,
			Remote:     name,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Message:    "remote saved",
		})
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "remote saved"
		return nil
	},
}

var remoteCopyIdCmd = &cobra.Command{
	Use:   "copy-id [name]",
	Short: "Copy local SSH public key to a remote environment",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = strings.TrimSpace(args[0])
		} else if stdinIsTerminal() {
			name, _ = pterm.DefaultInteractiveTextInput.Show("Enter remote name (e.g. staging)")
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			return fmt.Errorf("remote name is required")
		}

		keyPathRaw, _ := cmd.Flags().GetString("identity")
		config, err := loadFullConfig()
		if err != nil {
			return err
		}
		_, remoteCfg, err := ensureRemoteKnown(config, name)
		if err != nil {
			return err
		}

		pubKeyPath := resolvePublicKeyForRemote(name, remoteCfg, keyPathRaw)
		if pubKeyPath == "" {
			return fmt.Errorf("could not find a public key to copy. Please specify one with --identity/-i")
		}

		pterm.Info.Printf("Copying public key '%s' to remote '%s' (%s)...\n", pubKeyPath, name, remote.RemoteTarget(remoteCfg))
		return copySSHKeyToRemote(name, remoteCfg, pubKeyPath)
	},
}

var remoteExecCmd = &cobra.Command{
	Use:   "exec [name] -- <command>",
	Short: "Execute a command on a remote environment",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		startedAt := time.Now()
		remoteName := args[0]
		operationStatus := engine.OperationStatusFailure
		operationCategory := ""
		operationMessage := ""
		configForObservability := engine.Config{}
		defer func() {
			writeOperationEventBestEffort(
				"remote.exec",
				operationStatus,
				configForObservability,
				remoteName,
				"",
				operationMessage,
				operationCategory,
				time.Since(startedAt),
			)
			if operationStatus == engine.OperationStatusSuccess {
				cwd, _ := os.Getwd()
				trackProjectRegistryBestEffort(configForObservability, cwd, "remote-exec")
			}
		}()
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		configForObservability = config
		_, remoteCfg, err := ensureRemoteKnown(config, remoteName)
		if err != nil {
			operationCategory = "validation"
			operationMessage = err.Error()
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.exec",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   "validation",
				Remote:     remoteName,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Message:    err.Error(),
			})
			return err
		}

		commandLine := strings.TrimSpace(strings.Join(args[1:], " "))
		if commandLine == "" {
			err := fmt.Errorf("missing remote command after '--'")
			operationCategory = "validation"
			operationMessage = "missing remote command after '--'"
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.exec",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   "validation",
				Remote:     remoteName,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Message:    "missing remote command after '--'",
			})
			return err
		}

		if remoteCfg.Path != "" {
			commandLine = fmt.Sprintf("cd %s && %s", remote.QuoteRemotePath(remoteCfg.Path), commandLine)
		}

		// Pre-flight check: offer to copy SSH key if auth fails
		_ = offerSSHKeyCopyOnAuthFailure(remoteName, remoteCfg)

		sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, commandLine)
		sshCmd.Stdin = os.Stdin
		sshCmd.Stdout = os.Stdout
		sshCmd.Stderr = os.Stderr

		if err := sshCmd.Run(); err != nil {
			details := remote.ClassifyFailure(err, "")
			operationCategory = details.Category
			operationMessage = err.Error()
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.exec",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   details.Category,
				Remote:     remoteName,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Message:    err.Error(),
			})
			return fmt.Errorf("remote exec failed: %w", err)
		}
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "remote.exec",
			Status:     remote.RemoteAuditStatusSuccess,
			Remote:     remoteName,
			DurationMS: time.Since(startedAt).Milliseconds(),
		})
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "remote exec completed"
		return nil
	},
}

var remoteTestCmd = &cobra.Command{
	Use:   "test [name]",
	Short: "Test SSH connectivity to a remote",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteName := args[0]
		startedAt := time.Now()
		operationStatus := engine.OperationStatusFailure
		operationCategory := ""
		operationMessage := ""
		configForObservability := engine.Config{}
		defer func() {
			writeOperationEventBestEffort(
				"remote.test",
				operationStatus,
				configForObservability,
				remoteName,
				"",
				operationMessage,
				operationCategory,
				time.Since(startedAt),
			)
			if operationStatus == engine.OperationStatusSuccess {
				cwd, _ := os.Getwd()
				trackProjectRegistryBestEffort(configForObservability, cwd, "remote-test")
			}
		}()
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		configForObservability = config
		_, remoteCfg, err := ensureRemoteKnown(config, remoteName)
		if err != nil {
			operationCategory = "validation"
			operationMessage = err.Error()
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation: "remote.test",
				Status:    remote.RemoteAuditStatusFailure,
				Category:  "validation",
				Remote:    remoteName,
				Message:   err.Error(),
			})
			return err
		}
		effectiveProtected, _ := engine.RemoteWriteBlocked(remoteName, remoteCfg)
		pterm.Info.Printf(
			"Remote profile: capabilities=%s, auth=%s, protected=%t, strict_host_key=%t\n",
			strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
			remote.NormalizeAuthMethod(remoteCfg.Auth.Method),
			effectiveProtected,
			remoteCfg.Auth.StrictHostKey,
		)

		testArgs := remote.BuildSSHArgs(remoteName, remoteCfg, false, false)
		testArgs = append(testArgs, "-o", "ConnectTimeout=5", remote.RemoteTarget(remoteCfg), "echo govard-remote-ok")
		testCmd := exec.Command("ssh", testArgs...)

		// Pre-flight check: offer to copy SSH key if auth fails
		_ = offerSSHKeyCopyOnAuthFailure(remoteName, remoteCfg)

		sshStartedAt := time.Now()
		output, err := testCmd.CombinedOutput()
		sshDuration := time.Since(sshStartedAt)
		if err != nil {
			details := reportRemoteCommandFailure("SSH connectivity", err, output, sshDuration, false)
			operationCategory = details.Category
			operationMessage = err.Error()
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.test.ssh",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   details.Category,
				Remote:     remoteName,
				DurationMS: sshDuration.Milliseconds(),
				Message:    err.Error(),
			})
			return err
		}
		if !strings.Contains(string(output), "govard-remote-ok") {
			err := fmt.Errorf("unexpected SSH probe response")
			details := reportRemoteCommandFailure(
				"SSH connectivity",
				err,
				output,
				sshDuration,
				false,
			)
			operationCategory = details.Category
			operationMessage = "unexpected SSH probe response"
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.test.ssh",
				Status:     remote.RemoteAuditStatusFailure,
				Category:   details.Category,
				Remote:     remoteName,
				DurationMS: sshDuration.Milliseconds(),
				Message:    "unexpected SSH probe response",
			})
			return err
		}
		pterm.Success.Printf("SSH connectivity check passed (%s).\n", formatDuration(sshDuration))
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "remote.test.ssh",
			Status:     remote.RemoteAuditStatusSuccess,
			Remote:     remoteName,
			DurationMS: sshDuration.Milliseconds(),
		})
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "remote SSH connectivity check passed"

		rsyncArgs := remote.BuildSSHArgs(remoteName, remoteCfg, false, false)
		rsyncArgs = append(rsyncArgs, "-o", "ConnectTimeout=5", remote.RemoteTarget(remoteCfg), "command -v rsync >/dev/null 2>&1 && echo govard-rsync-ok")
		rsyncCmd := exec.Command("ssh", rsyncArgs...)
		rsyncStartedAt := time.Now()
		rsyncOutput, err := rsyncCmd.CombinedOutput()
		rsyncDuration := time.Since(rsyncStartedAt)
		if err != nil {
			details := reportRemoteCommandFailure("Remote rsync availability", err, rsyncOutput, rsyncDuration, true)
			operationCategory = details.Category
			operationMessage = "remote SSH passed; rsync availability check reported warning"
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.test.rsync",
				Status:     remote.RemoteAuditStatusWarning,
				Category:   details.Category,
				Remote:     remoteName,
				DurationMS: rsyncDuration.Milliseconds(),
				Message:    err.Error(),
			})
			return err
		}
		if strings.Contains(string(rsyncOutput), "govard-rsync-ok") {
			pterm.Success.Printf("Remote rsync availability check passed (%s).\n", formatDuration(rsyncDuration))
			writeRemoteAuditEvent(remote.AuditEvent{
				Operation:  "remote.test.rsync",
				Status:     remote.RemoteAuditStatusSuccess,
				Remote:     remoteName,
				DurationMS: rsyncDuration.Milliseconds(),
			})
			operationMessage = "remote connectivity and rsync availability checks passed"
			return nil
		}

		err = fmt.Errorf("unexpected rsync probe response")
		details := reportRemoteCommandFailure(
			"Remote rsync availability",
			err,
			rsyncOutput,
			rsyncDuration,
			true,
		)
		writeRemoteAuditEvent(remote.AuditEvent{
			Operation:  "remote.test.rsync",
			Status:     remote.RemoteAuditStatusWarning,
			Category:   details.Category,
			Remote:     remoteName,
			DurationMS: rsyncDuration.Milliseconds(),
			Message:    "unexpected rsync probe response",
		})
		operationCategory = details.Category
		operationMessage = "remote SSH passed; rsync probe response was unexpected"
		return err
	},
}

func init() {
	remoteAddCmd.Flags().String("host", "", "Remote host")
	remoteAddCmd.Flags().String("user", "", "Remote user")
	remoteAddCmd.Flags().String("path", "", "Remote path on target host (quote '~/...' in shell usage to preserve remote home expansion)")
	remoteAddCmd.Flags().Int("port", 22, "Remote port")
	remoteAddCmd.Flags().String("capabilities", "none", "Remote capabilities to block (comma-separated: files,media,db or none)")
	remoteAddCmd.Flags().String("auth-method", remote.AuthMethodKeychain, "Remote auth method (keychain, ssh-agent, keyfile)")
	remoteAddCmd.Flags().String("key-path", "", "SSH private key path (stored in auth store when --auth-method=keychain)")
	remoteAddCmd.Flags().Bool("strict-host-key", false, "Enable strict SSH host key checking")
	remoteAddCmd.Flags().String("known-hosts-file", "", "Custom SSH known_hosts file (implies --strict-host-key)")
	remoteAddCmd.Flags().Bool("protected", false, "Mark remote as protected")
	remoteCopyIdCmd.Flags().StringP("identity", "i", "", "Path to the SSH public key to copy")

	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteExecCmd)
	remoteCmd.AddCommand(remoteCopyIdCmd)
	remoteCmd.AddCommand(remoteTestCmd)
	remoteCmd.AddCommand(remoteAuditCmd)

	rootCmd.AddCommand(remoteCmd)
}

// RootCommandForTest exposes the root command for tests.
func RootCommandForTest() *cobra.Command {
	return rootCmd
}

func ensureRemoteKnown(config engine.Config, name string) (string, engine.RemoteConfig, error) {
	resolvedName, ok := findRemoteByNameOrEnvironment(config, name)
	if !ok {
		return "", engine.RemoteConfig{}, fmt.Errorf("unknown remote: %s", name)
	}
	remote := config.Remotes[resolvedName]
	resolved, err := resolveRemoteConfigSecrets(resolvedName, remote)
	if err != nil {
		return "", engine.RemoteConfig{}, err
	}
	return resolvedName, resolved, nil
}

func findRemoteByNameOrEnvironment(config engine.Config, requested string) (string, bool) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" || len(config.Remotes) == 0 {
		return "", false
	}

	if _, ok := config.Remotes[requested]; ok {
		return requested, true
	}

	names := make([]string, 0, len(config.Remotes))
	for name := range config.Remotes {
		names = append(names, name)
	}
	engine.SortRemoteNames(names)
	for _, name := range names {
		if strings.EqualFold(engine.NormalizeRemoteEnvironment(name), engine.NormalizeRemoteEnvironment(requested)) {
			return name, true
		}
	}

	return "", false
}

func reportRemoteCommandFailure(step string, err error, output []byte, duration time.Duration, warning bool) remote.FailureDetails {
	details := remote.ClassifyFailure(err, string(output))
	message := fmt.Sprintf("%s failed (%s, %s): %v", step, details.Category, formatDuration(duration), err)
	if warning {
		pterm.Warning.Println(message)
	} else {
		pterm.Error.Println(message)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed != "" {
		if warning {
			pterm.Warning.Println(trimmed)
		} else {
			pterm.Error.Println(trimmed)
		}
	}

	if details.Hint != "" {
		if warning {
			pterm.Warning.Printf("Hint: %s\n", details.Hint)
		} else {
			pterm.Error.Printf("Hint: %s\n", details.Hint)
		}
	}
	return details
}

func formatDuration(duration time.Duration) string {
	return duration.Round(time.Millisecond).String()
}

// ResolveAutoRemote determines the source environment to use.
// If requested is provided, it tries to find it.
// If requested is empty, it prioritizes "staging" then "dev".
func ResolveAutoRemote(config engine.Config, requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		if remoteName, ok := findRemoteByNameOrEnvironment(config, requested); ok {
			return remoteName, nil
		}
		return "", fmt.Errorf("remote '%s' is not configured", requested)
	}

	// Priority: staging then dev
	if remoteName, ok := findRemoteByNameOrEnvironment(config, "staging"); ok {
		return remoteName, nil
	}
	if remoteName, ok := findRemoteByNameOrEnvironment(config, "dev"); ok {
		return remoteName, nil
	}

	return "", fmt.Errorf("no remote environment found (tried staging, dev). Please specify one with -e (e.g., -e staging) or add a remote with 'govard remote add staging'")
}
