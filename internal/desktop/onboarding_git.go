package desktop

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// desktopGitValidateTimeout bounds `git ls-remote` reachability probes: a
// hung remote must fail onboarding fast instead of wedging the binding.
var desktopGitValidateTimeout = 2 * time.Minute

// desktopGitCloneTimeout bounds `git clone --depth 1`: shallow clones of
// large repos run long, but a silent one has hung.
var desktopGitCloneTimeout = 15 * time.Minute

var defaultValidateGitConnectionForDesktop = func(protocol string, repoURL string) error {
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is not available in PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), desktopGitValidateTimeout)
	defer cancel()
	output, err := runDesktopGitCommand(ctx, gitBinary, "", "ls-remote", "--exit-code", repoURL, "HEAD")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git connection validation timed out after %s", desktopGitValidateTimeout)
		}
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			return fmt.Errorf("%s", trimmed)
		}
		return err
	}
	return nil
}

var validateGitConnectionForDesktop = defaultValidateGitConnectionForDesktop

var defaultCloneGitRepoForDesktop = func(repoURL string, destination string) error {
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is not available in PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), desktopGitCloneTimeout)
	defer cancel()
	output, err := runDesktopGitCommand(ctx, gitBinary, filepath.Clean(destination), "clone", "--depth", "1", repoURL, ".")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone timed out after %s", desktopGitCloneTimeout)
		}
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			return fmt.Errorf("%s", trimmed)
		}
		return err
	}
	return nil
}

// runDesktopGitCommand runs git with combined output under the ceiling ctx.
// exec.CommandContext only kills the direct child, and grandchildren holding
// the pipes (ssh, helpers) keep CombinedOutput blocked past the deadline, so
// the ceiling kills the whole process tree instead. The watchdog arms after
// Start: arming before would read cmd.Process from a second goroutine while
// Start writes it, which `go test -race` rightfully flags.
func runDesktopGitCommand(ctx context.Context, gitBinary, dir string, args ...string) (string, error) {
	cmd := exec.Command(gitBinary, args...)
	setDesktopProcessGroup(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Start(); err != nil {
		return "", err
	}
	stopWatchdog := context.AfterFunc(ctx, func() { killDesktopProcessTree(cmd) })
	defer stopWatchdog()
	if err := cmd.Wait(); err != nil {
		return combined.String(), err
	}
	return combined.String(), nil
}

var cloneGitRepoForDesktop = defaultCloneGitRepoForDesktop

type onboardingCloneStepReporter func(step string, message string)

func cloneProjectSourceFromGit(projectPath string, gitProtocol string, gitURL string, report onboardingCloneStepReporter) error {
	normalizedProtocol, err := normalizeOnboardingGitProtocol(gitProtocol)
	if err != nil {
		return err
	}

	normalizedURL := strings.TrimSpace(gitURL)
	if err := validateOnboardingGitURL(normalizedProtocol, normalizedURL); err != nil {
		return err
	}

	reportCloneStep(report, "git.validate", "Validating Git connection...")
	if err := validateGitConnectionForDesktop(normalizedProtocol, normalizedURL); err != nil {
		return fmt.Errorf("%s", buildGitConnectionHelp(normalizedProtocol, normalizedURL, err))
	}

	reportCloneStep(report, "folder.prepare", "Preparing target folder...")
	if err := prepareGitCloneDestination(projectPath); err != nil {
		return err
	}

	reportCloneStep(report, "git.clone", "Cloning source repository...")
	if err := cloneGitRepoForDesktop(normalizedURL, projectPath); err != nil {
		return fmt.Errorf("clone repository failed: %w", err)
	}

	return nil
}

func normalizeOnboardingGitProtocol(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "ssh":
		return "ssh", nil
	case "https":
		return "https", nil
	default:
		return "", fmt.Errorf("unsupported git protocol %q (expected ssh or https)", value)
	}
}

func validateOnboardingGitURL(protocol string, repoURL string) error {
	if strings.TrimSpace(repoURL) == "" {
		return fmt.Errorf("git repository URL is required when Git onboarding is enabled")
	}

	lowerURL := strings.ToLower(strings.TrimSpace(repoURL))
	switch protocol {
	case "ssh":
		if strings.HasPrefix(lowerURL, "git@") || strings.HasPrefix(lowerURL, "ssh://") {
			return nil
		}
		return fmt.Errorf("invalid SSH git URL %q (expected git@host:org/repo.git or ssh://...)", repoURL)
	case "https":
		if strings.HasPrefix(lowerURL, "https://") {
			return nil
		}
		return fmt.Errorf("invalid HTTPS git URL %q (expected https://...)", repoURL)
	default:
		return fmt.Errorf("unsupported git protocol %q", protocol)
	}
}

