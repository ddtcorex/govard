package tests

import (
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopFakePlatformRecordsCalls(t *testing.T) {
	fake := &desktop.FakePlatform{DirectoryResult: "/tmp/sample-project", SaveFileResult: "/tmp/out.log"}

	fake.Emit("logs:line", "hello")
	if err := fake.OpenURL("https://sample-project.test"); err != nil {
		t.Fatalf("OpenURL: %v", err)
	}
	dir, err := fake.ChooseDirectory("Pick", "/home")
	if err != nil || dir != "/tmp/sample-project" {
		t.Fatalf("ChooseDirectory = %q, %v", dir, err)
	}
	path, err := fake.ChooseSaveFile(desktop.SaveFileOptions{Title: "Save Logs", DefaultFilename: "govard-logs.log"})
	if err != nil || path != "/tmp/out.log" {
		t.Fatalf("ChooseSaveFile = %q, %v", path, err)
	}
	fake.ShowWindow()
	fake.HideWindow()
	fake.Quit()

	if got := fake.EventsNamed("logs:line"); len(got) != 1 || got[0] != "hello" {
		t.Fatalf("EventsNamed = %#v", got)
	}
	if urls := fake.OpenedURLs(); len(urls) != 1 || urls[0] != "https://sample-project.test" {
		t.Fatalf("OpenedURLs = %#v", urls)
	}
	if reqs := fake.SaveFileRequests(); len(reqs) != 1 || reqs[0].Title != "Save Logs" {
		t.Fatalf("SaveFileRequests = %#v", reqs)
	}
	if fake.ShowCount() != 1 || fake.HideCount() != 1 || fake.QuitCount() != 1 {
		t.Fatalf("window counters show=%d hide=%d quit=%d", fake.ShowCount(), fake.HideCount(), fake.QuitCount())
	}
}

func TestDesktopNewAppWithPlatformWiresEveryService(t *testing.T) {
	fake := &desktop.FakePlatform{}
	app := desktop.NewApp(desktop.WithPlatform(fake))

	platforms := desktop.AppPlatformsForTest(app)
	if len(platforms) != 8 { // App + 7 services
		t.Fatalf("expected 8 platform holders, got %d", len(platforms))
	}
	for i, p := range platforms {
		if p != desktop.Platform(fake) {
			t.Fatalf("holder %d does not use the injected platform", i)
		}
	}
}

func TestDesktopDefaultPlatformIsStubOutsideDesktopBuild(t *testing.T) {
	p := desktop.DefaultPlatformForTest()
	p.Emit("logs:line", "ignored") // must not panic
	p.ShowWindow()
	p.HideWindow()
	p.Quit()
	if err := p.OpenURL("https://sample-project.test"); err == nil || !strings.Contains(err.Error(), "desktop runtime not available") {
		t.Fatalf("OpenURL error = %v", err)
	}
	if _, err := p.ChooseDirectory("Pick", ""); err == nil {
		t.Fatal("ChooseDirectory: expected error from stub")
	}
	if _, err := p.ChooseSaveFile(desktop.SaveFileOptions{}); err == nil {
		t.Fatal("ChooseSaveFile: expected error from stub")
	}
}

func TestDesktopNewAppWithoutOptionsUsesDefaultPlatform(t *testing.T) {
	app := desktop.NewApp()
	for i, p := range desktop.AppPlatformsForTest(app) {
		if p == nil {
			t.Fatalf("holder %d has a nil platform", i)
		}
	}
}

func TestDesktopPickProjectDirectoryUsesPlatform(t *testing.T) {
	fake := &desktop.FakePlatform{DirectoryResult: "/tmp/sample-project/"}
	app := desktop.NewApp(desktop.WithPlatform(fake))

	path, err := app.Onboarding.PickProjectDirectory()
	if err != nil {
		t.Fatalf("PickProjectDirectory: %v", err)
	}
	if path != "/tmp/sample-project" {
		t.Fatalf("path = %q, want cleaned /tmp/sample-project", path)
	}
}

func TestDesktopPickProjectDirectoryCancelledReturnsEmpty(t *testing.T) {
	fake := &desktop.FakePlatform{DirectoryResult: "   "}
	app := desktop.NewApp(desktop.WithPlatform(fake))

	path, err := app.Onboarding.PickProjectDirectory()
	if err != nil || path != "" {
		t.Fatalf("PickProjectDirectory = %q, %v; want empty, nil", path, err)
	}
}

func TestDesktopQuitDesktopAppUsesPlatform(t *testing.T) {
	fake := &desktop.FakePlatform{}
	app := desktop.NewApp(desktop.WithPlatform(fake))

	app.Quit()

	if fake.QuitCount() != 1 {
		t.Fatalf("QuitCount = %d, want 1", fake.QuitCount())
	}
}
