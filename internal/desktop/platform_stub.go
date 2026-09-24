//go:build !desktop

package desktop

// stubPlatform backs non-desktop builds (the CLI binary and unit tests).
type stubPlatform struct{}

func newDefaultPlatform() Platform { return stubPlatform{} }

func (stubPlatform) Emit(string, any) {}

func (stubPlatform) OpenURL(string) error { return errDesktopNotAvailableFn() }

func (stubPlatform) ChooseDirectory(string, string) (string, error) {
	return "", errDesktopNotAvailableFn()
}

func (stubPlatform) ChooseSaveFile(SaveFileOptions) (string, error) {
	return "", errDesktopNotAvailableFn()
}

func (stubPlatform) ShowWindow() {}
func (stubPlatform) HideWindow() {}
func (stubPlatform) Quit()       {}
