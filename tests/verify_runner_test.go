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

func TestVerifyRunnerGateP5WithoutSnapshot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", dir)

	_, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 5, verify.VerifyOpts{AllowDestructive: true})
	if err == nil {
		t.Fatalf("expected ErrNeedSnapshot, got nil")
	}
	if err != verify.ErrNeedSnapshot {
		t.Fatalf("expected ErrNeedSnapshot, got %v", err)
	}
}

func TestVerifyRunnerAllowDestructiveRequired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", dir)
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	// Create a fake phase4 file with P4-08 PASS
	verifyDir := filepath.Join(dir, "verify-runs")
	if err := os.MkdirAll(verifyDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	res := verify.RunResult{
		GovardVersion: "test",
		ProjectSHA:    "test-sha",
		Phase:         "phase4",
		Items: []verify.RunItem{
			{ID: "P4-08", Command: "govard snapshot create", ExitCode: 0, EvidenceExcerpt: "ok"},
		},
	}
	b, _ := json.Marshal(res)
	if err := os.WriteFile(filepath.Join(verifyDir, "2026-01-01T00-00-00Z-phase4.json"), b, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 5, verify.VerifyOpts{AllowDestructive: false})
	if err != verify.ErrNeedAllowDestructive {
		t.Fatalf("expected ErrNeedAllowDestructive, got %v", err)
	}

	// With allow, should succeed (even if items are stubs)
	got, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 5, verify.VerifyOpts{AllowDestructive: true})
	if err != nil {
		t.Fatalf("expected success with AllowDestructive, got %v", err)
	}
	if len(got.Items) == 0 {
		t.Fatalf("expected P5 items, got 0")
	}
}

func TestVerifyRunnerPlanNoSideEffect(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", dir)

	callCount := 0
	orig := make([]verify.Item, len(verify.Registry))
	copy(orig, verify.Registry)
	// Replace Runs with counting mock
	for i := range verify.Registry {
		verify.Registry[i].Run = func(_ context.Context, _ engine.Config, _ verify.VerifyOpts) verify.Evidence {
			callCount++
			return verify.Evidence{ExitCode: 0, OutputExcerpt: "should not happen in plan"}
		}
	}
	t.Cleanup(func() {
		copy(verify.Registry, orig)
	})

	_, err := verify.RunPhase(context.Background(), engine.Config{}, 1, verify.VerifyOpts{Plan: true})
	if err != nil {
		t.Fatalf("RunPhase plan: %v", err)
	}
	if callCount != 0 {
		t.Fatalf("Plan should not call Run, but called %d times", callCount)
	}
}

func TestVerifyRunnerJSONSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", dir)
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 1, verify.VerifyOpts{JSON: true})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	if res.GovardVersion == "" {
		t.Fatalf("missing GovardVersion")
	}
	if len(res.Items) == 0 {
		t.Fatalf("expected items")
	}
	// Check file written
	verifyDir := filepath.Join(dir, "verify-runs")
	entries, err := os.ReadDir(verifyDir)
	if err != nil {
		t.Fatalf("read verify-runs: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected file in verify-runs")
	}
	b, err := os.ReadFile(filepath.Join(verifyDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var got verify.RunResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json invalid: %v", err)
	}
	if got.Phase == "" || len(got.Items) == 0 {
		t.Fatalf("json content missing phase/items")
	}
}
