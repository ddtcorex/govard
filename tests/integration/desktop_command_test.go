//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
)

func TestDesktopCommandRuntimePaths(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "desktop-m2")

	t.Run("DesktopLaunchesBinaryFromPATH", func(t *testing.T) {
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		installRuntimeCommandShim(t, shim, "govard-desktop", 0)

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "desktop")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "govard-desktop|")
	})

	t.Run("DesktopLaunchesBinaryWithBackgroundFlag", func(t *testing.T) {
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		installRuntimeCommandShim(t, shim, "govard-desktop", 0)

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "desktop", "--background")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		assertContains(t, logs, "govard-desktop|--background")
	})

	// The dev loop is Vite plus a plain `go run`: no Wails CLI is involved, and
	// the app finds the dev server through FRONTEND_DEVSERVER_URL.
	t.Run("DesktopDevRunsViteAndGoRun", func(t *testing.T) {
		shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
		installRuntimeCommandShim(t, shim, "pnpm", 0)
		installRuntimeCommandShim(t, shim, "go", 0)

		result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "desktop", "--dev")
		result.AssertSuccess(t)

		logs := shim.ReadLog(t)
		if !strings.Contains(logs, "pnpm|dev") {
			t.Fatalf("expected 'pnpm|dev' in logs, got: %s\n\nstdout: %s\nstderr: %s", logs, result.Stdout, result.Stderr)
		}
		if !strings.Contains(logs, "go|run -tags desktop ./cmd/govard-desktop") {
			t.Fatalf("expected the go run invocation in logs, got: %s\n\nstdout: %s\nstderr: %s", logs, result.Stdout, result.Stderr)
		}
		if !strings.Contains(result.Stdout, "FRONTEND_DEVSERVER_URL=http://localhost:5173") {
			t.Fatalf("expected FRONTEND_DEVSERVER_URL in stdout, got: %s", result.Stdout)
		}
	})
}
