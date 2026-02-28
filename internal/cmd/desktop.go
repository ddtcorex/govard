package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"govard/internal/desktop"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var desktopDev bool
var desktopBackground bool

var desktopCmd = &cobra.Command{
	Use:   "desktop",
	Short: "Launch the Govard Desktop app",
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
}

func runDesktopDev() {
	desktopDir, err := findDesktopDir()
	if err != nil {
		pterm.Error.Printf("Failed to locate desktop project: %v\n", err)
		return
	}

	if wailsPath, err := findWailsCLI(); err == nil {
		args := []string{"dev", "-tags", "desktop"}
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

	pterm.Warning.Println("Wails CLI not found; falling back to `go run -tags desktop ./cmd/govard-desktop`.")
	root, err := desktop.FindRepoRoot()
	if err != nil {
		pterm.Error.Printf("Failed to locate repo root: %v\n", err)
		return
	}
	args := []string{"run", "-tags", "desktop", "./cmd/govard-desktop"}
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
	if path, err := exec.LookPath("govard-desktop"); err == nil {
		return path, nil
	}

	root, err := desktop.FindRepoRoot()
	if err != nil {
		return "", fmt.Errorf("govard-desktop binary not found in PATH. Build it from a Govard source checkout with `go build -tags desktop -o bin/govard-desktop ./cmd/govard-desktop`")
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

	return "", fmt.Errorf("govard-desktop binary not found. Build it with `wails build -tags desktop` (from %s) or `go build -tags desktop -o bin/govard-desktop ./cmd/govard-desktop`", filepath.Join(root, "desktop"))
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
