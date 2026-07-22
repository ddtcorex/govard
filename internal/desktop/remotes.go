package desktop

import (
	"context"
	"errors"
	"fmt"
	"govard/internal/conventions"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"govard/internal/engine"
	engineremote "govard/internal/engine/remote"

	"gopkg.in/yaml.v3"
)

const remoteMagentoAdminProbeScript = `$c=@include "app/etc/env.php"; if(!is_array($c)){fwrite(STDERR,"env.php not found"); exit(2);} echo (string)($c["backend"]["frontName"] ?? "` + conventions.DefaultAdminPath + `");`
const remoteLastSyncReadLimit = 5000

var defaultRunGovardCommandForDesktop = func(root string, args []string) (string, error) {
	return runGovardCommandForDesktopWithTimeout(root, args, 2*time.Minute)
}

var defaultRunGovardCommandForDesktopWithTimeout = func(root string, args []string, timeout time.Duration) (string, error) {
	binary, err := exec.LookPath("govard")
	if err != nil {
		return "", fmt.Errorf("govard CLI not found in PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = filepath.Clean(root)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("command timed out after %v: %s", timeout, trimmed)
		}
		if trimmed != "" {
			return "", fmt.Errorf("%v: %s", err, trimmed)
		}
		return "", err
	}
	return trimmed, nil
}

var runGovardCommandForDesktopWithTimeout = defaultRunGovardCommandForDesktopWithTimeout

var runGovardCommandForDesktop = defaultRunGovardCommandForDesktop

var defaultStartGovardCommandForDesktop = func(root string, args []string) error {
	binary, err := exec.LookPath("govard")
	if err != nil {
		return fmt.Errorf("govard CLI not found in PATH")
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = filepath.Clean(root)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start govard command: %w", err)
	}

	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

var startGovardCommandForDesktop = defaultStartGovardCommandForDesktop

func listProjectRemotes(project string) (RemoteSnapshot, error) {
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return RemoteSnapshot{
			Project:  project,
			Remotes:  []RemoteEntry{},
			Warnings: []string{fmt.Sprintf("Resolution error for %q: %v", project, err)},
		}, nil
	}
	res, err := listProjectRemotesByPath(root)
	if err != nil {
		return RemoteSnapshot{
			Project:  project,
			Remotes:  []RemoteEntry{},
			Warnings: []string{fmt.Sprintf("Config error for %q at %s: %v", project, root, err)},
		}, nil
	}
	return res, nil
}

func listProjectRemotesByPath(root string) (RemoteSnapshot, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(root))
	cfg, _, err := engine.LoadConfigFromDir(cleanRoot, true)
	if err != nil {
		return RemoteSnapshot{}, fmt.Errorf("load config for remotes: %w", err)
	}

	projectName := strings.TrimSpace(cfg.ProjectName)
	if projectName == "" {
		projectName = filepath.Base(cleanRoot)
	}
	lastSyncByRemote := buildRemoteLastSyncLabels(projectName, time.Now().UTC())

	return RemoteSnapshot{
		Project:  projectName,
		Remotes:  buildRemoteEntries(cfg.Remotes, lastSyncByRemote),
		Warnings: []string{},
	}, nil
}

func testRemote(project string, remoteName string) (string, error) {
	startedAt := time.Now()
	status := engine.OperationStatusFailure
	category := "runtime"
	message := ""
	defer func() {
		writeDesktopOperationEvent(
			"desktop.remote.test",
			status,
			project,
			remoteName,
			"",
			message,
			category,
			time.Since(startedAt),
		)
	}()

	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		category = "validation"
		message = "remote name is required"
		return "", fmt.Errorf("%s", message)
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	snapshot, err := listProjectRemotesByPath(root)
	if err != nil {
		message = err.Error()
		return "", err
	}
	if !hasRemoteName(snapshot.Remotes, remoteName) {
		category = "validation"
		message = fmt.Sprintf("unknown remote: %s", remoteName)
		return "", fmt.Errorf("%s", message)
	}

	output, err := runGovardCommandForDesktopWithTimeout(root, []string{"remote", "test", remoteName}, 2*time.Minute)
	if err != nil {
		message = err.Error()
		return "", fmt.Errorf("remote test failed: %w", err)
	}
	if output == "" {
		output = fmt.Sprintf("Remote '%s' test completed.", remoteName)
	}

	status = engine.OperationStatusSuccess
	category = ""
	message = "remote test completed"
	return output, nil
}

func openRemoteURL(project string, remoteName string, ctx context.Context) (string, error) {
	startedAt := time.Now()
	status := engine.OperationStatusFailure
	category := "runtime"
	message := ""
	defer func() {
		writeDesktopOperationEvent(
			"desktop.remote.open_url",
			status,
			project,
			remoteName,
			"",
			message,
			category,
			time.Since(startedAt),
		)
	}()

	targetURL, resolvedRemoteName, err := resolveRemoteAdminURL(project, remoteName)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	result, err := openDestination(
		ctx,
		targetURL,
		fmt.Sprintf("Opening %s...", targetURL),
	)
	if err != nil {
		message = err.Error()
		return "", err
	}

	status = engine.OperationStatusSuccess
	category = ""
	message = fmt.Sprintf("remote URL opened for %s", resolvedRemoteName)
	return result, nil
}

