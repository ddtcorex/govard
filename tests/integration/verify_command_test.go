//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCommandPlanJSON(t *testing.T) {
	env := NewTestEnvironment(t)
	dir := env.CreateTestProject(t, "verify-plan", map[string]string{
		".govard.yml": "project_name: verify-test\nframework: magento2\ndomain: verify-test.test\n",
	})
	result := env.RunGovardWithEnv(t, dir, nil, "verify", "--plan", "--json")
	result.AssertSuccess(t)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		// Try as array of phases when --plan with all phases? But our verify --plan --json with no phase runs all 5 sequentially, last output is phase5.
		// So just check stdout contains JSON.
		t.Fatalf("stdout not JSON: %v\nstdout: %s", err, result.Stdout)
	}
}

func TestVerifyCommandPhase1JSON(t *testing.T) {
	env := NewTestEnvironment(t)
	dir := env.CreateTestProject(t, "verify-phase1", map[string]string{
		".govard.yml": "project_name: verify-test2\nframework: magento2\ndomain: verify-test2.test\n",
	})
	result := env.RunGovardWithEnv(t, dir, nil, "verify", "--phase", "1", "--json")

	// stdout must be exactly one JSON object even when items fail: the verdict
	// line is human output and belongs on stderr. This runs the real binary, so
	// it is the only test that exercises the real file descriptors.
	var payload struct {
		Status string `json:"status"`
		Items  []struct {
			ID       string `json:"id"`
			ExitCode int    `json:"exit_code"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("stdout not a single JSON object: %v\nstdout: %s", err, result.Stdout)
	}
	if len(payload.Items) == 0 {
		t.Fatalf("JSON missing items: %s", result.Stdout)
	}

	red := 0
	for _, item := range payload.Items {
		if item.ExitCode != 0 {
			red++
		}
	}
	if red > 0 {
		if payload.Status != "failed" {
			t.Fatalf("status = %q with %d red item(s), want \"failed\"", payload.Status, red)
		}
		// A red checklist is an execution failure (1), never success. This
		// project cannot make every P1 item pass, so the assertion is the
		// correlation: the exit code tracks the items.
		if result.ExitCode == 0 {
			t.Fatalf("exit code 0 with %d red item(s); a failing checklist must exit non-zero\nstdout: %s", red, result.Stdout)
		}
	} else if result.ExitCode != 0 {
		t.Fatalf("exit code %d with no red item(s), want 0\nstdout: %s", result.ExitCode, result.Stdout)
	}
}

// TestVerifyCommandRedRunWithErrorJSONWritesOneDocument pins that --json and
// --error-json together still leave ONE document on stdout: the verdict message
// is already on stderr, so the CLI must not append an envelope after the run
// artifact. Two JSON documents are unparseable for every machine consumer.
func TestVerifyCommandRedRunWithErrorJSONWritesOneDocument(t *testing.T) {
	env := NewTestEnvironment(t)
	dir := env.CreateTestProject(t, "verify-error-json", map[string]string{
		".govard.yml": "project_name: verify-test3\nframework: magento2\ndomain: verify-test3.test\n",
	})
	result := env.RunGovardWithEnv(t, dir, nil, "verify", "--phase", "1", "--json", "--error-json")

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\nstdout: %s", err, result.Stdout)
	}
	if result.ExitCode == 0 {
		t.Fatalf("a red checklist must exit non-zero; stdout: %s", result.Stdout)
	}
}

// TestVerifyFrameworkItemFailurePropagates pins a framework-declared item's
// failure path end to end. It cannot live in the unit suite: execGovard treats
// the running executable as a test binary whenever its path contains ".test",
// so every in-process item is stubbed to exit 0 and GOVARD_VERIFY_BIN is
// ignored there. The shim also proves the item keeps the command's own output —
// a red verdict with an empty excerpt is not diagnosable.
func TestVerifyFrameworkItemFailurePropagates(t *testing.T) {
	env := NewTestEnvironment(t)
	dir := env.CreateTestProject(t, "verify-framework-item", map[string]string{
		".govard.yml": "project_name: verify-fw\nframework: laravel\ndomain: verify-fw.test\n",
	})

	shim := filepath.Join(t.TempDir(), "govard-shim")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\necho 'shim: deliberate failure' >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}

	result := env.RunGovardWithEnv(t, dir, []string{"GOVARD_VERIFY_BIN=" + shim},
		"verify", "--phase", "3", "--json")

	var payload struct {
		Status string `json:"status"`
		Items  []struct {
			ID              string `json:"id"`
			ExitCode        int    `json:"exit_code"`
			EvidenceExcerpt string `json:"evidence_excerpt"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\nstdout: %s", err, result.Stdout)
	}

	found := false
	for _, item := range payload.Items {
		if item.ID != "P3-LAR-02" {
			continue
		}
		found = true
		if item.ExitCode != 7 {
			t.Errorf("P3-LAR-02 exit_code = %d, want the shim's 7", item.ExitCode)
		}
		if !strings.Contains(item.EvidenceExcerpt, "shim: deliberate failure") {
			t.Errorf("P3-LAR-02 kept no output: %q", item.EvidenceExcerpt)
		}
	}
	if !found {
		t.Fatalf("P3-LAR-02 is missing from the phase 3 artifact: %s", result.Stdout)
	}
	if payload.Status != "failed" {
		t.Fatalf("status = %q with a red framework item, want \"failed\"", payload.Status)
	}
}

// TestDeployStatusFailsForAnUnconfiguredRemoteInBothForms is the condition the
// P4-14 checklist item needs: `deploy status --remote <name>` has to exit
// non-zero, or the item is green for exactly the failure it exists to detect.
//
// The two forms used to disagree, and this test used to record the gap: the
// JSON document was written before the "no configured remote could be reached"
// check, so `--json` exited 0 while the human form exited 1. A name the project
// never configured is now refused while the remote resolves — before either form
// prints anything — so both answer 4 and there is no form left that hides it.
func TestDeployStatusFailsForAnUnconfiguredRemoteInBothForms(t *testing.T) {
	env := NewTestEnvironment(t)
	dir := env.CreateTestProject(t, "deploy-status-exit", map[string]string{
		".govard.yml": "project_name: status-exit\nframework: magento2\ndomain: status-exit.test\n",
	})

	jsonResult := env.RunGovardWithEnv(t, dir, nil, "deploy", "status", "--remote", "absent-remote", "--json")
	if jsonResult.ExitCode != 4 {
		t.Fatalf("deploy status --json exit code = %d, want 4 — P4-14 reads the exit code, and a remote the project never configured is a configuration error\nstdout: %s",
			jsonResult.ExitCode, jsonResult.Stdout)
	}

	humanResult := env.RunGovardWithEnv(t, dir, nil, "deploy", "status", "--remote", "absent-remote")
	if humanResult.ExitCode != 4 {
		t.Fatalf("deploy status exit code = %d, want 4; the two forms must not disagree about a name the project never had\nstdout: %s",
			humanResult.ExitCode, humanResult.Stdout)
	}
}
