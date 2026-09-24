//go:build desktop

package desktop

import (
	"io/fs"
	"os"

	"github.com/wailsapp/wails/v2"
)

// Run is the single launch path for every desktop entry point. It lives in its
// own desktop-tagged file because it imports wails/v2, which untagged builds
// must not link.
func Run(assets fs.FS) error {
	app := NewApp()
	launch := ResolveLaunchOptions(os.Args[1:], os.Getenv(DesktopBackgroundEnvVar))
	return wails.Run(BuildWailsOptions(app, assets, launch))
}