func openRemoteDB(project string, remoteName string) (string, error) {
	trimmedRemoteName := strings.TrimSpace(remoteName)
	if trimmedRemoteName == "" {
		return "", fmt.Errorf("remote name is required")
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		return "", fmt.Errorf("load config for remotes: %w", err)
	}

	resolvedRemoteName, _, err := resolveRemoteConfigForCapability(cfg, trimmedRemoteName, engine.RemoteCapabilityDB)
	if err != nil {
		return "", err
	}

	if err := startGovardCommandForDesktop(root, []string{"open", "db", "-e", resolvedRemoteName, "--client"}); err != nil {
		return "", err
	}

	return fmt.Sprintf("Opening remote database client for %s...", resolvedRemoteName), nil
}

func openRemoteSFTP(project string, remoteName string, ctx context.Context) (string, error) {
	trimmedRemoteName := strings.TrimSpace(remoteName)
	if trimmedRemoteName == "" {
		return "", fmt.Errorf("remote name is required")
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		return "", fmt.Errorf("load config for remotes: %w", err)
	}

	resolvedRemoteName, remoteCfg, err := resolveRemoteConfigForCapability(cfg, trimmedRemoteName, engine.RemoteCapabilityFiles)
	if err != nil {
		return "", err
	}

	target := buildRemoteSFTPURLForDesktop(remoteCfg)
	if ctx != nil {
		if opened, err := tryOpenSFTPWithFileZilla(target, remoteCfg); opened {
			return fmt.Sprintf("Opening SFTP for %s in FileZilla...", resolvedRemoteName), nil
		} else if err != nil {
			message := fmt.Sprintf("FileZilla launch failed: %v. ", err)
			fallbackMessage, fallbackErr := openDestination(ctx, target, fmt.Sprintf("Opening %s...", target))
			if fallbackErr != nil {
				return "", fallbackErr
			}
			return message + fallbackMessage, nil
		}
	}

	message, err := openDestination(ctx, target, fmt.Sprintf("Opening %s...", target))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Sprintf("Opening SFTP for %s...", resolvedRemoteName), nil
	}
	return message, nil
}

func openRemoteShell(project string, remoteName string, ctx context.Context) (string, error) {
	trimmedRemoteName := strings.TrimSpace(remoteName)
	if trimmedRemoteName == "" {
		return "", fmt.Errorf("remote name is required")
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		return "", fmt.Errorf("load config for remotes: %w", err)
	}

	resolvedRemoteName, remoteCfg, err := resolveRemoteConfigForCapability(cfg, trimmedRemoteName, engine.RemoteCapabilityFiles)
	if err != nil {
		return "", err
	}

	if ctx != nil {
		if opened, err := tryOpenSSHInTerminal(resolvedRemoteName, remoteCfg); opened {
			return fmt.Sprintf("Opening SSH for %s in terminal...", resolvedRemoteName), nil
		} else if err != nil {
			target := buildRemoteSSHURLForDesktop(remoteCfg)
			message := fmt.Sprintf("Terminal SSH launch failed: %v. ", err)
			fallbackMessage, fallbackErr := openDestination(ctx, target, fmt.Sprintf("Opening %s...", target))
			if fallbackErr != nil {
				return "", fallbackErr
			}
			return message + fallbackMessage, nil
		}
	}

	target := buildRemoteSSHURLForDesktop(remoteCfg)
	message, err := openDestination(ctx, target, fmt.Sprintf("Opening %s...", target))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Sprintf("Opening SSH for %s...", resolvedRemoteName), nil
	}
	return message, nil
}

func resolveRemoteAdminURL(project string, remoteName string) (string, string, error) {
	trimmedRemoteName := strings.TrimSpace(remoteName)
	if trimmedRemoteName == "" {
		return "", "", fmt.Errorf("remote name is required")
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", "", err
	}

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		return "", "", fmt.Errorf("load config for remotes: %w", err)
	}

	resolvedRemoteName, remoteCfg, err := resolveRemoteConfigForOpen(cfg, trimmedRemoteName)
	if err != nil {
		return "", "", err
	}

	adminPath := conventions.DefaultAdminPath
	if engine.IsMagento2Family(cfg.Framework) {
		detectedPath, probeErr := detectRemoteMagentoAdminPathForDesktop(resolvedRemoteName, remoteCfg)
		if probeErr == nil {
			adminPath = detectedPath
		}
	}

	return buildRemoteAdminURLForDesktop(remoteCfg, adminPath), resolvedRemoteName, nil
}

func resolveRemoteConfigForOpen(
	cfg engine.Config,
	requestedRemoteName string,
) (string, engine.RemoteConfig, error) {
	return resolveRemoteConfigForCapability(cfg, requestedRemoteName, engine.RemoteCapabilityFiles)
}

