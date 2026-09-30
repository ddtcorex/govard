package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// Every item is faked so nothing touches Docker — and the two host-probe
	// items (P2-13, P4-11) need the probe fake too: they do their own HTTP
	// against the domain derived from the project directory name.
	fakeProbeHTTP(t)

	stdout := &bytes.Buffer{}
	pterm.SetDefaultOutput(stdout)
	// os.Stdout, not nil: pterm's default writer is initialised to os.Stdout
	// (print.go:13) and there is no API that puts it back, so SetDefaultOutput(nil)
	// leaves a nil io.Writer behind and the next render in this package panics in
	// fmt.Fprint. This cleanup runs at the end of every test that uses this helper.
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

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

// TestVerifyAllPhasesJSONReportsASnapshotGateBlock pins the envelope on the
// aggregate path: a snapshot-gate block must still leave a JSON document on
// stdout. Returning silently left it empty for a consumer that asked for --json.
func TestVerifyAllPhasesJSONReportsASnapshotGateBlock(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	project := t.TempDir() // no snapshot, so no phase-4 artifact can satisfy the gate

	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, _ ...string) (verify.Evidence, bool) {
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "ok"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	// --allow-destructive is passed so the run reaches the snapshot gate rather
	// than stopping at the missing flag; every item is faked, so nothing runs.
	payload, err := runVerifyAllPhases(t, project, "--allow-destructive")
	if !errors.Is(err, verify.ErrNeedSnapshot) {
		t.Fatalf("Execute() = %v, want ErrNeedSnapshot", err)
	}
	if _, ok := payload["error"]; !ok {
		t.Fatalf("stdout carried no gate envelope: %v", payload)
	}
}

// TestVerifyAllPhasesWithoutTheFlagRunsNothing pins the ordering: the
// destructive gate is checked before phase 1, in both output modes, so a
// refused run has not already stopped and started the environment.
func TestVerifyAllPhasesWithoutTheFlagRunsNothing(t *testing.T) {
	for _, jsonFlag := range []bool{true, false} {
		name := "human"
		if jsonFlag {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			project := t.TempDir()
			calls := fakeAllExec(t)

			stdout := &bytes.Buffer{}
			pterm.SetDefaultOutput(stdout)
			t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
			root := cmd.RootCommandForTest()
			root.SetOut(stdout)
			root.SetErr(io.Discard)
			// cobra flag values persist on the shared root between tests, so the
			// absence of the flag is stated, not assumed.
			args := []string{"verify", "--project", project, "--allow-destructive=false", "--yes=false"}
			args = append(args, "--json="+strconv.FormatBool(jsonFlag))
			root.SetArgs(args)

			err := root.Execute()
			if !errors.Is(err, verify.ErrNeedAllowDestructive) {
				t.Fatalf("Execute() = %v, want ErrNeedAllowDestructive", err)
			}
			if *calls != 0 {
				t.Fatalf("%d command(s) ran before the gate refused the run", *calls)
			}
			if jsonFlag {
				var payload map[string]any
				if e := json.Unmarshal(stdout.Bytes(), &payload); e != nil || payload["error"] == nil {
					t.Fatalf("stdout = %q, want one JSON error envelope (%v)", stdout.String(), e)
				}
			}
			if entries, _ := os.ReadDir(verify.ProjectRunsDir(project)); len(entries) != 0 {
				t.Fatalf("a refused run wrote %d artifact(s)", len(entries))
			}
		})
	}
}

// TestVerifyAllPhasesHumanRunReachesPhase5 drives the whole
// public path without --json: phase 4 must leave the artifact phase 5 reads.
func TestVerifyAllPhasesHumanRunReachesPhase5(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	project := t.TempDir()
	makeSnapshot(t, project, "20260101-000000", "2026-01-01T00:00:00Z")
	fakeAllExec(t)

	pterm.SetDefaultOutput(io.Discard)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"verify", "--project", project, "--json=false", "--allow-destructive", "--yes=false"})

	err := root.Execute()
	if errors.Is(err, verify.ErrNeedSnapshot) || errors.Is(err, verify.ErrNeedAllowDestructive) {
		t.Fatalf("Execute() = %v: the gate rejected a run whose phase 4 just recorded the snapshot", err)
	}
	if _, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: project}); !ok {
		t.Fatal("no gate-satisfying artifact after a human-mode run")
	}
}

// TestVerifyAllPhasesKeepsPhases1To4WhenTheRecordCannotBeWritten: with an
// unwritable runs dir phase 5 fails closed (its gate cannot see phase 4), but
// with an error that names the failed record write, and the phase 1-4 results
// are still rendered in both output modes.
func TestVerifyAllPhasesKeepsPhases1To4WhenTheRecordCannotBeWritten(t *testing.T) {
	for _, jsonFlag := range []bool{true, false} {
		name := "human"
		if jsonFlag {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			project := t.TempDir()
			makeSnapshot(t, project, "20260101-000000", "2026-01-01T00:00:00Z")
			// A regular file where the runs dir belongs: MkdirAll fails.
			runsDir := verify.ProjectRunsDir(project)
			if err := os.MkdirAll(filepath.Dir(runsDir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(runsDir, []byte("not a directory"), 0o644); err != nil {
				t.Fatal(err)
			}
			fakeAllExec(t)

			stdout := &bytes.Buffer{}
			pterm.SetDefaultOutput(stdout)
			t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
			root := cmd.RootCommandForTest()
			root.SetOut(stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"verify", "--project", project, "--allow-destructive", "--yes=false",
				"--json=" + strconv.FormatBool(jsonFlag)})

			err := root.Execute()
			if !errors.Is(err, verify.ErrRunNotRecorded) {
				t.Fatalf("Execute() = %v, want ErrRunNotRecorded", err)
			}
			if errors.Is(err, verify.ErrNeedSnapshot) {
				t.Fatalf("error %q is the generic snapshot error, want the record failure", err)
			}
			out := stdout.String()
			if !strings.Contains(out, "P4-08") || strings.Contains(out, "P5-01") {
				t.Fatalf("output must carry phases 1-4 and no phase 5 item:\n%s", out)
			}
			if jsonFlag {
				var payload struct {
					Phase string `json:"phase"`
					Items []struct {
						ID string `json:"id"`
					} `json:"items"`
				}
				if e := json.Unmarshal(stdout.Bytes(), &payload); e != nil {
					t.Fatalf("stdout is not one JSON document: %v\n%s", e, out)
				}
				if payload.Phase != "all" || len(payload.Items) == 0 {
					t.Fatalf("merged result = %+v, want phase all with the phase 1-4 items", payload)
				}
			}
		})
	}
}
