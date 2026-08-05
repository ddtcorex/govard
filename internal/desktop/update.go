package desktop

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/updater"
)

const desktopBinaryName = "govard-desktop"
const desktopSelfUpdateDesktopTargetEnvVar = "GOVARD_SELF_UPDATE_DESKTOP_TARGET"

var desktopUpdateHTTPClient = &http.Client{Timeout: 5 * time.Second}
var desktopExecutablePath = os.Executable
var desktopBinaryLookPath = exec.LookPath
var desktopGovardLookPath = exec.LookPath
var desktopPrivilegedCommandLookPath = exec.LookPath
var desktopANSIEscapePattern = regexp.MustCompile(`(?:\x1B|\x9B)\[[0-?]*[ -/]*[@-~]`)
var desktopControlCharPattern = regexp.MustCompile(`[\x00-\x08\x0B-\x1F\x7F]`)
var desktopPermissionDeniedPattern = regexp.MustCompile(`(?i)permission denied replacing\s+\S+\s+at\s+([^;\n]+)`)
var desktopAuthorizationDeniedPattern = regexp.MustCompile(`(?i)(request dismissed|not authorized|authorization failed|authentication failed|authentication is required|pkexec)`)

var defaultRunDesktopSelfUpdate = func() (string, error) {
	binary, err := resolveGovardBinaryForDesktopUpdate()
	if err != nil {
		return "", fmt.Errorf("govard CLI not found in PATH")
	}

	desktopTarget := resolveDesktopBinaryForSelfUpdateTarget()

	output, err := runDesktopSelfUpdateCommand(binary, desktopTarget, false)
	if err == nil {
		return output, nil
	}

	if runtime.GOOS != "linux" {
		return "", err
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if !strings.Contains(lower, "requires elevated privileges") {
		return "", err
	}

	return runDesktopSelfUpdateCommand(binary, desktopTarget, true)
}

func runDesktopSelfUpdateCommand(govardBinary, desktopTarget string, elevated bool) (string, error) {
	if strings.TrimSpace(govardBinary) == "" {
		return "", errors.New("govard CLI path is empty")
	}

	var output []byte
	var err error

	if elevated {
		pkexecPath, lookupErr := desktopPrivilegedCommandLookPath("pkexec")
		if lookupErr != nil {
			return "", errors.New(`update requires elevated privileges. Run "sudo govard self-update --yes" in Terminal, then reopen Govard Desktop`)
		}

		envBinary := "/usr/bin/env"
		if _, err := os.Stat(envBinary); err != nil {
			lookedUpEnv, lookupErr := desktopPrivilegedCommandLookPath("env")
			if lookupErr != nil {
				return "", errors.New(`update requires elevated privileges. Run "sudo govard self-update --yes" in Terminal, then reopen Govard Desktop`)
			}
			envBinary = lookedUpEnv
		}

		args := buildDesktopElevatedSelfUpdateArgs(envBinary, govardBinary, desktopTarget)

		output, err = runDesktopPrivilegedSelfUpdate(pkexecPath, args)
	} else {
		cmd := exec.Command(govardBinary, "self-update", "--yes")
		cmd.Env = append(
			os.Environ(),
			"NO_COLOR=1",
			"CLICOLOR=0",
		)
		if strings.TrimSpace(desktopTarget) != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", desktopSelfUpdateDesktopTargetEnvVar, desktopTarget))
		}
		output, err = cmd.CombinedOutput()
	}

	trimmed := sanitizeDesktopSelfUpdateOutput(string(output))
	if err != nil {
		return "", errors.New(summarizeDesktopSelfUpdateError(err, trimmed))
	}
	return trimmed, nil
}

// buildDesktopElevatedSelfUpdateArgs builds the "pkexec /usr/bin/env ..." argument
// list used to run "govard self-update" with elevated privileges. It forwards the
// invoking process's resolved Govard home directory as an explicit GOVARD_HOME_DIR
// env var (read from the invoking process's own environment before pkexec strips
// it), so the elevated child resolves the exact same persisted update channel
// (~/.govard/update-channel.json) as the invoking user, rather than silently
// falling back to the elevated user's home directory (typically /root).
func buildDesktopElevatedSelfUpdateArgs(envBinary, govardBinary, desktopTarget string) []string {
	args := []string{
		envBinary,
		"NO_COLOR=1",
		"CLICOLOR=0",
		fmt.Sprintf("%s=%s", conventions.EnvGovardHome, conventions.GetGovardHome()),
	}
	if strings.TrimSpace(desktopTarget) != "" {
		args = append(args, fmt.Sprintf("%s=%s", desktopSelfUpdateDesktopTargetEnvVar, desktopTarget))
	}
	args = append(args, govardBinary, "self-update", "--yes")
	return args
}

