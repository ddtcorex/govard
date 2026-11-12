package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/pterm/pterm"

	"govard/internal/cmd"
	"govard/internal/ui"
	"govard/internal/verify"
)

// TestVerifyErrorJSONKeepsGateRefusalOffStdout pins the documented promise of
// --error-json (docs/reference: the envelope is the only thing a script has to
// parse, so it does not have to read human text). verify refuses before phase 1
// runs and, without --json, announces the refusal with a pterm warning, which
// the pterm default writer sends to stdout: the envelope that follows is then
// preceded by a decorated line. The warning belongs on stderr.
func TestVerifyErrorJSONKeepsGateRefusalOffStdout(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_TEST_SATISFIED_CAPABILITIES", "docker")

	stdout := &bytes.Buffer{}
	pterm.SetDefaultOutput(stdout)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	stderr := &bytes.Buffer{}
	root := cmd.RootCommandForTest()
	t.Cleanup(func() { _ = root.PersistentFlags().Set("error-json", "false") })
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.PersistentFlags().Set("error-json", "true"); err != nil {
		t.Fatalf("set --error-json: %v", err)
	}
	root.SetArgs([]string{"verify", "--project", t.TempDir()})

	err := root.Execute()
	if !errors.Is(err, verify.ErrNeedAllowDestructive) {
		t.Fatalf("err = %v, want ErrNeedAllowDestructive", err)
	}

	trimmed := bytes.TrimSpace(stdout.Bytes())
	if len(trimmed) > 0 && !json.Valid(trimmed) {
		t.Fatalf("stdout is not one JSON document:\n%q", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("need --allow-destructive")) {
		t.Fatalf("the refusal notice should be readable on stderr, got:\n%q", stderr.String())
	}
}

// TestMachineModeRoutesHumanOutputToStderr pins the mechanism the CLI uses: in
// machine mode every pterm line (Info/Success/Warning and the printers built from
// them) goes to the stream the caller chose instead of stdout, while stdout keeps
// whatever the command prints as data.
func TestMachineModeRoutesHumanOutputToStderr(t *testing.T) {
	stdout := &bytes.Buffer{}
	pterm.SetDefaultOutput(stdout)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	stderr := &bytes.Buffer{}
	ui.RouteHumanOutputTo(stderr)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	pterm.Info.Println("progress line")
	pterm.Success.Println("done line")

	if stdout.Len() != 0 {
		t.Fatalf("machine mode left human output on stdout: %q", stdout.String())
	}
	for _, want := range []string{"progress line", "done line"} {
		if !bytes.Contains(stderr.Bytes(), []byte(want)) {
			t.Fatalf("stderr is missing %q, got: %q", want, stderr.String())
		}
	}
}

// TestMachineModeLeavesNoWhitespaceOnStdout extends the byte-exact rule to every
// command the audit found decorating stdout with blank lines: the envelope is the
// whole document, so even the newlines that used to frame a banner belong on
// stderr.
func TestMachineModeLeavesNoWhitespaceOnStdout(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"init refuses an unknown framework", []string{"init", "--framework", "nosuch-framework"}},
		{"test finds nothing to run", []string{"test"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			chdirForTest(t, dir)
			writeRuntimeConfig(t, dir, "project_name: blank-demo\ndomain: blank-demo.test\nframework: wordpress\n")

			// Two paths reach stdout: pterm's default writer and fmt.Print*, which
			// goes straight to os.Stdout and ignores cobra's writer.
			stdout := &bytes.Buffer{}
			processStdout, restoreStdout := captureProcessStdout(t)
			t.Cleanup(restoreStdout)
			pterm.SetDefaultOutput(stdout)
			t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

			stderr := &bytes.Buffer{}
			root := cmd.RootCommandForTest()
			t.Cleanup(func() { _ = root.PersistentFlags().Set("error-json", "false") })
			root.SetOut(stdout)
			root.SetErr(stderr)
			if err := root.PersistentFlags().Set("error-json", "true"); err != nil {
				t.Fatalf("set --error-json: %v", err)
			}
			root.SetArgs(append(tc.args, "--error-json"))

			_ = root.Execute()

			written := append(processStdout(), stdout.Bytes()...)
			if len(written) != 0 {
				t.Fatalf("machine mode left %d bytes on stdout: %q", len(written), string(written))
			}
		})
	}
}
