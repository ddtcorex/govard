package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pterm/pterm"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/verify"
)

// TestVerifyJSONStdoutStaysMachineParseableWhenAnItemFails pins the machine
// contract: `--json` must leave the process's stdout as exactly one JSON object.
//
// The test emulates the real file descriptors by pointing pterm's default writer
// and the command's stdout at the SAME buffer — in production both are fd 1, so
// a verdict line printed through pterm lands in the middle of the JSON. Routing
// pterm somewhere else here would make this test pass while the CLI stays broken.
func TestVerifyJSONStdoutStaysMachineParseableWhenAnItemFails(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	project := t.TempDir()

	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		if len(args) > 0 && args[0] == "lock" {
			return verify.Evidence{ExitCode: 1, OutputExcerpt: "WARNING Lockfile mismatch"}, true
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	pterm.SetDefaultOutput(stdout)
	// os.Stdout, not nil: pterm's default writer is initialised to os.Stdout
	// (print.go:13) and no API restores it, so a nil here is a landmine — the next
	// render in this package writes to a nil io.Writer and panics in fmt.Fprint.
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	root := cmd.RootCommandForTest()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"verify", "--phase", "1", "--json", "--project", project})

	err := root.Execute()
	if !errors.Is(err, cmd.ErrItemsFailed) {
		t.Fatalf("Execute() = %v, want ErrItemsFailed", err)
	}

	var parsed map[string]any
	if e := json.Unmarshal(stdout.Bytes(), &parsed); e != nil {
		t.Fatalf("stdout is not a single JSON object: %v\nstdout=%q", e, stdout.String())
	}
	if parsed["status"] != "failed" {
		t.Fatalf("status = %v, want \"failed\"", parsed["status"])
	}
	if !strings.Contains(stderr.String(), "checklist failed") {
		t.Fatalf("the human verdict must go to stderr; stderr=%q", stderr.String())
	}
}
