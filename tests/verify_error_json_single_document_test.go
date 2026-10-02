package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/pterm/pterm"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/verify"
)

// verifyStdoutDocuments runs `verify --json --error-json` and returns how many
// JSON documents the command wrote to stdout and the error it returned.
// main appends the --error-json envelope itself unless the error says it was
// already reported, so documents-on-stdout plus that envelope is what a
// consumer actually sees; the contract is exactly one.
func verifyStdoutDocuments(t *testing.T, project string, extraArgs ...string) (docs int, total int, err error) {
	t.Helper()
	fakeProbeHTTP(t)
	stdout := &bytes.Buffer{}
	pterm.SetDefaultOutput(stdout)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	root := cmd.RootCommandForTest()
	// Flags live on shared command objects and survive between Execute calls.
	resetFlags := func() {
		_ = root.PersistentFlags().Set("error-json", "false")
		for _, f := range []string{"allow-destructive", "yes"} {
			_ = cmd.VerifyCommandForTest().Flags().Set(f, "false")
		}
		_ = cmd.VerifyCommandForTest().Flags().Set("phase", "0")
	}
	resetFlags()
	t.Cleanup(resetFlags)
	root.SetOut(stdout)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"verify", "--json", "--error-json", "--project", project}, extraArgs...))
	err = root.Execute()

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for {
		var v any
		if e := dec.Decode(&v); e != nil {
			if !errors.Is(e, io.EOF) {
				t.Fatalf("stdout is not a JSON stream: %v\n%q", e, stdout.String())
			}
			break
		}
		docs++
	}
	total = docs
	var reported interface{ AlreadyReported() bool }
	if err != nil && (!errors.As(err, &reported) || !reported.AlreadyReported()) {
		total++ // main prints the envelope
	}
	return docs, total, err
}

func fakeAllGreen(t *testing.T) {
	t.Helper()
	verify.SetExecGovardFakeForTest(func(context.Context, engine.Config, verify.VerifyOpts, ...string) (verify.Evidence, bool) {
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
}

func TestVerifyErrorJSONGateRefusalIsOneDocument(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_TEST_SATISFIED_CAPABILITIES", "docker")
	fakeAllGreen(t)
	_, total, err := verifyStdoutDocuments(t, t.TempDir())
	if !errors.Is(err, verify.ErrNeedAllowDestructive) {
		t.Fatalf("err = %v, want ErrNeedAllowDestructive", err)
	}
	if total != 1 {
		t.Fatalf("consumer sees %d documents, want 1", total)
	}
}

func TestVerifyErrorJSONSnapshotGateIsOneDocument(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_TEST_SATISFIED_CAPABILITIES", "docker")
	fakeAllGreen(t)
	docs, total, err := verifyStdoutDocuments(t, t.TempDir(), "--allow-destructive")
	if !errors.Is(err, verify.ErrNeedSnapshot) {
		t.Fatalf("err = %v, want ErrNeedSnapshot", err)
	}
	if total != 1 || docs != 1 {
		t.Fatalf("docs=%d total=%d, want the merged phase 1-4 document alone", docs, total)
	}
}

func TestVerifyErrorJSONItemsFailedIsOneDocument(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	project := t.TempDir()
	if err := os.WriteFile(engine.LockFilePath(project), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		if len(args) > 0 && args[0] == "lock" {
			return verify.Evidence{ExitCode: 1, OutputExcerpt: "mismatch"}, true
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	_, total, err := verifyStdoutDocuments(t, project, "--phase", "1")
	if !errors.Is(err, cmd.ErrItemsFailed) {
		t.Fatalf("err = %v, want ErrItemsFailed", err)
	}
	if total != 1 {
		t.Fatalf("consumer sees %d documents, want 1", total)
	}
}
