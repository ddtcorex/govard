package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"govard/internal/desktop"
	govardruntime "govard/internal/runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var desktopDev bool
var desktopBackground bool
var desktopExecutablePath = os.Executable
var desktopBinaryLookPath = exec.LookPath

// desktopVersionProbeTimeout bounds the `--version` probe `desktop doctor` runs
// against the desktop binary: an old build ignores the flag and opens its GUI,
// which would otherwise block the diagnosis forever.
const desktopVersionProbeTimeout = 5 * time.Second

// desktopVersionProbeWaitDelay bounds how long the probe keeps reading the
// child's output pipe after the deadline has passed.
const desktopVersionProbeWaitDelay = time.Second

// probeDesktopVersion asks a desktop binary for its version, bounded by ctx.
//
// Killing the child is not enough on its own: a build that ignores --version
// spawns its own children that inherit the output pipe, and reading it would
// then wait for them. WaitDelay closes the pipe, and the caller's deadline is
// reported as its own error so the diagnosis can name the cause.
func probeDesktopVersion(ctx context.Context, binaryPath string) (string, error) {
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	cmd.WaitDelay = desktopVersionProbeWaitDelay
	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", err
	}
	return string(out), nil
}

func desktopBuildTags(isProd bool) string {
	tags := []string{"desktop"}
	if isProd {
		tags = append(tags, "production")
	} else {
		tags = append(tags, "dev")
	}
	if runtime.GOOS == "linux" {
		tags = append(tags, "webkit2_41")
	}
	return strings.Join(tags, ",")
}

var desktopCmd = &cobra.Command{
	Annotations: map[string]string{
		govardruntime.AnnotationRequires: string(govardruntime.CapDocker),
	},
	Use:     "desktop",
	Aliases: []string{"gui"},
	Short:   "Launch the Govard Desktop app",
	Run: func(cmd *cobra.Command, args []string) {
		if desktopDev {
			runDesktopDev()
			return
		}
		if err := runDesktopBinary(desktopBackground); err != nil {
			pterm.Error.Printf("Failed to launch Govard Desktop: %v\n", err)
		}
	},
}

func init() {
	desktopCmd.Flags().BoolVar(&desktopDev, "dev", false, "Run the desktop app in Wails dev mode")
	desktopCmd.Flags().BoolVar(&desktopBackground, "background", false, "Enable background mode (start hidden and keep running after window close)")
	desktopCmd.AddCommand(desktopDoctorCmd)
}

var desktopDoctorCmd = &cobra.Command{
	Annotations: map[string]string{
		// The diagnosis is entirely local: the desktop binary, the display
		// environment, WebKitGTK, and the user-namespace sysctl. Requiring
		// Docker here would make the diagnostic unusable on exactly the host
		// that needs it, so it overrides the group's requirement.
		govardruntime.AnnotationRequires: string(govardruntime.CapNone),
	},
	Use:   "doctor",
	Short: "Diagnose issues with the desktop environment",
	Run: func(cmd *cobra.Command, args []string) {
		pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgCyan)).Println("Govard Desktop Doctor")

		// 1. Check for the binary
		binaryPath, err := findDesktopBinary()
		if err != nil {
			pterm.Error.Println("govard-desktop binary not found in PATH or repo.")
		} else {
			pterm.Success.Printf("Found desktop binary at: %s\n", binaryPath)
			// Try running it with --version. A desktop build that predates the
			// flag ignores it and starts the GUI instead, so the probe is
			// bounded: a diagnostic that hangs cannot report anything.
			probeCtx, cancel := context.WithTimeout(context.Background(), desktopVersionProbeTimeout)
			out, err := probeDesktopVersion(probeCtx, binaryPath)
			cancel()
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				pterm.Error.Printf("Version check timed out after %s: %s did not answer --version (old or broken build).\n",
					desktopVersionProbeTimeout, binaryPath)
			case err != nil:
				pterm.Error.Printf("Failed to execute binary for version check: %v\n", err)
			default:
				pterm.Success.Printf("Binary execution check: %s", out)
			}
		}

		// 2. Check for display
		display := os.Getenv("DISPLAY")
		wayland := os.Getenv("WAYLAND_DISPLAY")
		if display == "" && wayland == "" {
			pterm.Warning.Println("Neither DISPLAY nor WAYLAND_DISPLAY environment variables are set. GUI apps may fail to start.")
		} else {
			if wayland != "" {
				pterm.Success.Printf("Wayland display detected: %s\n", wayland)
			}
			if display != "" {
				pterm.Success.Printf("X11 display detected: %s\n", display)
			}
		}

		// 3. System specific checks
		if runtime.GOOS == "linux" {
			// WebKitGTK 4.1
			if ldconfigOut, err := exec.Command("ldconfig", "-p").Output(); err == nil {
				if strings.Contains(string(ldconfigOut), "libwebkit2gtk-4.1") {
					pterm.Success.Println("WebKitGTK 4.1 found in library cache.")
				} else {
					pterm.Error.Println("WebKitGTK 4.1 NOT found in library cache. Run 'sudo apt install libwebkit2gtk-4.1-0'.")
				}
			}

			// Ubuntu 24.04 User Namespace Restriction
			if _, err := os.Stat("/proc/sys/kernel/apparmor_restrict_unprivileged_userns"); err == nil {
				out, err := exec.Command("cat", "/proc/sys/kernel/apparmor_restrict_unprivileged_userns").Output()
				if err == nil && strings.TrimSpace(string(out)) == "1" {
					pterm.Warning.Println("Ubuntu 24.04 restricted user namespaces detected. This often breaks WebKit sandboxing.")
					pterm.Info.Println("Tip: Try running 'echo \"kernel.apparmor_restrict_unprivileged_userns=0\" | sudo tee /etc/sysctl.d/99-apparmor-userns.conf && sudo sysctl --system' if the app crashes on startup.")
				}
			}
		}

		pterm.Println("\nIf the app still fails to start, try running it with 'GDK_BACKEND=x11 govard desktop' to force X11 mode.")
	},
}