func resolveRemoteConfigForCapability(
	cfg engine.Config,
	requestedRemoteName string,
	capability string,
) (string, engine.RemoteConfig, error) {
	trimmedRequested := strings.TrimSpace(requestedRemoteName)
	if trimmedRequested == "" {
		return "", engine.RemoteConfig{}, fmt.Errorf("remote name is required")
	}

	if remoteCfg, ok := cfg.Remotes[trimmedRequested]; ok {
		if capability != "" && !engine.RemoteCapabilityEnabled(remoteCfg, capability) {
			return "", engine.RemoteConfig{}, fmt.Errorf(
				"remote '%s' does not allow %s operations (capabilities: %s)",
				trimmedRequested,
				capability,
				strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
			)
		}
		return trimmedRequested, remoteCfg, nil
	}

	normalizedRequested := engine.NormalizeRemoteEnvironment(trimmedRequested)
	for name, remoteCfg := range cfg.Remotes {
		// Try standard alias normalization first (e.g. "Staging" -> "staging")
		if engine.NormalizeRemoteEnvironment(name) == normalizedRequested {
			if capability != "" && !engine.RemoteCapabilityEnabled(remoteCfg, capability) {
				return "", engine.RemoteConfig{}, fmt.Errorf(
					"remote '%s' does not allow %s operations (capabilities: %s)",
					name,
					capability,
					strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
				)
			}
			return name, remoteCfg, nil
		}

		// Fallback to simple case-insensitive match for custom names
		if strings.EqualFold(name, trimmedRequested) {
			if capability != "" && !engine.RemoteCapabilityEnabled(remoteCfg, capability) {
				return "", engine.RemoteConfig{}, fmt.Errorf(
					"remote '%s' does not allow %s operations (capabilities: %s)",
					name,
					capability,
					strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
				)
			}
			return name, remoteCfg, nil
		}
	}

	return "", engine.RemoteConfig{}, fmt.Errorf("unknown remote: %s", trimmedRequested)
}

func detectRemoteMagentoAdminPathForDesktop(
	remoteName string,
	remoteCfg engine.RemoteConfig,
) (string, error) {
	remoteCommand := "php -r " + engine.ShellQuote(remoteMagentoAdminProbeScript)
	if path := strings.TrimSpace(remoteCfg.Path); path != "" {
		remoteCommand = "cd " + engineremote.QuoteRemotePath(path) + " && " + remoteCommand
	}

	probeCmd := engineremote.BuildSSHExecCommand(remoteName, remoteCfg, true, remoteCommand)
	output, err := probeCmd.CombinedOutput()
	if err != nil {
		return conventions.DefaultAdminPath, fmt.Errorf("probe failed: %w", err)
	}

	value := strings.Trim(strings.TrimSpace(string(output)), "/")
	if value == "" {
		value = conventions.DefaultAdminPath
	}
	return value, nil
}

func buildRemoteAdminURLForDesktop(remoteCfg engine.RemoteConfig, adminPath string) string {
	base := strings.TrimSpace(remoteCfg.URL)
	if base == "" {
		base = strings.TrimSpace(remoteCfg.Host)
		if base == "" {
			base = "localhost"
		}
		if !strings.HasPrefix(strings.ToLower(base), "http://") &&
			!strings.HasPrefix(strings.ToLower(base), "https://") {
			base = "https://" + base
		}
	}

	base = strings.TrimRight(base, "/")
	trimmedPath := strings.Trim(strings.TrimSpace(adminPath), "/")
	if trimmedPath == "" {
		trimmedPath = conventions.DefaultAdminPath
	}

	return base + "/" + trimmedPath
}

func buildRemoteSFTPURLForDesktop(remoteCfg engine.RemoteConfig) string {
	host := strings.TrimSpace(remoteCfg.Host)
	if host == "" {
		host = "localhost"
	}
	port := remoteCfg.Port
	if port <= 0 {
		port = 22
	}

	targetURL := &url.URL{
		Scheme: "sftp",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   strings.TrimSpace(remoteCfg.Path),
	}
	user := strings.TrimSpace(remoteCfg.User)
	if user != "" {
		targetURL.User = url.User(user)
	}
	return targetURL.String()
}

func buildRemoteSSHURLForDesktop(remoteCfg engine.RemoteConfig) string {
	host := strings.TrimSpace(remoteCfg.Host)
	if host == "" {
		host = "localhost"
	}
	port := remoteCfg.Port
	if port <= 0 {
		port = 22
	}

	targetURL := &url.URL{
		Scheme: "ssh",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	user := strings.TrimSpace(remoteCfg.User)
	if user != "" {
		targetURL.User = url.User(user)
	}
	path := strings.TrimSpace(remoteCfg.Path)
	if path != "" {
		targetURL.Path = path
	}
	return targetURL.String()
}

func tryOpenSFTPWithFileZilla(target string, remoteCfg engine.RemoteConfig) (bool, error) {
	if runtime.GOOS == "linux" && strings.TrimSpace(os.Getenv("DISPLAY")) == "" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == "" {
		return false, nil
	}

	fileZillaBinary, err := exec.LookPath("filezilla")
	if err != nil {
		return false, nil
	}

	agentSock, err := resolveSSHAgentSocketForDesktop(remoteCfg)
	if err != nil {
		return false, err
	}

	args := []string{}
	authMethod := engineremote.NormalizeAuthMethod(remoteCfg.Auth.Method)
	if authMethod == engineremote.AuthMethodSSHAgent || authMethod == engineremote.AuthMethodKeyfile {
		args = append(args, "-l", "interactive")
	}
	args = append(args, target)

	cmd := exec.Command(fileZillaBinary, args...)
	if agentSock != "" {
		cmd.Env = append(os.Environ(), "SSH_AUTH_SOCK="+agentSock)
	}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start filezilla: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return true, nil
}

func tryOpenSSHInTerminal(remoteName string, remoteCfg engine.RemoteConfig) (bool, error) {
	if runtime.GOOS != "linux" {
		return false, nil
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) == "" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == "" {
		return false, nil
	}

	sshBinary, err := exec.LookPath("ssh")
	if err != nil {
		return false, fmt.Errorf("ssh binary not found: %w", err)
	}
	agentSock, err := resolveSSHAgentSocketForDesktop(remoteCfg)
	if err != nil {
		return false, err
	}

	sshArgs := engineremote.BuildSSHInteractiveArgs(remoteName, remoteCfg, true)
	sshArgs = append(
		sshArgs,
		engineremote.RemoteTarget(remoteCfg),
		buildRemoteShellCommandForDesktop(remoteCfg.Path),
	)

	type launcher struct {
		binary string
		prefix []string
	}

	launchers := []launcher{
		{binary: "x-terminal-emulator", prefix: []string{"-e", sshBinary}},
		{binary: "gnome-terminal", prefix: []string{"--", sshBinary}},
		{binary: "konsole", prefix: []string{"-e", sshBinary}},
		{binary: "xfce4-terminal", prefix: []string{"-x", sshBinary}},
	}

	lastErr := error(nil)
	launcherFound := false
	for _, candidate := range launchers {
		terminalBinary, lookupErr := exec.LookPath(candidate.binary)
		if lookupErr != nil {
			continue
		}
		launcherFound = true
		args := append([]string{}, candidate.prefix...)
		args = append(args, sshArgs...)

		cmd := exec.Command(terminalBinary, args...)
		if agentSock != "" {
			cmd.Env = append(os.Environ(), "SSH_AUTH_SOCK="+agentSock)
		}
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}

		go func() {
			_ = cmd.Wait()
		}()
		return true, nil
	}

	if launcherFound && lastErr != nil {
		return false, fmt.Errorf("start terminal launcher: %w", lastErr)
	}
	return false, nil
}

