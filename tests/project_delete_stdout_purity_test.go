package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pterm/pterm"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// captureProcessStdout redirects os.Stdout for the duration of a test and returns
// what was written, so a direct fmt.Println is observable.
func captureProcessStdout(t *testing.T) (func() []byte, func()) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	collected := make([]byte, 0)
	return func() []byte {
			_ = writer.Close()
			remaining, _ := io.ReadAll(reader)
			collected = append(collected, remaining...)
			return collected
		}, func() {
			os.Stdout = original
			_ = reader.Close()
		}
}

// TestProjectDeleteKeepsStdoutCleanForTheErrorEnvelope pins the contract root.go
// states: stdout carries the machine-readable document, and everything a human is
// meant to read belongs on stderr. `project delete` used to paint its banner and
// its progress lines onto stdout, so a failing run under --error-json put prose in
// front of the JSON envelope and a consumer could not parse the stream.
func TestProjectDeleteKeepsStdoutCleanForTheErrorEnvelope(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "scratch")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create the project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: scratch\nframework: custom\n"), 0o644); err != nil {
		t.Fatalf("write the project config: %v", err)
	}

	// A registry that can be read but not written: the delete paints its banner,
	// starts its cleanup, and then fails on the registry update, which is the
	// failure this contract is about (prose already on the stream).
	registryDir := filepath.Join(root, "govard-home")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("create the govard home: %v", err)
	}
	t.Setenv("GOVARD_HOME_DIR", registryDir)
	registryPath := filepath.Join(registryDir, "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        projectDir,
		ProjectName: "scratch",
		Domain:      "scratch.test",
		Framework:   "custom",
	}); err != nil {
		t.Fatalf("seed the registry: %v", err)
	}

	// The command paints through two paths: pterm (its captured default writer) and
	// fmt.Print*, which goes straight to os.Stdout. Capture both, because the
	// contract is about the process's stdout, not about cobra's writer.
	stdout := &bytes.Buffer{}
	processStdout, restoreStdout := captureProcessStdout(t)
	t.Cleanup(restoreStdout)
	pterm.SetDefaultOutput(stdout)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	stderr := &bytes.Buffer{}
	rootCmd := cmd.RootCommandForTest()
	t.Cleanup(func() { _ = rootCmd.PersistentFlags().Set("error-json", "false") })
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	_ = rootCmd.PersistentFlags().Set("error-json", "true")
	rootCmd.SetArgs([]string{"project", "delete", "scratch", "--force"})

	// Make the registry unwritable only after it was seeded.
	if err := os.Chmod(registryDir, 0o500); err != nil {
		t.Skipf("cannot make the registry read-only here: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(registryDir, 0o700) })

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected the delete to fail on the unwritable registry")
	}

	// Whatever the command printed, stdout must be parseable as JSON or empty: the
	// envelope that follows is the only document a consumer should have to read.
	writtenToStdout := append(append([]byte{}, processStdout()...), stdout.Bytes()...)
	trimmed := bytes.TrimSpace(writtenToStdout)
	if len(trimmed) > 0 && !json.Valid(trimmed) {
		t.Fatalf("stdout carries human output ahead of the envelope:\n%q", string(writtenToStdout))
	}
	if bytes.Contains(writtenToStdout, []byte("DELETING PROJECT")) {
		t.Fatalf("the banner belongs on stderr, got it on stdout:\n%q", string(writtenToStdout))
	}
	if !bytes.Contains(stderr.Bytes(), []byte("DELETING PROJECT")) {
		t.Fatalf("the banner should have moved to stderr, got:\n%q", stderr.String())
	}
}
