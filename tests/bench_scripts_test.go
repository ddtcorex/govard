package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// benchHarnessTargets are the arms the harness ships. Each needs an executable
// adapter, because run.sh refuses a target it cannot find next to itself.
var benchHarnessTargets = []string{"govard", "ddev", "warden"}

func runBenchRunScript(t *testing.T, args ...string) (string, int) {
	t.Helper()
	command := exec.Command("bash", append([]string{"../scripts/bench/run.sh"}, args...)...)
	out, err := command.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run bench script: %v", err)
	}
	return string(out), exitErr.ExitCode()
}

func TestBenchScriptsParse(t *testing.T) {
	scripts, err := filepath.Glob("../scripts/bench/*.sh")
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Fatal("no bench scripts found; the harness moved without this test")
	}
	for _, script := range scripts {
		out, err := exec.Command("bash", "-n", script).CombinedOutput()
		if err != nil {
			t.Errorf("%s does not parse: %v\n%s", script, err, out)
		}
	}
}

func TestBenchAdaptersAreExecutable(t *testing.T) {
	for _, target := range benchHarnessTargets {
		path := filepath.Join("..", "scripts", "bench", target+".sh")
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("adapter for %s is missing: %v", target, err)
			continue
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("adapter for %s is not executable, so run.sh refuses the target", target)
		}
	}
}

func TestBenchRunRefusesAMissingFixture(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-fixture")

	out, code := runBenchRunScript(t, "--target", "govard", "--fixture", missing, "--runs", "1", "--out", t.TempDir())

	if code == 0 {
		t.Fatalf("a missing fixture must fail the run, output:\n%s", out)
	}
	if !strings.Contains(out, missing) {
		t.Errorf("message must name the fixture path %q, got:\n%s", missing, out)
	}
	if !strings.Contains(out, "fixture.sh") {
		t.Errorf("message must say how to create a fixture, got:\n%s", out)
	}
}

func TestBenchRunRefusesAFixtureWithoutConfig(t *testing.T) {
	empty := t.TempDir()

	out, code := runBenchRunScript(t, "--target", "govard", "--fixture", empty, "--runs", "1", "--out", t.TempDir())

	if code == 0 {
		t.Fatalf("a fixture with no .govard.yml must fail the run, output:\n%s", out)
	}
	if !strings.Contains(out, ".govard.yml") {
		t.Errorf("message must name the missing config, got:\n%s", out)
	}
}

func TestBenchRunRefusesAnUnknownTarget(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, ".govard.yml"), []byte("project_name: bench\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, code := runBenchRunScript(t, "--target", "lando", "--fixture", fixture, "--runs", "1", "--out", t.TempDir())

	if code == 0 {
		t.Fatalf("an unknown target must fail the run, output:\n%s", out)
	}
	if !strings.Contains(out, "no adapter for target lando") {
		t.Errorf("message must name the unknown target, got:\n%s", out)
	}
}