func buildRemoteShellCommandForDesktop(projectPath string) string {
	trimmedPath := strings.TrimSpace(projectPath)
	if trimmedPath == "" {
		return "(bash -l || sh)"
	}
	return "cd " + engineremote.QuoteRemotePath(trimmedPath) + " && (bash -l || sh)"
}

func resolveSSHAgentSocketForDesktop(remoteCfg engine.RemoteConfig) (string, error) {
	authMethod := engineremote.NormalizeAuthMethod(remoteCfg.Auth.Method)
	if authMethod != engineremote.AuthMethodSSHAgent {
		return "", nil
	}

	socket := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if socket != "" {
		return socket, nil
	}

	candidate := fmt.Sprintf("/run/user/%d/keyring/ssh", os.Getuid())
	if info, err := os.Stat(candidate); err == nil && info.Mode()&os.ModeSocket != 0 {
		return candidate, nil
	}

	return "", fmt.Errorf(
		"remote auth uses ssh-agent but SSH_AUTH_SOCK is unavailable; load key into ssh-agent or use auth.method=keyfile/keychain",
	)
}

func runRemoteSyncPresetWithOptions(
	project string,
	remoteName string,
	preset string,
	options map[string]bool,
) (string, error) {
	startedAt := time.Now()
	status := engine.OperationStatusFailure
	category := "runtime"
	message := ""
	defer func() {
		writeDesktopOperationEvent(
			"desktop.remote.sync.plan",
			status,
			project,
			remoteName,
			"local",
			message,
			category,
			time.Since(startedAt),
		)
	}()

	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		category = "validation"
		message = "remote name is required"
		return "", fmt.Errorf("%s", message)
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	snapshot, err := listProjectRemotesByPath(root)
	if err != nil {
		message = err.Error()
		return "", err
	}
	if !hasRemoteName(snapshot.Remotes, remoteName) {
		category = "validation"
		message = fmt.Sprintf("unknown remote: %s", remoteName)
		return "", fmt.Errorf("%s", message)
	}

	normalizedPreset, err := normalizeRemoteSyncPreset(preset)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	var args []string
	if normalizedPreset == "full" {
		args, err = buildBootstrapArgsWithOptions(remoteName, options, true)
	} else {
		args, err = buildRemoteSyncArgsWithOptions(remoteName, preset, options, true)
	}

	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	output, err := runGovardCommandForDesktopWithTimeout(root, args, 15*time.Minute)
	if err != nil {
		message = err.Error()
		return "", fmt.Errorf("sync plan failed: %w", err)
	}
	if output == "" {
		output = "Sync plan generated."
	}

	status = engine.OperationStatusPlan
	category = ""
	message = "sync plan generated"
	return output, nil
}

func runRemoteSyncBackgroundWithOptions(
	ctx context.Context,
	project string,
	remoteName string,
	preset string,
	options map[string]bool,
) error {
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		return fmt.Errorf("remote name is required")
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return err
	}

	normalizedPreset, err := normalizeRemoteSyncPreset(preset)
	if err != nil {
		return err
	}

	var args []string
	if normalizedPreset == "full" {
		args, err = buildBootstrapArgsWithOptions(remoteName, options, false)
	} else {
		args, err = buildRemoteSyncArgsWithOptions(remoteName, preset, options, false)
	}

	if err != nil {
		return err
	}

	// We no longer run in the background with event emitting to the UI for some things,
	// but for the visual-sync-console, we STILL need events.
	// HOWEVER, the user also wants to be able to open the OS terminal.
	// I will keep the background-with-events logic here, and add a SEPARATE bridge method for OS Terminal.

	binary, err := exec.LookPath("govard")
	if err != nil {
		return fmt.Errorf("govard CLI not found in PATH")
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = filepath.Clean(root)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to pipe stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to pipe stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background sync: %w", err)
	}

	RegisterSyncingProject(project, remoteName)
	emitEvent(ctx, "sync:started", map[string]string{
		"project": project,
		"remote":  remoteName,
		"preset":  preset,
	})

	// We use two scanners to emit events to the frontend
	done := make(chan struct{}, 2)
	go scanLogPipe(ctx, stdout, "sync:output", done)
	go scanLogPipe(ctx, stderr, "sync:output", done)

	go func() {
		<-done
		<-done
		err := cmd.Wait()
		UnregisterSyncingProject(project)
		if err != nil {
			emitEvent(ctx, "sync:failed", fmt.Sprintf("Sync failed: %v", err))
		} else {
			emitEvent(ctx, "sync:completed", "Sync completed successfully")
		}
	}()

	return nil
}