func runDesktopDev() {
	desktopDir, err := findDesktopDir()
	if err != nil {
		pterm.Error.Printf("Failed to locate desktop project: %v\n", err)
		return
	}

	if wailsPath, err := findWailsCLI(); err == nil {
		tags := desktopBuildTags(false)
		pterm.Info.Printf("Running Wails dev with tags: %s\n", tags)
		args := []string{"dev", "-tags", tags}
		if desktopBackground {
			args = append(args, "--", desktop.DesktopBackgroundFlag)
		}
		cmd := exec.Command(wailsPath, args...)
		cmd.Dir = desktopDir
		cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := cmd.Run(); err != nil {
			pterm.Error.Printf("Failed to run Wails dev: %v\n", err)
		}
		return
	}

	productionTags := desktopBuildTags(true)
	pterm.Warning.Printf("Wails CLI not found; falling back to `go run -tags %q ./cmd/govard-desktop`.\n", productionTags)
	root, err := desktop.FindRepoRoot()
	if err != nil {
		pterm.Error.Printf("Failed to locate repo root: %v\n", err)
		return
	}
	args := []string{"run", "-tags", productionTags, "./cmd/govard-desktop"}
	if desktopBackground {
		args = append(args, desktop.DesktopBackgroundFlag)
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		pterm.Error.Printf("Failed to run desktop app: %v\n", err)
	}
}

func runDesktopBinary(background bool) error {
	binaryPath, err := findDesktopBinary()
	if err != nil {
		return err
	}
	cmd := exec.Command(binaryPath, buildDesktopBinaryArgs(background)...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

func buildDesktopBinaryArgs(background bool) []string {
	if background {
		return []string{desktop.DesktopBackgroundFlag}
	}
	return []string{}
}

func findDesktopBinary() (string, error) {
	if executablePath, err := desktopExecutablePath(); err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(executablePath); resolveErr == nil {
			executablePath = resolved
		}
		sibling := filepath.Join(filepath.Dir(executablePath), "govard-desktop")
		if runtime.GOOS == "windows" {
			sibling = sibling + ".exe"
		}
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}

	if path, err := desktopBinaryLookPath("govard-desktop"); err == nil {
		return path, nil
	}

	root, err := desktop.FindRepoRoot()
	if err != nil {
		return "", fmt.Errorf("govard-desktop binary not found in PATH. Build it from a Govard source checkout with `go build -tags %q -o bin/govard-desktop ./cmd/govard-desktop`", desktopBuildTags(true))
	}

	candidates := []string{
		filepath.Join(root, "desktop", "build", "bin", "govard-desktop"),
		filepath.Join(root, "bin", "govard-desktop"),
		filepath.Join(root, "govard-desktop"),
	}

	if runtime.GOOS == "windows" {
		candidates = append(candidates, []string{
			filepath.Join(root, "desktop", "build", "bin", "govard-desktop.exe"),
			filepath.Join(root, "bin", "govard-desktop.exe"),
			filepath.Join(root, "govard-desktop.exe"),
		}...)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("govard-desktop binary not found. Build it with `wails build -tags %q` (from %s) or `go build -tags %q -o bin/govard-desktop ./cmd/govard-desktop`", desktopBuildTags(true), filepath.Join(root, "desktop"), desktopBuildTags(true))
}

func findDesktopDir() (string, error) {
	root, err := desktop.FindRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "desktop"), nil
}

func findWailsCLI() (string, error) {
	if path, err := exec.LookPath("wails"); err == nil {
		return path, nil
	}

	candidates := []string{}
	if gobin := strings.TrimSpace(os.Getenv("GOBIN")); gobin != "" {
		candidates = append(candidates, filepath.Join(gobin, "wails"))
	}

	if gopath := strings.TrimSpace(os.Getenv("GOPATH")); gopath != "" {
		for _, base := range filepath.SplitList(gopath) {
			base = strings.TrimSpace(base)
			if base == "" {
				continue
			}
			candidates = append(candidates, filepath.Join(base, "bin", "wails"))
		}
	}

	goPathOut, err := exec.Command("go", "env", "GOPATH").Output()
	if err == nil {
		for _, base := range filepath.SplitList(strings.TrimSpace(string(goPathOut))) {
			base = strings.TrimSpace(base)
			if base == "" {
				continue
			}
			candidates = append(candidates, filepath.Join(base, "bin", "wails"))
		}
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		if stat, err := os.Stat(clean); err == nil && !stat.IsDir() {
			return clean, nil
		}
	}
	return "", fmt.Errorf("wails CLI not found in PATH, GOBIN, or GOPATH/bin")
}