func runDesktopPrivilegedSelfUpdate(pkexecPath string, baseArgs []string) ([]byte, error) {
	if strings.TrimSpace(pkexecPath) == "" {
		return nil, errors.New("pkexec path is empty")
	}

	variants := [][]string{
		baseArgs,
		append([]string{"--disable-internal-agent"}, baseArgs...),
	}

	var lastOutput []byte
	var lastErr error
	for i, args := range variants {
		cmd := exec.Command(pkexecPath, args...)
		cmd.Env = append(
			os.Environ(),
			"NO_COLOR=1",
			"CLICOLOR=0",
		)

		output, err := cmd.CombinedOutput()
		if err == nil {
			return output, nil
		}

		lastOutput = output
		lastErr = err

		if i >= len(variants)-1 {
			break
		}

		sanitized := strings.ToLower(sanitizeDesktopSelfUpdateOutput(string(output)))
		errText := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(sanitized, "request dismissed") || strings.Contains(errText, "request dismissed") || strings.Contains(sanitized, "not authorized") || strings.Contains(errText, "not authorized") || strings.Contains(sanitized, "authorization failed") || strings.Contains(errText, "authorization failed") {
			continue
		}

		break
	}

	return lastOutput, lastErr
}

var runDesktopSelfUpdate = defaultRunDesktopSelfUpdate

var defaultRestartDesktopBinary = func(binaryPath string) error {
	var cmd *exec.Cmd

	// We must delay the child process launch slightly so that the parent Wails process
	// has time to quit and release the SingleInstanceLock. If the child attempts to lock
	// immediately, it will be rejected because the parent hasn't fully shut down yet.
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd.exe", "/c", "timeout /t 2 /nobreak > nul & start \"\" \""+binaryPath+"\"")
	case "darwin":
		cmd = exec.Command("sh", "-c", "sleep 1.5 && exec \"$0\" \"$@\"", binaryPath)
	default:
		// Linux: use gtk-launch to maintain desktop environment integration (dock icons),
		// falling back to direct binary execution if gtk-launch isn't available.
		cmdStr := `sleep 1.5 && (command -v gtk-launch >/dev/null 2>&1 && gtk-launch govard || exec "$0" "$@")`
		cmd = exec.Command("sh", "-c", cmdStr, binaryPath)
	}

	cmd.Env = os.Environ()

	// Detach process to prevent it being killed with the parent
	// and to ensure the window manager sees it as a fresh top-level application instance.
	setSysProcAttrForDetach(cmd)

	return cmd.Start()
}

var restartDesktopBinary = defaultRestartDesktopBinary

func (app *App) CheckForUpdates() (UpdateCheckResult, error) {
	current := normalizeDesktopVersionTag(Version)
	channel := updater.GetChannel()
	result := UpdateCheckResult{
		CurrentVersion: current,
		Channel:        channel,
	}

	release, err := updater.FetchChannelRelease(desktopUpdateHTTPClient, channel)
	if err != nil {
		result.Message = "Could not check for updates."
		return result, err
	}

	result.LatestVersion = normalizeDesktopVersionTag(release.Tag)
	result.Changelog = release.Body
	result.Outdated = updater.ShouldNotifyUpdate(Version, release.Tag)
	if result.Outdated {
		result.Message = fmt.Sprintf("Update available: %s -> %s", current, result.LatestVersion)
		return result, nil
	}

	result.Message = fmt.Sprintf("Govard Desktop is up to date (%s).", current)
	return result, nil
}

func (app *App) GetUpdateChannel() (string, error) {
	return updater.GetChannel(), nil
}

func (app *App) SetUpdateChannel(channel string) (string, error) {
	if err := updater.SetChannel(channel); err != nil {
		return "", err
	}
	return updater.GetChannel(), nil
}

func (app *App) InstallLatestUpdate() (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("automatic update is not supported on Windows yet; install a fresh release instead")
	}

	output, err := runDesktopSelfUpdate()
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(output) == "" {
		return "Update completed. Restart Govard Desktop to run the new version.", nil
	}
	return output, nil
}

func resolveGovardBinaryForDesktopUpdate() (string, error) {
	candidates := []string{}

	if executablePath, err := desktopExecutablePath(); err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(executablePath); resolveErr == nil {
			executablePath = resolved
		}
		sibling := filepath.Join(filepath.Dir(executablePath), "govard")
		if runtime.GOOS == "windows" {
			sibling += ".exe"
		}
		candidates = append(candidates, sibling)
	}

	if pathFromPATH, err := desktopGovardLookPath("govard"); err == nil {
		candidates = append(candidates, pathFromPATH)
	}

	for _, candidate := range candidates {
		clean := strings.TrimSpace(candidate)
		if clean == "" {
			continue
		}
		if resolved, resolveErr := filepath.EvalSymlinks(clean); resolveErr == nil {
			clean = resolved
		}
		if stat, statErr := os.Stat(clean); statErr == nil && !stat.IsDir() {
			return clean, nil
		}
	}

	return "", errors.New("govard CLI not found")
}

