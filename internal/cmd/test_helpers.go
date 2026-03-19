package cmd

import "os/exec"

// FindWailsCLIForTest exposes Wails binary discovery for external tests.
func FindWailsCLIForTest() (string, error) {
	return findWailsCLI()
}

// DesktopBinaryArgsForTest exposes govard-desktop argument construction for tests.
func DesktopBinaryArgsForTest(background bool) []string {
	return buildDesktopBinaryArgs(background)
}

// FindDesktopBinaryForTest exposes desktop binary discovery for external tests.
func FindDesktopBinaryForTest() (string, error) {
	return findDesktopBinary()
}

// SetDesktopExecutablePathForTest overrides os.Executable usage for tests.
func SetDesktopExecutablePathForTest(fn func() (string, error)) func() {
	previous := desktopExecutablePath
	desktopExecutablePath = fn
	return func() {
		desktopExecutablePath = previous
	}
}

// SetDesktopLookPathForTest overrides exec.LookPath usage for tests.
func SetDesktopLookPathForTest(fn func(file string) (string, error)) func() {
	previous := desktopBinaryLookPath
	desktopBinaryLookPath = fn
	return func() {
		desktopBinaryLookPath = previous
	}
}

// ExecLookPathForTest delegates to exec.LookPath for test stubs.
func ExecLookPathForTest(file string) (string, error) {
	return exec.LookPath(file)
}

// DesktopProductionBuildTagsForTest exposes desktop production build tags.
func DesktopProductionBuildTagsForTest() string {
	return desktopBuildTags(true)
}