func prepareGitCloneDestination(projectPath string) error {
	absPath, err := filepath.Abs(filepath.Clean(strings.TrimSpace(projectPath)))
	if err != nil {
		return fmt.Errorf("resolve clone destination: %w", err)
	}
	cleanPath := filepath.Clean(absPath)
	if cleanPath == "" || cleanPath == "." || cleanPath == string(filepath.Separator) {
		return fmt.Errorf("invalid clone destination path: %s", projectPath)
	}
	if err := validateSafeCloneDestination(cleanPath); err != nil {
		return err
	}

	entries, readErr := os.ReadDir(cleanPath)
	if readErr != nil {
		return fmt.Errorf("read selected project folder: %w", readErr)
	}

	for _, entry := range entries {
		target := filepath.Join(cleanPath, entry.Name())
		if removeErr := os.RemoveAll(target); removeErr != nil {
			return fmt.Errorf("clear selected project folder: %w", removeErr)
		}
	}
	return nil
}

func validateSafeCloneDestination(cleanPath string) error {
	if isFilesystemRootPath(cleanPath) {
		return fmt.Errorf("refusing to clone into filesystem root path: %s", cleanPath)
	}

	// Wiping the destination deletes every entry in it, so hidden
	// directories (.config, .ssh and friends) are refused outright: nobody
	// onboards a project into one on purpose.
	if base := filepath.Base(cleanPath); strings.HasPrefix(base, ".") {
		return fmt.Errorf("refusing to clone into hidden directory: %s", cleanPath)
	}

	if home, err := os.UserHomeDir(); err == nil {
		if sameCleanPath(cleanPath, home) {
			return fmt.Errorf("refusing to clone into home directory: %s", cleanPath)
		}
		for _, name := range []string{"Work", "workspace", "projects"} {
			candidate := filepath.Join(home, name)
			if sameCleanPath(cleanPath, candidate) {
				return fmt.Errorf("refusing to clone into workspace root-like directory: %s", cleanPath)
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil && sameCleanPath(cleanPath, cwd) {
		return fmt.Errorf("refusing to clone into current working directory: %s", cleanPath)
	}

	return nil
}

func isFilesystemRootPath(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return false
	}
	if clean == string(filepath.Separator) {
		return true
	}
	volume := filepath.VolumeName(clean)
	if volume == "" {
		return false
	}
	remainder := strings.TrimPrefix(clean, volume)
	return remainder == "" || remainder == string(filepath.Separator)
}

func sameCleanPath(left string, right string) bool {
	leftClean, leftErr := filepath.Abs(filepath.Clean(strings.TrimSpace(left)))
	rightClean, rightErr := filepath.Abs(filepath.Clean(strings.TrimSpace(right)))
	if leftErr != nil || rightErr != nil {
		return false
	}
	return leftClean == rightClean
}

func reportCloneStep(report onboardingCloneStepReporter, step string, message string) {
	if report == nil {
		return
	}
	report(strings.TrimSpace(step), strings.TrimSpace(message))
}

func buildGitConnectionHelp(protocol string, repoURL string, cause error) string {
	// The cause carries raw git stderr (ANSI, control bytes) and the URL is
	// caller input: both are UI-bound via the returned error, so both are
	// sanitized before formatting.
	detail := strings.TrimSpace(sanitizeUIText(cause.Error(), commandTimeoutOutputMax))
	if detail == "" {
		detail = "unknown error"
	}
	safeURL := strings.TrimSpace(sanitizeUIText(repoURL, commandTimeoutOutputMax))

	if protocol == "ssh" {
		host := inferGitSSHHost(repoURL)
		return fmt.Sprintf(
			"Git SSH connection validation failed: %s. Setup steps: 1) load key into ssh-agent (`ssh-add -l`), 2) add public key to your Git provider account, 3) verify access with `ssh -T git@%s`.",
			detail,
			host,
		)
	}

	return fmt.Sprintf(
		"Git HTTPS connection validation failed: %s. Setup steps: 1) verify repository URL and access rights, 2) configure Git credentials/PAT in your credential helper, 3) verify access with `git ls-remote %s HEAD`.",
		detail,
		safeURL,
	)
}

func inferGitSSHHost(repoURL string) string {
	trimmed := strings.TrimSpace(repoURL)
	if strings.HasPrefix(trimmed, "git@") {
		hostPart := strings.TrimPrefix(trimmed, "git@")
		if index := strings.IndexAny(hostPart, ":/"); index > 0 {
			return hostPart[:index]
		}
	}

	if strings.HasPrefix(trimmed, "ssh://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			host := strings.TrimSpace(parsed.Hostname())
			if host != "" {
				return host
			}
		}
	}

	return "github.com"
}
