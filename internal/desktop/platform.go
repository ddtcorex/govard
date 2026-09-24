package desktop

import (
	"fmt"
)

// Platform is the seam between the desktop services and the GUI runtime.
// Services never call the Wails runtime directly; they call a Platform.
type Platform interface {
	Emit(event string, data any)
	OpenURL(url string) error
	ChooseDirectory(title, defaultDir string) (string, error)
	ChooseSaveFile(opts SaveFileOptions) (string, error)
	ShowWindow()
	HideWindow()
	Quit()
}

// SaveFileOptions describes a native save dialog request.
type SaveFileOptions struct {
	Title           string
	DefaultDir      string
	DefaultFilename string
}

// errDesktopNotAvailableFn reports a runtime call from a build that has no GUI
// runtime (the CLI binary and the untagged unit tests). It is a function
// variable so callers cannot accidentally compare the errors by value.
var errDesktopNotAvailableFn = func() error {
	return fmt.Errorf("desktop runtime not available")
}
