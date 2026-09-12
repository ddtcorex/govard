package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func writableContext(t *testing.T, settings map[string]any) (*deploy.StepContext, string) {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("1", "abcdef", "main")
	release.Path = host.ReleasePath("1")
	target := filepath.Join(release.Path, "var")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("prepare writable dir: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:         "local",
		CommandTimeout: 0,
		Settings:       settings,
	})
	sc.Release = release
	sc.Runner = captureRunner{base: deploy.LocalRunner{}, seen: new([]string)}
	return sc, target
}

func TestWritableAppliesChmodByDefault(t *testing.T) {
	sc, target := writableContext(t, map[string]any{"writable_dirs": []string{"var"}})

	if err := deploy.CoreWritable(context.Background(), sc); err != nil {
		t.Fatalf("writable: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o775 {
		t.Fatalf("mode = %v, want 0775", info.Mode().Perm())
	}
}

func TestWritableSkipDoesNothing(t *testing.T) {
	sc, _ := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "skip",
	})

	if err := deploy.CoreWritable(context.Background(), sc); err != nil {
		t.Fatalf("writable skip: %v", err)
	}
	// A skip that still ran commands would be a lie in the plan output.
	if commands := *sc.Runner.(captureRunner).seen; len(commands) != 0 {
		t.Fatalf("writable_mode=skip ran %v", commands)
	}
}

func TestWritableChownNeedsAConfiguredOwner(t *testing.T) {
	sc, _ := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "chmod+chown",
	})

	err := deploy.CoreWritable(context.Background(), sc)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("err = %v, want a refusal naming settings.owner", err)
	}
}

func TestWritableRejectsAnUnknownMode(t *testing.T) {
	sc, _ := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "teleport",
	})

	if err := deploy.CoreWritable(context.Background(), sc); err == nil {
		t.Fatal("want a refusal for an unknown writable_mode")
	}
}

// `writable_mode: acl` is the mode the reference deploy tool defaults to, and the
// one a project migrating from it reaches for: permissions that inherit onto the
// files the web server creates later, without a `chown -R` over the whole release.
// Executed with the real `setfacl`/`getfacl`: the property is the ACL on the tree,
// not the text of the command that sets it.
func TestWritableAppliesACLWhenConfigured(t *testing.T) {
	if _, err := exec.LookPath("setfacl"); err != nil {
		t.Skip("setfacl is not installed")
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		t.Skipf("no current user: %v", err)
	}

	sc, target := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "acl",
		"owner":         current.Username,
	})
	if err := deploy.CoreWritable(context.Background(), sc); err != nil {
		t.Fatalf("writable acl: %v", err)
	}

	assertFaclHasEntry(t, target, "user:"+current.Username+":")

	// The default ACL is the half that makes the mode worth having: a file created
	// after the deploy inherits the entry, so the web server can write what it
	// generates without another chown.
	inherited := filepath.Join(target, "cache-file.txt")
	if err := os.WriteFile(inherited, []byte("generated after the deploy"), 0o644); err != nil {
		t.Fatalf("create a file under the ACL: %v", err)
	}
	assertFaclHasEntry(t, inherited, "user:"+current.Username+":")
}

// An ACL without a subject is meaningless, so the mode needs `owner` exactly like
// the chown modes do.
func TestWritableACLNeedsAConfiguredOwner(t *testing.T) {
	sc, _ := writableContext(t, map[string]any{
		"writable_dirs": []string{"var"},
		"writable_mode": "acl",
	})

	err := deploy.CoreWritable(context.Background(), sc)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("err = %v, want a refusal naming settings.owner", err)
	}
}

// A target without setfacl must be refused before the release is built, not at the
// writable step after it exists.
func TestCoreCheckProbesForSetfaclWhenTheModeIsACL(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	seen := new([]string)
	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:         "local",
		CommandTimeout: 0,
		Settings:       map[string]any{"writable_mode": "acl", "owner": "www-data"},
	})
	sc.Runner = captureRunner{base: deploy.LocalRunner{}, seen: seen}
	sc.Release = deploy.NewReleaseForTest("1", "abcdef", "main")

	if err := deploy.CoreCheck(context.Background(), sc); err != nil {
		t.Fatalf("check: %v", err)
	}
	joined := strings.Join(*seen, "\n")
	if !strings.Contains(joined, "setfacl") {
		t.Fatalf("the preflight must probe for setfacl:\n%s", joined)
	}

	// And the probe fails the preflight when the binary is not there.
	t.Setenv("PATH", t.TempDir())
	if err := deploy.CheckWritableModeForTest(context.Background(), sc); err == nil {
		t.Fatal("a target without setfacl must be refused")
	}
}

func assertFaclHasEntry(t *testing.T, path, want string) {
	t.Helper()
	out, err := exec.Command("getfacl", "-p", path).CombinedOutput()
	if err != nil {
		t.Fatalf("getfacl %s: %v\n%s", path, err, out)
	}
	if !strings.Contains(string(out), want) {
		t.Fatalf("getfacl %s = %q, want an entry %q", path, string(out), want)
	}
}

// A configuration typo is a *configuration* error: it is refused while the settings
// are validated (exit 4), before a plan exists, rather than by the step that would
// hit it after the release directory was built.
func TestUnknownWritableModeIsAConfigurationError(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	err := deploy.ValidateSettings(recipe, map[string]any{"writable_mode": "teleport"})
	if !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
	}
	for _, want := range []string{"teleport", "acl"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to name %q", err, want)
		}
	}

	// Every mode the writable step implements is accepted, including the empty
	// value that means the default.
	for _, mode := range []string{"", "chmod", "chown", "chmod+chown", "acl", "skip"} {
		if err := deploy.ValidateSettings(recipe, map[string]any{"writable_mode": mode}); err != nil {
			t.Fatalf("writable_mode %q: %v", mode, err)
		}
	}
}
