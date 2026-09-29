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

// A run that was not told which remote to use does not guess one: it used to
// probe whatever the project happens to call staging. Every item whose argv
// names a remote keeps its row and reports why it did not run.
func TestRemoteItemsSkipWithoutAnExplicitRemote(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	probe := installGuardProbe(t)

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 4,
		verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}

	// P4-16 (`remote list`) is deliberately absent: it carries the same guard
	// but names no remote, so it must keep running.
	for _, id := range []string{"P4-01", "P4-03", "P4-04", "P4-05", "P4-06", "P4-07", "P4-13", "P4-14", "P4-15"} {
		row := findGuardRow(t, res, id)
		if !row.Skipped {
			t.Errorf("%s ran without --remote: %q", id, row.EvidenceExcerpt)
		}
		if !strings.Contains(row.SkipReason, "--remote") {
			t.Errorf("%s skip reason %q does not name --remote", id, row.SkipReason)
		}
	}
	if row := findGuardRow(t, res, "P4-16"); row.Skipped {
		t.Errorf("P4-16 names no remote and must still run: %+v", row)
	}
	for _, args := range probe.argv {
		if strings.Contains(strings.Join(args, " "), "staging") {
			t.Errorf("a run with no --remote invoked %q, which carries the removed staging default", strings.Join(args, " "))
		}
	}

	// --allow-remote-write arms the two REMOTE-WRITE items, but it is not a
	// remote: without one they still have no target to write to.
	before := probe.called()
	res, err = verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 2,
		verify.VerifyOpts{AllowRemoteWrite: true, ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase phase 2: %v", err)
	}
	for _, id := range []string{"P2-04", "P2-05", "P2-06", "P2-07", "P2-08"} {
		row := findGuardRow(t, res, id)
		if !row.Skipped {
			t.Errorf("%s ran without --remote: %q", id, row.EvidenceExcerpt)
		}
		if !strings.Contains(row.SkipReason, "--remote") {
			t.Errorf("%s skip reason %q does not name --remote", id, row.SkipReason)
		}
	}
	for _, args := range probe.argv[before:] {
		if len(args) > 0 && args[0] == "bootstrap" {
			t.Errorf("with --allow-remote-write but no --remote, a bootstrap item still invoked %q", strings.Join(args, " "))
		}
	}
}

// The remote an item contacts is the one --remote names, and nothing else.
func TestRemoteItemsUseTheNamedRemote(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	probe := installGuardProbe(t)

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 4,
		verify.VerifyOpts{Remote: "sandbox", ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}

	found := false
	for _, it := range res.Items {
		if it.ID != "P4-13" {
			continue
		}
		found = true
		if it.Skipped {
			t.Fatalf("P4-13 skipped although --remote named sandbox: %s", it.SkipReason)
		}
	}
	if !found {
		t.Fatal("P4-13 missing from phase 4")
	}

	var got []string
	for _, args := range probe.argv {
		if len(args) > 1 && args[0] == "deploy" && args[1] == "plan" {
			got = args
		}
	}
	if joined := strings.Join(got, " "); joined != "deploy plan sandbox --json" {
		t.Fatalf("P4-13 invoked %q, want %q", joined, "deploy plan sandbox --json")
	}
}
