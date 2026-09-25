package tests

import (
	"strings"
	"testing"
)

// The dev loop must observe the root command's context: a SIGINT/SIGTERM
// delivered to govard alone cancels it, and only the grouped start forwards that
// to the whole app process group (issue #423). A bare app.Run() absorbs the
// cancel and leaves `go run`, the window and Vite alive.
func TestDesktopDevAppIsStartedWithTheCommandContext(t *testing.T) {
	src := readRepoFile(t, "internal/cmd/desktop.go")
	if strings.Contains(src, "app.Run()") {
		t.Error("runDesktopDev must start the app with startDevGrouped(ctx, app), not app.Run()")
	}
	if !strings.Contains(src, "startDevGrouped(ctx, app)") {
		t.Error("runDesktopDev must start the app with startDevGrouped(ctx, app)")
	}
	if !strings.Contains(src, "runDesktopDev(cmd.Context())") {
		t.Error("the desktop dev command must pass its context into runDesktopDev")
	}
}
