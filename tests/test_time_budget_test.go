package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/testbudget"
)

func writeBudgetFile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "budget.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write budget file: %v", err)
	}
	return path
}

const validBudgetYAML = `
suites:
  unit:
    default: 2s
    overrides:
      TestSlowOne:
        budget: 5s
        reason: it waits on a real poll interval
      TestOverridden:
        budget: 8s
        reason: it drives a real process group to exit
`

func TestTestBudgetLoadReadsSuiteDefaultsAndOverrides(t *testing.T) {
	budget, err := testbudget.Load(writeBudgetFile(t, validBudgetYAML), 1)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	suite, err := budget.Suite("unit")
	if err != nil {
		t.Fatalf("Suite() error = %v", err)
	}
	if got, want := time.Duration(suite.Default), 2*time.Second; got != want {
		t.Fatalf("default = %v, want %v", got, want)
	}

	limit, reason := suite.LimitFor("TestSlowOne")
	if limit != 5*time.Second {
		t.Fatalf("override limit = %v, want 5s", limit)
	}
	if reason != "it waits on a real poll interval" {
		t.Fatalf("override reason = %q, want the reason written in the file", reason)
	}

	if limit, reason := suite.LimitFor("TestAnythingElse"); limit != 2*time.Second || reason != "" {
		t.Fatalf("unlisted test limit = (%v, %q), want (2s, no reason)", limit, reason)
	}
}

// The allowlist is the part of this gate most likely to rot, so the reason is
// enforced at load time rather than reviewed by eye. A file that loads is a file
// where every allowance can be explained.
func TestTestBudgetLoadRefusesAnOverrideWithoutAReason(t *testing.T) {
	_, err := testbudget.Load(writeBudgetFile(t, `
suites:
  unit:
    default: 2s
    overrides:
      TestMystery:
        budget: 30s
        reason: "   "
`), 1)
	if err == nil {
		t.Fatal("Load() accepted an override with a blank reason")
	}
	if !strings.Contains(err.Error(), "TestMystery") {
		t.Fatalf("error %q does not name the offending test", err)
	}
}

func TestTestBudgetLoadRejectsUnparseableAndNonPositiveDurations(t *testing.T) {
	for name, body := range map[string]string{
		"not a duration": `
suites:
  unit:
    default: soon
`,
		"bare number": `
suites:
  unit:
    default: 2
`,
		"zero": `
suites:
  unit:
    default: 0s
`,
		"negative": `
suites:
  unit:
    default: -3s
`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := testbudget.Load(writeBudgetFile(t, body), 1); err == nil {
				t.Fatalf("Load() accepted %s", name)
			}
		})
	}
}

func TestTestBudgetLoadRejectsAnEmptyOrMissingSuite(t *testing.T) {
	if _, err := testbudget.Load(writeBudgetFile(t, "suites: {}\n"), 1); err == nil {
		t.Fatal("Load() accepted a budget file with no suites")
	}

	budget, err := testbudget.Load(writeBudgetFile(t, validBudgetYAML), 1)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := budget.Suite("integration"); err == nil {
		t.Fatal("Suite() returned a suite the file never declared")
	}
}

// A shared CI runner is slower than a laptop. The scale exists so the whole file
// moves together instead of every slow test being individually re-argued.
func TestTestBudgetScaleWidensEveryBudget(t *testing.T) {
	budget, err := testbudget.Load(writeBudgetFile(t, validBudgetYAML), 2.5)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	suite, err := budget.Suite("unit")
	if err != nil {
		t.Fatalf("Suite() error = %v", err)
	}

	if got, want := time.Duration(suite.Default), 5*time.Second; got != want {
		t.Fatalf("scaled default = %v, want %v", got, want)
	}
	if got, _ := suite.LimitFor("TestSlowOne"); got != 12500*time.Millisecond {
		t.Fatalf("scaled override = %v, want 12.5s", got)
	}

	if _, err := testbudget.Load(writeBudgetFile(t, validBudgetYAML), 0); err == nil {
		t.Fatal("Load() accepted a zero scale")
	}
}

func TestTestBudgetEvaluateFlagsOnlyTestsOverTheirLimit(t *testing.T) {
	budget, err := testbudget.Load(writeBudgetFile(t, validBudgetYAML), 1)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	suite, err := budget.Suite("unit")
	if err != nil {
		t.Fatalf("Suite() error = %v", err)
	}

	violations := suite.Evaluate(map[string]time.Duration{
		"TestFast":       10 * time.Millisecond,
		"TestAtTheLimit": 2 * time.Second, // equal is within budget, not over it
		"TestSlowOne":    4 * time.Second, // inside its own allowance
		"TestUnlisted":   3 * time.Second, // over the default, no allowance on file
		"TestOverridden": 9 * time.Second, // over even its own allowance
	})

	if len(violations) != 2 {
		names := make([]string, 0, len(violations))
		for _, v := range violations {
			names = append(names, v.Test)
		}
		t.Fatalf("got %d violations %v, want 2 (TestUnlisted, TestOverridden)", len(violations), names)
	}

	// Slowest first, so the report leads with the worst offender.
	if violations[0].Test != "TestOverridden" {
		t.Fatalf("first violation = %s, want the slowest test first", violations[0].Test)
	}
	// The two failures are different problems and the report has to say which is
	// which: TestUnlisted has no allowance at all, TestOverridden blew through one.
	if !violations[0].FromOverride {
		t.Fatal("TestOverridden blew through an allowance that exists, so FromOverride must be true")
	}
	if violations[1].Test != "TestUnlisted" || violations[1].FromOverride {
		t.Fatalf("second violation = %+v, want TestUnlisted with no allowance on file", violations[1])
	}
}

