package tests

import (
	"context"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// Each new remote item is pinned by the exact argv it invokes, read back from
// the fake runner's evidence. The title is not evidence: P4-05/P4-06/P4-07
// shipped titles promising invocations their bodies never made.
func TestVerifyRemoteItemArgv(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 4,
		verify.VerifyOpts{Remote: "sandbox"})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}

	want := map[string]string{
		"P4-13": "fake: deploy plan sandbox --json",
		// Deliberately no --json: runDeployStatus returns its JSON line before
		// the "no configured remote could be reached" check, so the JSON form
		// exits 0 for an unreachable remote and the item could never fail.
		"P4-14": "fake: deploy status --remote sandbox",
		"P4-15": "fake: deploy releases --remote sandbox --json",
		"P4-16": "fake: remote list",
	}

	seen := map[string]bool{}
	for _, it := range res.Items {
		expected, ok := want[it.ID]
		if !ok {
			continue
		}
		seen[it.ID] = true
		if it.EvidenceExcerpt != expected {
			t.Errorf("%s ran %q, want %q", it.ID, it.EvidenceExcerpt, expected)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("%s missing from phase 4", id)
		}
	}
}

func TestVerifyRemoteItemsDefaultToStaging(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 4, verify.VerifyOpts{})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	for _, it := range res.Items {
		if it.ID != "P4-13" {
			continue
		}
		if !strings.Contains(it.EvidenceExcerpt, "deploy plan staging --json") {
			t.Fatalf("P4-13 with no --remote ran %q, want the staging default", it.EvidenceExcerpt)
		}
		return
	}
	t.Fatal("P4-13 missing from phase 4")
}