func resolveDesktopBinaryForSelfUpdateTarget() string {
	executablePath, err := desktopExecutablePath()
	if err != nil {
		return ""
	}

	if resolved, resolveErr := filepath.EvalSymlinks(executablePath); resolveErr == nil {
		executablePath = resolved
	}
	executablePath = strings.TrimSpace(executablePath)
	if executablePath == "" {
		return ""
	}

	if stat, statErr := os.Stat(executablePath); statErr != nil || stat.IsDir() {
		return ""
	}
	return executablePath
}

func sanitizeDesktopSelfUpdateOutput(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = desktopANSIEscapePattern.ReplaceAllString(normalized, "")
	normalized = strings.ReplaceAll(normalized, "\u241B", "")

	lines := strings.Split(normalized, "\n")
	sanitized := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := desktopControlCharPattern.ReplaceAllString(line, "")
		trimmed := strings.TrimSpace(clean)
		if trimmed == "" {
			continue
		}
		sanitized = append(sanitized, trimmed)
	}

	return strings.Join(sanitized, "\n")
}

func summarizeDesktopSelfUpdateError(runErr error, sanitizedOutput string) string {
	if message := desktopPermissionDeniedHint(sanitizedOutput); message != "" {
		return message
	}

	if message := desktopAuthorizationDeniedHint(sanitizedOutput); message != "" {
		return message
	}

	if message := extractDesktopSelfUpdateErrorMessage(sanitizedOutput); message != "" {
		return message
	}

	if runErr != nil {
		trimmed := strings.TrimSpace(runErr.Error())
		if trimmed != "" {
			return trimmed
		}
	}

	return "automatic update failed"
}

func desktopPermissionDeniedHint(sanitizedOutput string) string {
	match := desktopPermissionDeniedPattern.FindStringSubmatch(sanitizedOutput)
	if len(match) < 2 {
		return ""
	}

	targetPath := strings.TrimSpace(match[1])
	if targetPath == "" {
		return `Update requires elevated privileges. Run "sudo govard self-update" in Terminal, then reopen Govard Desktop.`
	}

	return fmt.Sprintf(
		`Update requires elevated privileges to modify %s. Run "sudo govard self-update" in Terminal, then reopen Govard Desktop.`,
		targetPath,
	)
}

func desktopAuthorizationDeniedHint(sanitizedOutput string) string {
	if !desktopAuthorizationDeniedPattern.MatchString(sanitizedOutput) {
		return ""
	}

	return `Administrator authorization was not granted. Run "sudo govard self-update --yes" in Terminal, then reopen Govard Desktop.`
}

func extractDesktopSelfUpdateErrorMessage(sanitizedOutput string) string {
	if strings.TrimSpace(sanitizedOutput) == "" {
		return ""
	}

	lines := strings.Split(sanitizedOutput, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "error:") {
			message := strings.TrimSpace(line[len("error:"):])
			if message != "" {
				return message
			}
			continue
		}

		if isDesktopSelfUpdateNoiseLine(lower) {
			continue
		}
		return line
	}

	return ""
}

func isDesktopSelfUpdateNoiseLine(lowerTrimmedLine string) bool {
	if lowerTrimmedLine == "" {
		return true
	}

	prefixes := []string{
		"govard self-update",
		"usage:",
		"flags:",
		"global flags:",
		"--",
		"info ",
		"success ",
		"warning ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerTrimmedLine, prefix) {
			return true
		}
	}

	return false
}

func (app *App) RestartDesktopApp() (string, error) {
	binaryPath, err := resolveDesktopBinaryForRestart()
	if err != nil {
		return "", fmt.Errorf("resolve desktop binary for restart: %w", err)
	}

	if err := restartDesktopBinary(binaryPath); err != nil {
		return "", fmt.Errorf("restart desktop app: %w", err)
	}

	if app != nil && app.ctx != nil {
		// Give the child process time to initialize and the RPC layer
		// a moment to flush the response before quitting the parent.
		go func() {
			time.Sleep(800 * time.Millisecond)
			quitApplication(app.ctx)
		}()
	}

	return "Restarting Govard Desktop...", nil
}

func resolveDesktopBinaryForRestart() (string, error) {
	candidates := []string{}

	if executable, err := desktopExecutablePath(); err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
			executable = resolved
		}
		candidates = append(candidates, executable)
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), desktopBinaryName))
	}

	if lookedUp, err := desktopBinaryLookPath(desktopBinaryName); err == nil {
		candidates = append(candidates, lookedUp)
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if resolved, resolveErr := filepath.EvalSymlinks(trimmed); resolveErr == nil {
			trimmed = resolved
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true

		if stat, err := os.Stat(trimmed); err == nil && !stat.IsDir() {
			return trimmed, nil
		}
	}

	return "", fmt.Errorf("%s not found", desktopBinaryName)
}

func normalizeDesktopVersionTag(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed
	}
	return "v" + trimmed
}