// An allowance for a test that no longer exists is how an allowlist dies
// quietly: the test is renamed, the entry stays, and the next person to write a
// slow test finds a precedent waiting.
func TestTestBudgetStaleOverridesReportsAllowancesForTestsThatNoLongerRun(t *testing.T) {
	budget, err := testbudget.Load(writeBudgetFile(t, validBudgetYAML), 1)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	suite, err := budget.Suite("unit")
	if err != nil {
		t.Fatalf("Suite() error = %v", err)
	}

	stale := suite.StaleOverrides(map[string]bool{"TestStillHere": true})
	if len(stale) != 2 || stale[0] != "TestOverridden" || stale[1] != "TestSlowOne" {
		t.Fatalf("stale = %v, want [TestOverridden TestSlowOne]", stale)
	}
}

// `go test ./...` runs every package in one invocation, and two packages are
// free to declare a test with the same name. Recording by name alone would keep
// whichever finished last, so a slow one could vanish from the report entirely —
// the one way a budget gate silently stops working.
func TestTestBudgetTimingsKeepTheSlowestOfASharedTestName(t *testing.T) {
	timings := testbudget.NewTimings()
	timings.Record("govard/tests", "TestShared", 10*time.Millisecond)
	timings.Record("govard/internal/cmd", "TestShared", 900*time.Millisecond)

	byName := timings.ByName()
	if len(byName) != 1 {
		t.Fatalf("ByName() has %d entries, want the two packages merged into 1", len(byName))
	}
	if got := byName["TestShared"]; got != 900*time.Millisecond {
		t.Fatalf("ByName()[TestShared] = %v, want the slower instance (900ms), not the last one written", got)
	}

	// The total counts both packages' time: they are two different tests that
	// happen to share a name, and both seconds were really spent.
	if got, want := timings.LeafElapsed(), 910*time.Millisecond; got != want {
		t.Fatalf("LeafElapsed() = %v, want %v", got, want)
	}
}

// A parent's reported duration already contains its subtests, so adding every
// name would report the same wall-clock time several times over.
func TestTestBudgetTimingsCountASubtestOnlyOnce(t *testing.T) {
	timings := testbudget.NewTimings()
	timings.Record("govard/tests", "TestParent", 3*time.Second)
	timings.Record("govard/tests", "TestParent/case_a", 1*time.Second)
	timings.Record("govard/tests", "TestParent/case_b", 2*time.Second)

	if got, want := timings.LeafElapsed(), 3*time.Second; got != want {
		t.Fatalf("LeafElapsed() = %v, want %v (the subtests, not parent+subtests)", got, want)
	}

	leaves := timings.LeafByName()
	if len(leaves) != 2 {
		t.Fatalf("LeafByName() has %d entries %v, want only the two subtests", len(leaves), leaves)
	}
	if _, present := leaves["TestParent"]; present {
		t.Fatal("LeafByName() still lists the parent, whose time is its subtests' time")
	}
}

// The shipped budget file is the one artefact every run depends on, so its shape
// is a contract rather than a suggestion: it must parse, and it must not carry
// an allowance nobody wrote a reason for.
func TestTestTimeBudgetFileIsValidAndEveryOverrideIsJustified(t *testing.T) {
	path := filepath.Join("test-time-budget.yml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	budget, err := testbudget.Load(path, 1)
	if err != nil {
		t.Fatalf("Load(%s) error = %v", path, err)
	}

	for _, name := range []string{"unit", "integration"} {
		suite, err := budget.Suite(name)
		if err != nil {
			t.Fatalf("suite %q: %v", name, err)
		}
		if suite.Default <= 0 {
			t.Fatalf("suite %q has no default budget", name)
		}
		for test, override := range suite.Overrides {
			if strings.TrimSpace(override.Reason) == "" {
				t.Fatalf("suite %q: %s is allowed %v with no reason", name, test, time.Duration(override.Budget))
			}
			if override.Budget < suite.Default {
				t.Fatalf("suite %q: %s is allowed %v, which is tighter than the %v default and belongs in neither map",
					name, test, time.Duration(override.Budget), time.Duration(suite.Default))
			}
		}
	}
}
