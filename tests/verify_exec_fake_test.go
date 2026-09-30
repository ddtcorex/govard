package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

func TestIsTestBinaryMatchesOnlyTheBasename(t *testing.T) {
	cases := map[string]bool{
		"/home/u/src/govard.test/bin/govard": false,
		"/tmp/x.testing/govard":              false,
		"/tmp/verify.test":                   true,
	}
	for bin, want := range cases {
		if got := verify.IsTestBinaryForTest(bin); got != want {
			t.Errorf("isTestBinary(%q) = %v, want %v", bin, got, want)
		}
	}
}

func TestGovardVerifyFakeEnvIsIgnoredOutsideTests(t *testing.T) {
	stub := filepath.Join(t.TempDir(), "govard")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(verify.EnvBinaryOverride, stub)
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	ev := verify.ExecGovardForTest(context.Background(), engine.Config{}, verify.VerifyOpts{}, "doctor")
	if ev.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7 (the real stub ran, nothing faked)", ev.ExitCode)
	}
	if ev.Fake {
		t.Fatalf("Fake = true for a real process run")
	}
}

func TestFakeEvidenceMarksTheRunArtifact(t *testing.T) {
	t.Setenv("GOVARD_VERIFY_FAKE", "1")
	t.Setenv(verify.EnvBinaryOverride, "")

	ev := verify.ExecGovardForTest(context.Background(), engine.Config{}, verify.VerifyOpts{}, "doctor")
	if !ev.Fake {
		t.Fatalf("test-binary fake path must set Evidence.Fake")
	}

	res := verify.RunResult{Items: []verify.RunItem{{ID: "X", Fake: ev.Fake}}}
	res.RefreshStatus()
	if !res.Fake {
		t.Fatalf("RunResult.Fake = false, want true when any item is fake")
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["fake"] != true {
		t.Fatalf("artifact lacks top-level fake marker: %s", b)
	}

	real := verify.RunResult{Items: []verify.RunItem{{ID: "Y"}}}
	real.RefreshStatus()
	rb, _ := json.Marshal(real)
	if string(rb) == "" || real.Fake || real.Status != "passed" {
		t.Fatalf("real run must stay unmarked and passed: %+v", real)
	}
	var rm map[string]any
	_ = json.Unmarshal(rb, &rm)
	if _, ok := rm["fake"]; ok {
		t.Fatalf("real run artifact must omit fake: %s", rb)
	}
}