// LaunchInTerminal opens a new OS terminal window and executes the given command.
func LaunchInTerminal(workingDir string, command string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("OS Terminal launch is currently only supported on Linux")
	}

	// Try to get user's shell, fallback to bash which has better colors than sh
	userShell := os.Getenv("SHELL")
	if userShell == "" {
		userShell = "/bin/bash"
	}

	// Try to resolve the shell binary name to ensure it's valid
	shellBin, err := exec.LookPath(userShell)
	if err != nil {
		shellBin = "/bin/bash"
	}

	// Construct the command string to run inside the shell
	// We use 'read' to keep the terminal open for user inspection
	shCmd := fmt.Sprintf("cd %s && %s; echo; echo '---------------------------------------'; echo 'Process finished. Press Enter to close.'; read", engine.ShellQuote(workingDir), command)

	type launcher struct {
		binary string
		prefix []string
	}

	// List of supported terminal emulators with their specific execution flags
	launchers := []launcher{
		{binary: "gnome-terminal", prefix: []string{"--", shellBin, "-c", shCmd}},
		{binary: "tilix", prefix: []string{"-e", shellBin, "-c", shCmd}},
		{binary: "konsole", prefix: []string{"-e", shellBin, "-c", shCmd}},
		{binary: "xfce4-terminal", prefix: []string{"-x", shellBin, "-c", shCmd}},
		{binary: "alacritty", prefix: []string{"-e", shellBin, "-c", shCmd}},
		{binary: "kitty", prefix: []string{"sh", "-c", shCmd}},
		{binary: "x-terminal-emulator", prefix: []string{"-e", shellBin, "-c", shCmd}},
	}

	lastErr := error(nil)
	launcherFound := false
	for _, candidate := range launchers {
		terminalBinary, lookupErr := exec.LookPath(candidate.binary)
		if lookupErr != nil {
			continue
		}
		launcherFound = true
		args := append([]string{}, candidate.prefix...)

		cmd := exec.Command(terminalBinary, args...)
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}

		go func() {
			_ = cmd.Wait()
		}()
		return nil
	}

	if launcherFound && lastErr != nil {
		return fmt.Errorf("failed to start terminal: %w", lastErr)
	}
	return fmt.Errorf("no supported terminal emulator found")
}

func launchGovardInTerminal(root string, args []string) error {
	binary, err := exec.LookPath("govard")
	if err != nil {
		return fmt.Errorf("govard CLI not found in PATH")
	}

	quotedArgs := make([]string, len(args))
	for i, a := range args {
		quotedArgs[i] = engine.ShellQuote(a)
	}
	syncCmd := binary + " " + strings.Join(quotedArgs, " ")
	return LaunchInTerminal(root, syncCmd)
}

func resolveProjectRootForRemotes(project string) (string, int, error) {
	trimmedProject := strings.TrimSpace(project)
	if trimmedProject == "" {
		return "", -1, fmt.Errorf("project is required")
	}

	// 1. Check if it's already a path with a config (absolute or relative)
	if pathHasBaseConfig(trimmedProject) {
		return filepath.Clean(trimmedProject), engine.ScoreExact, nil
	}

	// 2. NEW: Exact match in registry (Name or Domain)
	if match, err := findExactProjectMatch(trimmedProject); err == nil {
		if pathHasBaseConfig(match.Path) {
			return filepath.Clean(match.Path), engine.ScoreExact, nil
		}
	}

	// 3. Try registry with fuzzy matching
	match, score, err := engine.FindProjectByQuery(trimmedProject)

	// 4. If weak or no match, check for EXACT orphan match
	if err != nil || score >= engine.ScoreAmbiguousThreshold {
		orphans, orphanErr := engine.GetOrphanedComposeProjects(context.Background())
		if orphanErr == nil {
			for _, o := range orphans {
				if strings.EqualFold(o.Name, trimmedProject) {
					// For orphan cleanup, return the name as the path and mark as exact match
					return o.Name, engine.ScoreExact, nil
				}
			}
		}
	}

	if err == nil {
		// Even if config is missing, return the path so it can be cleaned up
		return filepath.Clean(match.Path), score, nil
	}

	// 5. Fallback to loadProjectInfo (which checks Docker labels)
	if info, err := loadProjectInfo(trimmedProject); err == nil {
		if info.workingDir != "" && pathHasBaseConfig(info.workingDir) {
			return filepath.Clean(info.workingDir), engine.ScoreExact, nil
		}
		if info.configPath != "" {
			return filepath.Clean(filepath.Dir(info.configPath)), engine.ScoreExact, nil
		}
	}

	// 6. Try matching by base name if it's a domain/path
	if strings.Contains(trimmedProject, ".") || strings.Contains(trimmedProject, "/") {
		var base string
		if strings.Contains(trimmedProject, ".") {
			base = strings.Split(trimmedProject, ".")[0]
		} else {
			base = filepath.Base(trimmedProject)
		}
		if match, score, err := engine.FindProjectByQuery(base); err == nil {
			return filepath.Clean(match.Path), score, nil
		}
	}

	return "", -1, fmt.Errorf("unable to resolve project root for identifier '%s'", trimmedProject)
}

