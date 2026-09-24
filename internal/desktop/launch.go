package desktop

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const (
	DesktopBackgroundFlag       = "--background"
	DesktopBackgroundEnvVar     = "GOVARD_DESKTOP_BACKGROUND"
	desktopSingleInstanceLockID = "govard.desktop.app"
)

type LaunchOptions struct {
	Background bool
}

func ResolveLaunchOptions(args []string, envBackground string) LaunchOptions {
	options := LaunchOptions{}
	for _, arg := range args {
		trimmed := strings.ToLower(strings.TrimSpace(arg))
		if trimmed == "--version" || trimmed == "-v" {
			fmt.Printf("Govard Desktop v%s\n", Version)
			os.Exit(0)
		}
		if trimmed == DesktopBackgroundFlag {
			options.Background = true
		}
	}
	if parseTruthyBool(envBackground) {
		options.Background = true
	}
	return options
}

// CheckAssetRoot fails when index.html is not at the root of assets. The Wails
// v3 window loads /index.html and the asset server re-roots the FS at the
// directory holding index.html, so a nested index is a silent blank window.
// Report it in the log instead.
func CheckAssetRoot(assets fs.FS) error {
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return fmt.Errorf("frontend assets have no index.html at their root (run `make frontend`): %w", err)
	}
	return nil
}

func parseTruthyBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
