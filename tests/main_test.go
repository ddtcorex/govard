package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pterm/pterm"
)

// chdirForTest changes the working directory to dir for the duration of the
// test, restoring the original directory on cleanup. Shared across many
// _test.go files whose subject functions read/write relative to cwd.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

// disableGitHousekeeping stops the git processes these tests start from doing
// background maintenance of their own.
//
// `git fetch` and `git commit` may run `gc --auto`, which detaches a `git gc`
// that keeps writing into the repository after the command that started it has
// returned. `t.TempDir` cleanup then fails with "directory not empty" while that
// process is still creating pack files — seen in CI on
// TestCoreCheckRefreshesTheSandboxMirror, which refreshes a bare mirror inside a
// temp directory. GIT_CONFIG_* injects the setting into every git process this
// binary starts, including the ones govard runs through its own runner, and it
// applies to repositories created later, which a per-repo `git config` would not.
func disableGitHousekeeping() {
	for key, value := range map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "gc.auto",
		"GIT_CONFIG_VALUE_0": "0",
		"GIT_CONFIG_KEY_1":   "maintenance.auto",
		"GIT_CONFIG_VALUE_1": "false",
	} {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	}
}

func TestMain(m *testing.M) {
	pterm.DisableColor()
	disableGitHousekeeping()
	originalGovardHome, hadGovardHome := os.LookupEnv("GOVARD_HOME_DIR")

	tempGovardHome, err := os.MkdirTemp("", "govard-tests-home-*")
	if err != nil {
		panic(err)
	}

	if err := os.Setenv("GOVARD_HOME_DIR", tempGovardHome); err != nil {
		panic(err)
	}

	code := m.Run()

	_ = os.RemoveAll(tempGovardHome)
	if hadGovardHome {
		_ = os.Setenv("GOVARD_HOME_DIR", originalGovardHome)
	} else {
		_ = os.Unsetenv("GOVARD_HOME_DIR")
	}

	os.Exit(code)
}

// A deploy preflight asks the target `php -r ...` twice (OPcache, version). In a
// test the "target" is this machine, so an unstubbed run executes whatever php
// the developer has: on a host where that is a docker shim, every ask costs a
// container start (0.5s quiet, 10s+ loaded) and the answer depends on the
// machine. hermeticPHP puts a stand-in first on PATH for the one test, so the
// assertions are about the preflight and not about the toolchain.
func TestHermeticPHPStandsInForTheHostPHP(t *testing.T) {
	hermeticPHP(t)
	for args, want := range map[string]string{
		"-r 'echo PHP_VERSION;'":                              "8.3.6",
		`-r 'echo extension_loaded("Zend OPcache") ? 1 : 0;'`: "0",
	} {
		out, err := exec.Command("sh", "-c", "php "+args).Output()
		if err != nil || strings.TrimSpace(string(out)) != want {
			t.Fatalf("php %s = %q, %v; want %q", args, out, err, want)
		}
	}
}

// hermeticPHP installs a `php` that answers the two probes the deploy preflight
// makes and succeeds silently for anything else. It does not run PHP code, so a
// test that needs real PHP semantics must not use it.
func hermeticPHP(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *PHP_VERSION*) printf '8.3.6' ;;\n" +
		"  *extension_loaded*) printf '0' ;;\n" +
		"esac\n"
	if err := os.WriteFile(dir+"/php", []byte(script), 0o755); err != nil {
		t.Fatalf("write php stand-in: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
