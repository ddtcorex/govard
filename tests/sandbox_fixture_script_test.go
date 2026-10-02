package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGovard stands in for the binary: it records its argv and writes the files
// a real init and bootstrap would leave, so the script's git step has something
// to commit. .govard.yml is gitignored in real projects; the fake reproduces that
// so the test proves the script force-adds it.
func fakeGovard(t *testing.T) (binary, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls.log")
	binary = filepath.Join(dir, "govard")
	script := `#!/usr/bin/env bash
echo "$*" >> "` + log + `"
case "$1" in
  init) printf 'project_name: fixture\nframework: magento2\n' > .govard.yml; printf '.govard.yml\n' > .gitignore ;;
  bootstrap) printf '{}' > composer.json; printf '{}' > composer.lock; mkdir -p app/etc; printf '<?php return [];' > app/etc/config.php ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, log
}

func runFixtureScript(t *testing.T, binary string, args ...string) (string, int) {
	t.Helper()
	command := exec.Command("bash", append([]string{"../scripts/sandbox-fixture.sh"}, args...)...)
	command.Env = append(os.Environ(), "GOVARD_BIN="+binary)
	out, err := command.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run script: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return string(out), code
}

func TestFixtureScriptRefusesANonEmptyDirectory(t *testing.T) {
	binary, log := fakeGovard(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runFixtureScript(t, binary, dir)
	if code != 2 || !strings.Contains(out, "not empty") {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a refused run must not call govard at all")
	}
}

func TestFixtureScriptNeedsADirectory(t *testing.T) {
	binary, _ := fakeGovard(t)
	out, code := runFixtureScript(t, binary)
	if code != 2 || !strings.Contains(out, "usage") {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
}

func TestFixtureScriptRunsInitBootstrapAndCommits(t *testing.T) {
	binary, log := fakeGovard(t)
	dir := filepath.Join(t.TempDir(), "fixture")
	out, code := runFixtureScript(t, binary, dir, "--framework-version", "2.4.6")
	if code != 0 {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	calls, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	if len(lines) != 2 ||
		lines[0] != "init --framework magento2 --framework-version 2.4.6 -y" ||
		lines[1] != "bootstrap --fresh -y" {
		t.Fatalf("govard calls = %q", lines)
	}
	tracked, err := exec.Command("git", "-C", dir, "ls-files").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".govard.yml", "composer.json", "composer.lock", "app/etc/config.php"} {
		if !strings.Contains(string(tracked), want) {
			t.Errorf("%s must be committed (a gitignored .govard.yml included): %s", want, tracked)
		}
	}
	count, _ := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").Output()
	if strings.TrimSpace(string(count)) != "1" {
		t.Fatalf("want exactly one commit, got %s", count)
	}
}

func TestFixtureScriptPrintsTheReuseCommands(t *testing.T) {
	binary, _ := fakeGovard(t)
	dir := filepath.Join(t.TempDir(), "fixture")
	out, code := runFixtureScript(t, binary, dir)
	if code != 0 {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
	for _, want := range []string{"env start", "sandbox up --profile full", "deploy build sandbox", "snapshot"} {
		if !strings.Contains(out, want) {
			t.Errorf("the reuse hints must mention %q:\n%s", want, out)
		}
	}
}
