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
	result.AssertSuccess(t)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("stdout not JSON: %v\nstdout: %s", err, result.Stdout)
	}
	if _, ok := payload["items"]; !ok {
		t.Fatalf("JSON missing items: %s", result.Stdout)
	}
}
