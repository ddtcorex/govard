package tests

import (
	"errors"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/verify"
)

func TestRunResultFailedAndCounts(t *testing.T) {
	res := verify.RunResult{Items: []verify.RunItem{
		{ID: "P1-01", ExitCode: 0},
		{ID: "P1-02", ExitCode: 1},
		{ID: "P1-03", ExitCode: 0},
	}}
	if !res.Failed() {
		t.Fatal("Failed() = false with one red item, want true")
	}
	passed, failed := res.Counts()
	if passed != 2 || failed != 1 {
		t.Fatalf("Counts() = %d/%d, want 2/1", passed, failed)
	}
}

func TestRunResultAllGreenIsNotFailed(t *testing.T) {
	res := verify.RunResult{Items: []verify.RunItem{{ID: "P1-01", ExitCode: 0}}}
	if res.Failed() {
		t.Fatal("Failed() = true with all green, want false")
	}
}

func TestSummariseReturnsErrorOnlyWhenItemsFailed(t *testing.T) {
	red := verify.RunResult{Items: []verify.RunItem{{ID: "P1-01", ExitCode: 1}}}
	if err := cmd.SummariseForTest(red); !errors.Is(err, cmd.ErrItemsFailed) {
		t.Fatalf("red run: got %v, want ErrItemsFailed", err)
	}
	green := verify.RunResult{Items: []verify.RunItem{{ID: "P1-01", ExitCode: 0}}}
	if err := cmd.SummariseForTest(green); err != nil {
		t.Fatalf("green run: got %v, want nil", err)
	}
}

func TestSummariseExitCodeIsOneNotTwo(t *testing.T) {
	err := cmd.SummariseForTest(verify.RunResult{Items: []verify.RunItem{{ID: "P1-01", ExitCode: 1}}})
	if got := cli.Code(err); got != cli.CodeError {
		t.Fatalf("cli.Code = %d, want %d (CodeError; 2 is reserved for USAGE)", got, cli.CodeError)
	}
}

func TestRefreshStatusRecomputesMergedVerdict(t *testing.T) {
	// A merged all-phases result starts life as a copy of phase 1, so it can
	// carry "passed" while a later phase is red. RefreshStatus must recompute.
	merged := verify.RunResult{Status: "passed", Items: []verify.RunItem{
		{ID: "P1-01", ExitCode: 0},
		{ID: "P4-08", ExitCode: 1},
	}}
	merged.RefreshStatus()
	if merged.Status != "failed" {
		t.Fatalf("merged Status = %q, want \"failed\" (phase 1's verdict must not survive the merge)", merged.Status)
	}
}

func TestRefreshStatusGreenStaysPassed(t *testing.T) {
	res := verify.RunResult{Status: "failed", Items: []verify.RunItem{{ID: "P1-01", ExitCode: 0}}}
	res.RefreshStatus()
	if res.Status != "passed" {
		t.Fatalf("Status = %q, want \"passed\"", res.Status)
	}
}