func findExactProjectMatch(query string) (engine.ProjectRegistryEntry, error) {
	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		return engine.ProjectRegistryEntry{}, err
	}

	lowerQuery := strings.ToLower(query)
	for _, entry := range entries {
		if strings.ToLower(entry.ProjectName) == lowerQuery ||
			strings.ToLower(entry.Domain) == lowerQuery {
			return entry, nil
		}
	}

	return engine.ProjectRegistryEntry{}, fmt.Errorf("no exact match for %q", query)
}

func pathHasBaseConfig(root string) bool {
	configPath := filepath.Join(filepath.Clean(strings.TrimSpace(root)), conventions.BaseConfigFile)
	info, err := os.Stat(configPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func buildRemoteEntries(
	remotes map[string]engine.RemoteConfig,
	lastSyncByRemote map[string]string,
) []RemoteEntry {
	if len(remotes) == 0 {
		return []RemoteEntry{}
	}

	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	engine.SortRemoteNames(names)

	entries := make([]RemoteEntry, 0, len(names))
	for _, name := range names {
		cfg := remotes[name]
		port := cfg.Port
		if port <= 0 {
			port = 22
		}

		capabilities := engine.RemoteCapabilityList(cfg)
		if len(capabilities) == 1 && capabilities[0] == "none" {
			capabilities = []string{}
		}

		effectiveProtected, _ := engine.RemoteWriteBlocked(name, cfg)

		entry := RemoteEntry{
			Name:         strings.TrimSpace(name),
			Host:         strings.TrimSpace(cfg.Host),
			User:         strings.TrimSpace(cfg.User),
			Path:         strings.TrimSpace(cfg.Path),
			Port:         port,
			LastSync:     strings.TrimSpace(lastSyncByRemote[strings.ToLower(strings.TrimSpace(name))]),
			Protected:    effectiveProtected,
			AuthMethod:   engineremote.NormalizeAuthMethod(cfg.Auth.Method),
			Capabilities: append([]string{}, capabilities...),
		}
		if entry.AuthMethod == "" {
			entry.AuthMethod = engineremote.AuthMethodKeychain
		}
		entries = append(entries, entry)
	}
	return entries
}

func buildRemoteLastSyncLabels(project string, now time.Time) map[string]string {
	events, err := engine.ReadOperationEvents(remoteLastSyncReadLimit)
	if err != nil {
		return map[string]string{}
	}
	return buildRemoteLastSyncLabelsFromEvents(project, events, now)
}

func buildRemoteLastSyncLabelsFromEvents(
	project string,
	events []engine.OperationEvent,
	now time.Time,
) map[string]string {
	trimmedProject := strings.TrimSpace(project)
	latestByRemote := map[string]time.Time{}

	for _, event := range events {
		if !isRemoteSyncOperation(event.Operation) {
			continue
		}
		if event.Status != engine.OperationStatusSuccess {
			continue
		}

		eventProject := strings.TrimSpace(event.Project)
		if trimmedProject != "" && eventProject != "" && !strings.EqualFold(eventProject, trimmedProject) {
			continue
		}

		source := strings.TrimSpace(event.Source)
		if source == "" || strings.EqualFold(source, "local") {
			continue
		}

		timestamp, ok := parseOperationTimestamp(event.Timestamp)
		if !ok {
			continue
		}

		remoteName := strings.ToLower(source)
		current, exists := latestByRemote[remoteName]
		if !exists || timestamp.After(current) {
			latestByRemote[remoteName] = timestamp
		}
	}

	labels := make(map[string]string, len(latestByRemote))
	for remoteName, timestamp := range latestByRemote {
		labels[remoteName] = formatLastSyncLabel(timestamp, now)
	}
	return labels
}

func isRemoteSyncOperation(operation string) bool {
	switch strings.TrimSpace(operation) {
	case "sync.run", "bootstrap.run":
		return true
	default:
		return false
	}
}

func parseOperationTimestamp(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}

	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

func formatLastSyncLabel(timestamp time.Time, now time.Time) string {
	ts := timestamp.UTC()
	ref := now.UTC()
	if ref.IsZero() {
		ref = time.Now().UTC()
	}
	if ref.Before(ts) {
		return "just now"
	}

	elapsed := ref.Sub(ts)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed/time.Minute))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed/time.Hour))
	case elapsed < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(elapsed/(24*time.Hour)))
	case elapsed < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(elapsed/(7*24*time.Hour)))
	default:
		return ts.Format("2006-01-02")
	}
}

func buildRemoteSyncPlanArgs(remoteName string, preset string) ([]string, error) {
	return buildRemoteSyncPlanArgsWithOptions(
		remoteName,
		preset,
		map[string]bool{"compress": true},
	)
}

