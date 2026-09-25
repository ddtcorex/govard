package tests

import (
	"os"
	"strings"
	"testing"
)

// Record mode must never couple itself into main.js or index.html - the loaders
// in services/ are the only seam (the plan's judgment call 4). This also pins
// the env-inheritance fact: runDesktopDev must not give the vite subprocess a
// non-os.Environ()-based Env, which would silently break GOVARD_PREVIEW_RECORD
// passthrough, because a nil exec.Cmd.Env makes the child inherit this process's
// environment.
func TestDesktopMainJSHasNoRecordModeCoupling(t *testing.T) {
	for _, rel := range []string{
		"../desktop/frontend/main.js",
		"../desktop/frontend/index.html",
	} {
		data, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		src := string(data)
		for _, banned := range []string{"GOVARD_PREVIEW_RECORD", "__preview/record", "record-bootstrap"} {
			if strings.Contains(src, banned) {
				t.Errorf("%s must not reference %q; record mode lives entirely in preview/", rel, banned)
			}
		}
	}
}

func TestDesktopRunDesktopDevDoesNotOverrideViteEnv(t *testing.T) {
	data, err := os.ReadFile("../internal/cmd/desktop.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if strings.Contains(src, "vite.Env") {
		t.Error("runDesktopDev must leave vite.Env nil (inherits os.Environ()) so GOVARD_PREVIEW_RECORD reaches the dev server; an explicit vite.Env assignment needs its own os.Environ()-based passthrough")
	}
}

// Record mode always installs the recording loader, whichever candidate
// PREVIEW_SEAM picks for playback. Branching on PREVIEW_SEAM here is the defect
// this pins: with the Phase 0 winner ("transport") that branch can only be a
// no-op, because the real app has no inner transport to chain - its
// customTransport is null and the built-in HTTP transport is not an object.
func TestDesktopRecordBootstrapDoesNotBranchOnPreviewSeam(t *testing.T) {
	src := readRepoFile(t, "desktop/frontend/preview/record-bootstrap.js")
	// Banning the import rather than the identifier keeps this a test about code
	// and not about prose: the file's own comment explains PREVIEW_SEAM.
	if strings.Contains(src, `from "./seam.js"`) {
		t.Error("record-bootstrap.js must install the recording loader unconditionally; importing the playback seam is what makes record mode a no-op under the transport seam")
	}
	for _, required := range []string{"createRecordingLoader", "loadGeneratedModules", "__setBindingsLoaderForTest"} {
		if !strings.Contains(src, required) {
			t.Errorf("record-bootstrap.js must use %s", required)
		}
	}
}
