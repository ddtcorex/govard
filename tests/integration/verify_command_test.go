//go:build integration
// +build integration

package integration

import (
	"encoding/json"
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