func buildRemoteSyncPlanArgsWithOptions(
	remoteName string,
	preset string,
	options map[string]bool,
) ([]string, error) {
	return buildRemoteSyncArgsWithOptions(remoteName, preset, options, true)
}

func buildRemoteSyncArgsWithOptions(
	remoteName string,
	preset string,
	options map[string]bool,
	planOnly bool,
) ([]string, error) {
	normalizedPreset, err := normalizeRemoteSyncPreset(preset)
	if err != nil {
		return nil, err
	}

	args := []string{"sync", "--source", strings.TrimSpace(remoteName), "--destination", "local"}
	switch normalizedPreset {
	case "files":
		args = append(args, "--file")
	case "media":
		args = append(args, "--media", "optimized")
	case "db":
		args = append(args, "--db")
	case "full":
		args = append(args, "--full")
	default:
		return nil, fmt.Errorf("unsupported sync preset '%s'", normalizedPreset)
	}

	// Default to true for compress unless explicitly set to false
	compress, hasCompress := options["compress"]
	// Compression toggle only applies to rsync scopes (files/media/full).
	if hasCompress && !compress && (normalizedPreset == "files" || normalizedPreset == "media" || normalizedPreset == "full") {
		args = append(args, "--no-compress")
	}

	if options["noNoise"] && (normalizedPreset == "db" || normalizedPreset == "full" || normalizedPreset == "files") {
		args = append(args, "--no-noise")
	}

	if options["noPii"] && (normalizedPreset == "db" || normalizedPreset == "full") {
		args = append(args, "--no-pii")
	}

	// Delete toggle only applies to rsync scopes (files/media/full).
	if options["delete"] && (normalizedPreset == "files" || normalizedPreset == "media" || normalizedPreset == "full") {
		args = append(args, "--delete")
	}

	if planOnly {
		args = append(args, "--plan")
	} else {
		args = append(args, "--yes")
	}
	return args, nil
}

type presetSyncOptions struct {
	Preset  string            `json:"preset"`
	Command string            `json:"command"`
	Options []presetOptionDef `json:"options"`
}

type presetOptionDef struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	DefaultValue bool   `json:"defaultValue"`
}

func buildPresetSyncOptionDefs(project, preset string) presetSyncOptions {
	normalizedPreset, _ := normalizeRemoteSyncPreset(preset)

	framework := ""
	if root, _, err := resolveProjectRootForRemotes(project); err == nil {
		if cfg, _, err := engine.LoadConfigFromDir(root, true); err == nil {
			framework = strings.ToLower(strings.TrimSpace(cfg.Framework))
		}
	}

	isMagento := engine.IsMagento2Family(framework) || framework == "magento1" || framework == "openmage"

	switch normalizedPreset {
	case "db":
		opts := []presetOptionDef{
			{Key: "noNoise", Label: "Exclude Noise", Description: "Exclude ephemeral/noise tables (logs, caches, etc)", DefaultValue: false},
			{Key: "noPii", Label: "Exclude PII", Description: "Exclude PII/sensitive tables (users, orders, etc)", DefaultValue: false},
		}
		return presetSyncOptions{
			Preset:  "db",
			Command: "sync",
			Options: opts,
		}
	case "media":
		opts := []presetOptionDef{
			{Key: "noNoise", Label: "Exclude Noise", Description: "Exclude logs and sensitive configs (.env, keys, etc)", DefaultValue: false},
			{Key: "compress", Label: "Use Compression", Description: "Compress data during transfer", DefaultValue: false},
			{Key: "delete", Label: "Delete Missing Files", Description: "Delete files on destination that are missing on source", DefaultValue: false},
		}
		return presetSyncOptions{
			Preset:  "media",
			Command: "sync",
			Options: opts,
		}
	case "full":
		opts := []presetOptionDef{
			// 1. Scopes
			{Key: "noDb", Label: "Skip DB Import", Description: "Do not import the database", DefaultValue: false},
			{Key: "noMedia", Label: "Skip Media Sync", Description: "Do not sync media files", DefaultValue: false},
			{Key: "noComposer", Label: "Skip Composer", Description: "Do not run composer install", DefaultValue: false},
		}
		if isMagento {
			opts = append(opts,
				presetOptionDef{Key: "noAdmin", Label: "Skip Admin Creation", Description: "Do not create an admin user", DefaultValue: false},
			)
		}

		opts = append(opts,
			// 2. Privacy & Scrubbing
			presetOptionDef{Key: "noNoise", Label: "Exclude Noise", Description: "Exclude ephemeral/noise tables, logs, and sensitive metadata", DefaultValue: false},
			presetOptionDef{Key: "noPii", Label: "Exclude PII", Description: "Exclude PII/sensitive database tables", DefaultValue: false},

			// 3. Transfer & Performance
			presetOptionDef{Key: "delete", Label: "Delete Missing Files", Description: "Delete files on destination missing on source (media/files)", DefaultValue: false},
			presetOptionDef{Key: "noStreamDb", Label: "Disable Stream DB", Description: "Do not stream database via pipe", DefaultValue: false},

			// 4. UX & Execution
			presetOptionDef{Key: "noUp", Label: "Skip Govard Up", Description: "Do not run govard up before bootstrap", DefaultValue: false},
		)

		return presetSyncOptions{
			Preset:  "full",
			Command: "bootstrap",
			Options: opts,
		}
	default:
		// Fallback for "files" or unknown presets
		opts := []presetOptionDef{
			{Key: "noNoise", Label: "Exclude Noise", Description: "Exclude logs and sensitive configs (.env, keys, etc)", DefaultValue: false},
			{Key: "compress", Label: "Use Compression", Description: "Compress data during transfer", DefaultValue: false},
			{Key: "delete", Label: "Delete Missing Files", Description: "Delete files on destination missing on source", DefaultValue: false},
		}
		return presetSyncOptions{
			Preset:  normalizedPreset,
			Command: "sync",
			Options: opts,
		}
	}
}

