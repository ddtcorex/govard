package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote environments",
}

var remoteAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add or update a remote environment",
	Args:  cobra.ExactArgs(1),
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
		name := args[0]

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

		if host == "" || user == "" || path == "" {
			err := fmt.Errorf("host, user, and path are required")
			pterm.Error.Println(err.Error())
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
			pterm.Error.Println(err.Error())
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
			pterm.Error.Println(err.Error())
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
			"Remote '%s' saved (environment=%s, capabilities=%s, auth=%s, protected=%t, strict_host_key=%t).\n",
			name,
			engine.NormalizeRemoteEnvironment(name),
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
		remoteCfg, err := ensureRemoteKnown(config, remoteName)
		if err != nil {
			pterm.Error.Println(err.Error())
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
			pterm.Error.Println(err.Error())
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
			commandLine = fmt.Sprintf("cd %s && %s", remoteCfg.Path, commandLine)
		}

		sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, commandLine)
		sshCmd.Stdin = os.Stdin
		sshCmd.Stdout = os.Stdout
		sshCmd.Stderr = os.Stderr

		if err := sshCmd.Run(); err != nil {
			pterm.Error.Printf("Remote exec failed: %v\n", err)
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
			return err
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
		remoteCfg, err := ensureRemoteKnown(config, remoteName)
		if err != nil {
			pterm.Error.Println(err.Error())
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
			"Remote profile: environment=%s, capabilities=%s, auth=%s, protected=%t, strict_host_key=%t\n",
			engine.NormalizeRemoteEnvironment(remoteName),
			strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
			remote.NormalizeAuthMethod(remoteCfg.Auth.Method),
			effectiveProtected,
			remoteCfg.Auth.StrictHostKey,
		)

		testArgs := remote.BuildSSHArgs(remoteName, remoteCfg, false, false)
		testArgs = append(testArgs, "-o", "ConnectTimeout=5", remote.RemoteTarget(remoteCfg), "echo govard-remote-ok")
		testCmd := exec.Command("ssh", testArgs...)
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
	remoteAddCmd.Flags().String("path", "", "Remote path")
	remoteAddCmd.Flags().Int("port", 22, "Remote port")
	remoteAddCmd.Flags().String("capabilities", "files,media,db,deploy", "Remote capabilities (files,media,db,deploy or all)")
	remoteAddCmd.Flags().String("auth-method", remote.AuthMethodKeychain, "Remote auth method (keychain, ssh-agent, keyfile)")
	remoteAddCmd.Flags().String("key-path", "", "SSH private key path (stored in auth store when --auth-method=keychain)")
	remoteAddCmd.Flags().Bool("strict-host-key", false, "Enable strict SSH host key checking")
	remoteAddCmd.Flags().String("known-hosts-file", "", "Custom SSH known_hosts file (implies --strict-host-key)")
	remoteAddCmd.Flags().Bool("protected", false, "Mark remote as protected")

	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteExecCmd)
	remoteCmd.AddCommand(remoteTestCmd)
	remoteCmd.AddCommand(remoteAuditCmd)

	rootCmd.AddCommand(remoteCmd)
}

// RemoteCommand exposes the remote command for testing.
func RemoteCommand() *cobra.Command {
	return remoteCmd
}

// RootCommandForTest exposes the root command for tests.
func RootCommandForTest() *cobra.Command {
	return rootCmd
}

func ensureRemoteKnown(config engine.Config, name string) (engine.RemoteConfig, error) {
	remote, ok := config.Remotes[name]
	if !ok {
		return engine.RemoteConfig{}, fmt.Errorf("unknown remote: %s", name)
	}
	resolved, err := resolveRemoteConfigSecrets(name, remote)
	if err != nil {
		return engine.RemoteConfig{}, err
	}
	return resolved, nil
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
