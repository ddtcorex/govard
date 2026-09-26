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

// DesktopApplicationID is the GTK application id of the desktop app, and — with
// the ".desktop" suffix — the name of the launcher the deb installs.
//
// On Wayland this id alone decides which launcher the window belongs to: GNOME
// Shell takes the GTK application id off the surface and looks up the desktop
// file id "<application id>.desktop" (gnome-shell src/shell-window-tracker.c,
// get_app_from_id). Wails derives "org.wails.<sanitised Options.Name>" whenever
// LinuxOptions leaves ApplicationID empty, that id matches no installed desktop
// file, and the window becomes a window-backed app: a second, differently named
// icon appears next to the pinned launcher. The value must also satisfy
// g_application_id_is_valid() — two or more '.'-separated elements of
// [A-Za-z0-9_-], none starting with a digit — or GTK refuses it and Wails falls
// back to the derived id again.
const DesktopApplicationID = "io.github.ddtcorex.govard"

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
