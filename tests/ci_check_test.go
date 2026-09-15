package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
)

// ciProject writes a minimal project the config loader accepts (ValidateConfig
// needs project_name, domain, and a remote host + user; the render only reads
// the branch) and moves the test into it, because the loader resolves
// .govard.yml from the working directory.
func ciProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".govard.yml"), `project_name: demo
framework: magento2
domain: demo.test
remotes:
  dev1:
    host: example.test
    user: deployer
    path: /home/dev/public_html
    branch: develop
`)
	t.Chdir(dir)
	return dir
}

// runCIGenerateCode runs `govard ci generate` and returns combined output plus
// the process-style exit code (0 ok, 1 execution, 2 usage, 4 configuration).
func runCIGenerateCode(t *testing.T, args ...string) (string, int) {
	t.Helper()
	out := &bytes.Buffer{}
	root := cmd.RootCommandForTest()
	root.SetArgs(append([]string{"ci", "generate"}, args...))
	root.SetOut(out)
	root.SetErr(io.Discard)
	t.Cleanup(func() {
		root.SetArgs(nil)
		flags := cmd.CIGenerateCommand().Flags()
		_ = flags.Set("provider", "")
		_ = flags.Set("output", "")
		_ = flags.Set("check", "false")
	})
	err := root.Execute()
	if err == nil {
		return out.String(), 0
	}
	return out.String() + err.Error(), cli.Code(err)
}

func TestCICheckDetectsDrift(t *testing.T) {
	dir := ciProject(t)
	outPath := filepath.Join(dir, ".gitlab-ci.yml")
	if err := os.WriteFile(outPath, []byte("stale: true\n"), 0644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	out, code := runCIGenerateCode(t, "--provider", "gitlab", "--check", "--output", outPath)
	if code == 0 {
		t.Fatalf("expected non-zero exit on drifted file, got output %q", out)
	}
	if !strings.Contains(out, "drift") {
		t.Fatalf("drift report should say what happened, got %q", out)
	}
}

func TestCIGenerateWritesOutput(t *testing.T) {
	dir := ciProject(t)
	outPath := filepath.Join(dir, ".gitlab-ci.yml")
	out, code := runCIGenerateCode(t, "--provider", "gitlab", "--output", outPath)
	if code != 0 {
		t.Fatalf("generate --output: exit %d\n%s", code, out)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if !strings.Contains(string(data), "stages: [integrity, lint, ship, rollback]") {
		t.Fatalf("generated file missing stages header\n---\n%s", data)
	}
	// The fixture is a Magento project without explicit lint keys: the
	// recipe defaults must still reach the render through the command layer.
	for _, want := range []string{"--standard=Magento2", "app/code app/design"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("generated file missing recipe default %q\n---\n%s", want, data)
		}
	}
	// A file the generator just wrote must pass its own drift check: a
	// mismatch here is a generator bug, never a reason to hand-edit the YAML.
	if out, code := runCIGenerateCode(t, "--provider", "gitlab", "--check", "--output", outPath); code != 0 {
		t.Fatalf("freshly generated file fails --check: exit %d\n%s", code, out)
	}
}
