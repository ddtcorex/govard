package tests

import (
	"runtime"
	"testing"
	"time"

	"govard/internal/desktop"
)

func TestDesktopLaunchOptionsEnableBackgroundFromFlag(t *testing.T) {
	options := desktop.ResolveLaunchOptionsForTest([]string{"--background"}, "")
	if !options.Background {
		t.Fatalf("expected background mode enabled from --background flag")
	}
}

func TestDesktopLaunchOptionsEnableBackgroundFromEnv(t *testing.T) {
	options := desktop.ResolveLaunchOptionsForTest(nil, "yes")
	if !options.Background {
		t.Fatalf("expected background mode enabled from env")
	}
}

func TestDesktopLaunchOptionsDisableBackgroundByDefault(t *testing.T) {
	options := desktop.ResolveLaunchOptionsForTest(nil, "")
	if options.Background {
		t.Fatalf("expected background mode disabled by default")
	}
}

func TestDesktopDecideClose(t *testing.T) {
	cases := []struct {
		name            string
		runInBackground bool
		trayAvailable   bool
		want            desktop.CloseAction
	}{
		{"background on, tray present: hide", true, true, desktop.CloseHide},
		{"background on, no tray: quit, never trap the window", true, false, desktop.CloseQuit},
		{"background off, tray present: quit", false, true, desktop.CloseQuit},
		{"background off, no tray: quit", false, false, desktop.CloseQuit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := desktop.DecideCloseForTest(tc.runInBackground, tc.trayAvailable); got != tc.want {
				t.Fatalf("decideClose(%v, %v) = %v, want %v", tc.runInBackground, tc.trayAvailable, got, tc.want)
			}
		})
	}
}

func TestDesktopCloseGateNeverCancelsAnExplicitQuit(t *testing.T) {
	gate := desktop.NewCloseGateForTest()
	if !gate.ShouldCancelClose(true, true) {
		t.Fatal("window close with background on and a tray should be cancelled (hidden)")
	}
	gate.MarkQuitting()
	if gate.ShouldCancelClose(true, true) {
		t.Fatal("after MarkQuitting, closing must never be cancelled")
	}
}

func TestDesktopTrayHostAvailableFalseWithoutSessionBus(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/govard-test-bus")
	done := make(chan bool, 1)
	go func() { done <- desktop.TrayHostAvailable() }()
	select {
	case got := <-done:
		if runtime.GOOS == "linux" && got {
			t.Fatal("TrayHostAvailable() = true with no session bus")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TrayHostAvailable() hung without a session bus")
	}
}

func TestDesktopBuildTrayProjectsSortsAndMarksRunning(t *testing.T) {
	d := desktop.Dashboard{Environments: []desktop.Environment{
		{Project: "zeta", Name: "Zeta", Domain: "zeta.test", Status: "stopped"},
		{Project: "sample-project", Name: "Sample", Domain: "sample-project.test", Status: "running"},
		{Project: "syncing-one", Name: "Syncing", Domain: "syncing-one.test", Status: "syncing"},
	}}
	got := desktop.BuildTrayProjectsForTest(d)
	if len(got) != 3 || got[0].Project != "sample-project" || got[1].Project != "syncing-one" || got[2].Project != "zeta" {
		t.Fatalf("order = %#v", got)
	}
	if !got[0].Running || !got[1].Running || got[2].Running {
		t.Fatalf("running flags = %v %v %v", got[0].Running, got[1].Running, got[2].Running)
	}
	if got[0].URL != "https://sample-project.test" {
		t.Fatalf("URL = %q", got[0].URL)
	}
}
