package remote

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/engine"
)

func RemoteTarget(remoteCfg engine.RemoteConfig) string {
	return fmt.Sprintf("%s@%s", remoteCfg.User, remoteCfg.Host)
}

func BuildSSHExecCommand(remoteName string, remoteCfg engine.RemoteConfig, forwardAgent bool, remoteCommand string) *exec.Cmd {
	args := BuildSSHArgs(remoteName, remoteCfg, forwardAgent, false)
	args = append(args, RemoteTarget(remoteCfg), remoteCommand)
	return exec.Command("ssh", args...)
}

func BuildSSHInteractiveArgs(remoteName string, remoteCfg engine.RemoteConfig, forwardAgent bool) []string {
	return BuildSSHArgs(remoteName, remoteCfg, forwardAgent, true)
}

func RunRemoteCapture(remoteName string, remoteCfg engine.RemoteConfig, remoteCommand string) (string, error) {
	cmd := BuildSSHExecCommand(remoteName, remoteCfg, true, remoteCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("remote command failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func BuildRsyncCommand(
	remoteName string,
	source string,
	destination string,
	remoteCfg engine.RemoteConfig,
	deleteFiles bool,
	resume bool,
	noCompress bool,
	includePatterns []string,
	excludePatterns []string,
) *exec.Cmd {
	rsyncMode := "-av"
	if !noCompress {
		rsyncMode = "-avz"
	}
	args := []string{rsyncMode, "--timeout=60", "--numeric-ids"}
	if deleteFiles {
		args = append(args, "--delete")
	}
	if resume {
		args = append(args, "--partial", "--append-verify")
	}
	for _, pattern := range includePatterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		args = append(args, "--include", trimmed)
	}
	for _, pattern := range excludePatterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		args = append(args, "--exclude", trimmed)
	}

	// Performance-optimized SSH arguments:
	// - aes128-ctr is usually faster and has lower CPU overhead.
	// - Compression=no avoids double-compression if rsync -z is already used.
	sshOptArgs := []string{"-o", "Cipher=aes128-ctr", "-o", "Compression=no"}
	sshArgs := append([]string{"ssh"}, BuildSSHArgs(remoteName, remoteCfg, false, false)...)
	sshArgs = append(sshArgs, sshOptArgs...)
	args = append(args, "-e", strings.Join(sshArgs, " "))
	args = append(args, source, destination)

	cmd := exec.Command("rsync", args...)
	cmd.Env = append(os.Environ(), "RSYNC_OLD_ARGS=1")
	return cmd
}

func RunRemoteShell(remoteName string, remoteCfg engine.RemoteConfig, remoteCommand string) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh binary not found: %w", err)
	}

	args := BuildSSHInteractiveArgs(remoteName, remoteCfg, true)
	// args[0] in syscall.Exec should be the path to the executable
	args = append([]string{sshPath}, args...)
	args = append(args, RemoteTarget(remoteCfg), remoteCommand)

	// Since we are replacing the current process, any cleanup logic should be handled before this.
	return engine.Handoff(sshPath, args)
}
