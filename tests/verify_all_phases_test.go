package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/pterm/pterm"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/verify"
)

// runVerifyAllPhases drives `govard verify` with no --phase through the real
// command tree, with every item faked so nothing touches Docker.
func runVerifyAllPhases(t *testing.T, project string, extraArgs ...string) (map[string]any, error) {
	t.Helper()

	stdout := &bytes.Buffer{}
	pterm.SetDefaultOutput(stdout)
	t.Cleanup(func() { pterm.SetDefaultOutput(nil) })

	root := cmd.RootCommandForTest()
	root.SetOut(stdout)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"verify", "--json", "--project", project}, extraArgs...))

	err := root.Execute()

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &payload); unmarshalErr != nil {
		t.Fatalf("stdout is not a single JSON object: %v\nstdout=%q", unmarshalErr, stdout.String())
	}
	return payload, err
}

// TestVerifyAllPhasesStopsBeforePhase5WithoutTheFlag pins the gate on the
// aggregate path: without --allow-destructive the run must stop before phase 5,
// report the gate as a JSON envelope, and leave stdout as that one object.
func TestVerifyAllPhasesStopsBeforePhase5WithoutTheFlag(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	project := t.TempDir()

	ranDestructive := false
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		// Phase 4 legitimately runs `snapshot create`/`list`; the destructive
		// phase-5 items are the volume wipe and the restore.
		if len(args) > 1 && args[0] == "snapshot" && args[1] == "restore" {
			ranDestructive = true
		}
		for _, a := range args {
			if a == "-v" {
				ranDestructive = true
			}
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	payload, err := runVerifyAllPhases(t, project)
	if !errors.Is(err, verify.ErrNeedAllowDestructive) {
		t.Fatalf("Execute() = %v, want ErrNeedAllowDestructive", err)
	}
	if _, ok := payload["error"]; !ok {
		t.Fatalf("expected the gate as a JSON error envelope, got %v", payload)
	}
	if ranDestructive {
		t.Fatal("a destructive phase-5 item (snapshot restore / env down -v) ran without --allow-destructive")
	}
}

// TestVerifyAllPhasesRecomputesTheMergedVerdict pins the all-phases merge: the
// combined result starts as a copy of phase 1, so a green phase 1 followed by a
// red later phase must still report "failed" and exit non-zero.
func TestVerifyAllPhasesRecomputesTheMergedVerdict(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	project := t.TempDir()

	// Satisfy the phase-5 gate for this project with a usable snapshot.
	makeSnapshot(t, project, "20260101-000000", "2026-01-01T00:00:00Z")
	writePhase4(t, project, goodPhase4(project, "20260101-000000"))

	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		// Phase 1 is green; the phase-4 snapshot list is red, so the merged
		// verdict must come from the later phase, not from phase 1.
		if len(args) > 1 && args[0] == "snapshot" && args[1] == "list" {
			return verify.Evidence{ExitCode: 1, OutputExcerpt: "ERROR list failed"}, true
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	payload, err := runVerifyAllPhases(t, project, "--allow-destructive")
	if !errors.Is(err, cmd.ErrItemsFailed) {
		t.Fatalf("Execute() = %v, want ErrItemsFailed", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("merged status = %v, want \"failed\" (phase 1's verdict must not survive the merge)", payload["status"])
	}
	if payload["phase"] != "all" {
		t.Fatalf("phase = %v, want \"all\"", payload["phase"])
	}
}