func buildBootstrapArgsWithOptions(remoteName string, options map[string]bool, planOnly bool) ([]string, error) {
	args := []string{"bootstrap", "--environment", strings.TrimSpace(remoteName)}

	if planOnly {
		args = append(args, "--plan")
	}

	if options["noDb"] {
		args = append(args, "--no-db")
	}
	if options["noMedia"] {
		args = append(args, "--no-media")
	}
	if options["noComposer"] {
		args = append(args, "--no-composer")
	}
	if options["noAdmin"] {
		args = append(args, "--no-admin")
	}
	if options["noUp"] {
		args = append(args, "--no-up")
	}
	if options["noStreamDb"] {
		args = append(args, "--no-stream-db")
	}
	if options["noNoise"] {
		args = append(args, "--no-noise")
	}
	if options["noPii"] {
		args = append(args, "--no-pii")
	}
	if !planOnly {
		args = append(args, "--yes")
	}

	return args, nil
}

func normalizeRemoteSyncPreset(preset string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "", "file", "files", "code", "source":
		return "files", nil
	case "media", "assets":
		return "media", nil
	case "db", "database":
		return "db", nil
	case "full", "all":
		return "full", nil
	default:
		return "", fmt.Errorf("unsupported sync preset '%s'", preset)
	}
}

func writeBaseConfig(root string, config engine.Config) error {
	writableConfig := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writableConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, conventions.BaseConfigFile), data, conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write %s: %w", conventions.BaseConfigFile, err)
	}
	return nil
}

func hasRemoteName(remotes []RemoteEntry, name string) bool {
	for _, remote := range remotes {
		if remote.Name == name {
			return true
		}
	}
	return false
}

// RemoteService methods

func (s *RemoteService) GetRemotes(project string) (RemoteSnapshot, error) {
	snapshot, err := listProjectRemotes(project)
	if err != nil {
		return RemoteSnapshot{
			Project:  project,
			Remotes:  []RemoteEntry{},
			Warnings: []string{},
		}, err
	}
	return snapshot, nil
}

// ResolveProjectRootForRemotesForTest matches the project identifier to its root path.
func ResolveProjectRootForRemotesForTest(project string) (string, error) {
	root, _, err := resolveProjectRootForRemotes(project)
	return root, err
}

func (s *RemoteService) TestRemote(project string, remoteName string) (string, error) {
	message, err := testRemote(project, remoteName)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) OpenRemoteURL(project string, remoteName string) (string, error) {
	message, err := openRemoteURL(project, remoteName, s.ctx)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) OpenRemoteDB(project string, remoteName string) (string, error) {
	message, err := openRemoteDB(project, remoteName)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) OpenRemoteSFTP(project string, remoteName string) (string, error) {
	message, err := openRemoteSFTP(project, remoteName, s.ctx)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) OpenRemoteShell(project string, remoteName string) (string, error) {
	message, err := openRemoteShell(project, remoteName, s.ctx)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) GetSyncOptions(project, preset string) presetSyncOptions {
	return buildPresetSyncOptionDefs(project, preset)
}

func (s *RemoteService) RunRemoteSyncPreset(project string, remoteName string, preset string, options map[string]bool) (string, error) {
	message, err := runRemoteSyncPresetWithOptions(project, remoteName, preset, options)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) RunRemoteSyncInTerminal(project string, remoteName string, preset string, options map[string]bool) (string, error) {
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		return "", fmt.Errorf("remote name is required")
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}

	normalizedPreset, err := normalizeRemoteSyncPreset(preset)
	if err != nil {
		return "", err
	}

	var args []string
	if normalizedPreset == "full" {
		args, err = buildBootstrapArgsWithOptions(remoteName, options, false)
	} else {
		args, err = buildRemoteSyncArgsWithOptions(remoteName, preset, options, false)
	}

	if err != nil {
		return "", err
	}

	err = launchGovardInTerminal(root, args)
	if err != nil {
		return "", err
	}

	return "Sync started in terminal", nil
}

func (s *RemoteService) RunRemoteSync(project string, remoteName string, preset string, options map[string]bool) (string, error) {
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	err := runRemoteSyncBackgroundWithOptions(ctx, project, remoteName, preset, options)
	if err != nil {
		return "", err
	}
	return "Sync started", nil
}

func writeDesktopOperationEvent(
	operation string,
	status engine.OperationStatus,
	project string,
	source string,
	destination string,
	message string,
	category string,
	duration time.Duration,
) {
	_ = engine.WriteOperationEvent(engine.OperationEvent{
		Operation:   operation,
		Status:      status,
		Project:     strings.TrimSpace(project),
		Source:      strings.TrimSpace(source),
		Destination: strings.TrimSpace(destination),
		Message:     strings.TrimSpace(message),
		Category:    strings.TrimSpace(category),
		DurationMS:  duration.Milliseconds(),
	})
}
