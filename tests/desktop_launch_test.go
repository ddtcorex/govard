package tests

import (
	"strings"
	"testing"
	"testing/fstest"

	"govard/internal/desktop"
)

func TestDesktopCheckAssetRootAcceptsIndexAtRoot(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	if err := desktop.CheckAssetRoot(assets); err != nil {
		t.Fatalf("CheckAssetRoot: %v", err)
	}
}

func TestDesktopCheckAssetRootRejectsNestedIndex(t *testing.T) {
	// The v3 asset server re-roots at the directory holding index.html, so a
	// nested index is served at /index.html only if nothing else is expected
	// at the root; an empty root means a blank window. Fail loudly instead.
	assets := fstest.MapFS{"dist/index.html": {Data: []byte("<html></html>")}}
	err := desktop.CheckAssetRoot(assets)
	if err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("CheckAssetRoot error = %v, want one naming index.html", err)
	}
}

func TestDesktopServicesHaveLifecycleContextBeforeStartup(t *testing.T) {
	app := desktop.NewApp(desktop.WithPlatform(&desktop.FakePlatform{}))
	for name, ctx := range desktop.ServiceContextsForTest(app) {
		if ctx == nil {
			t.Fatalf("%s lifecycle context is nil before startup", name)
		}
	}
}
