package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestAbsolutizeGatewayMountRewritesToEffectiveHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	content := "      - ../gateway:/govard-gateway:ro\n"
	first := string(engine.AbsolutizeGatewayMountForTest([]byte(content)))
	want := filepath.Join(home, "gateway") + ":/govard-gateway:ro"
	if !strings.Contains(first, want) {
		t.Fatalf("rewritten mount = %q, want it to contain %q", first, want)
	}
	if strings.Contains(first, "../gateway") {
		t.Fatalf("rewritten mount = %q, want no relative marker left", first)
	}

	// The rewrite is stable: the same input renders the same output, and
	// rendering the output again is a no-op (the marker is gone).
	second := string(engine.AbsolutizeGatewayMountForTest([]byte(content)))
	if second != first {
		t.Fatalf("second render = %q, want %q", second, first)
	}
	third := string(engine.AbsolutizeGatewayMountForTest([]byte(first)))
	if third != first {
		t.Fatalf("re-render of output = %q, want %q (idempotent)", third, first)
	}
}

func TestAbsolutizeGatewayMountLeavesUnmarkedContentAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	content := "      - ../registry:/govard-registry:ro\n"
	got := string(engine.AbsolutizeGatewayMountForTest([]byte(content)))
	if got != content {
		t.Fatalf("unmarked content = %q, want unchanged %q", got, content)
	}
}

func TestEnsureGatewayMountDirCreatesTheDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	if err := engine.EnsureGatewayMountDirForTest(); err != nil {
		t.Fatalf("EnsureGatewayMountDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(home, "gateway"))
	if err != nil {
		t.Fatalf("stat gateway dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("gateway path exists but is not a directory")
	}
}

// TestEnsureGatewayMountDirFailsLoudly pins that an uncreatable gateway
// directory surfaces as an error instead of a silent skip: a mount that
// cannot be pre-created will fail compose anyway, so the render must say so
// now. The blocker is a regular file where the parent must be (ENOTDIR),
// which fails for every uid -- a chmod-based read-only parent would not stop
// root.
func TestEnsureGatewayMountDirFailsLoudly(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Setenv("GOVARD_HOME_DIR", filepath.Join(blocker, "govard"))

	err := engine.EnsureGatewayMountDirForTest()
	if err == nil {
		t.Fatal("expected an error when the gateway directory cannot be created")
	}
	if !strings.Contains(err.Error(), "create the SSH gateway directory") {
		t.Fatalf("error %q does not name the failing step", err.Error())
	}
}
