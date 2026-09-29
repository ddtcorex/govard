// Package testbudget turns "how long may one test take?" into a machine-checked
// rule instead of a habit.
//
// The problem it answers is narrow and concrete: govard's suite is enormous and
// almost entirely cheap (thousands of tests run in under 10ms), yet its wall
// clock is dominated by a small tail that nobody notices until the whole run
// takes minutes. Without a budget that tail grows silently, because a test that
// goes from 40ms to 4s still passes and nobody looks at the clock.
//
// The budget is checked against the `Elapsed` field that `go test -json` already
// reports for every test, so enforcing it costs no extra wall time: the gate is
// the test run, not a second run that measures the first.
package testbudget

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that reads as a Go duration string ("250ms", "2s")
// in the budget file, which is what a human editing it actually wants to type.
// It is a parse-only type: the budget file is hand-maintained, so nothing needs
// to write one back out.
type Duration time.Duration

// UnmarshalYAML parses a Go duration string. A bare number is rejected on
// purpose: "2" is ambiguous between 2ns and 2s, and silently picking one of them
// would set a budget the author did not intend.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("duration must be a quoted string like \"2s\": %w", err)
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", raw, err)
	}
	if parsed <= 0 {
		return fmt.Errorf("duration %q must be greater than zero", raw)
	}
	*d = Duration(parsed)
	return nil
}

// Override is a test that is allowed to exceed the suite default. Reason is
// mandatory: an allowlist entry nobody can justify is indistinguishable from a
// test that simply got slow, which is the exact failure this package exists to
// prevent.
type Override struct {
	Budget Duration `yaml:"budget"`
	Reason string   `yaml:"reason"`
}

// Suite is the budget for one test suite (one `go test` invocation).
type Suite struct {
	Default   Duration            `yaml:"default"`
	Overrides map[string]Override `yaml:"overrides"`
}

// Budget is every suite's budget, keyed by suite name.
type Budget struct {
	Suites map[string]Suite `yaml:"suites"`
}

// Violation is one test that ran longer than its budget allows.
type Violation struct {
	Test   string
	Actual time.Duration
	Budget time.Duration
	// Reason is the justification from the override that granted the budget. It
	// is empty for a test held to the suite default.
	Reason string
	// FromOverride reports whether this test has an override entry at all, which
	// separates "your budget is too tight" from "this test ignored its budget".
	FromOverride bool
}

// Load reads a budget file. scale multiplies every budget, so a slower machine
// (a shared CI runner) can widen the whole file through one environment variable
// without editing it and without the file drifting away from the defaults.
func Load(path string, scale float64) (*Budget, error) {
	if scale <= 0 {
		return nil, fmt.Errorf("budget scale must be greater than zero, got %v", scale)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read budget file: %w", err)
	}

	var budget Budget
	if err := yaml.Unmarshal(data, &budget); err != nil {
		return nil, fmt.Errorf("parse budget file %s: %w", path, err)
	}
	if len(budget.Suites) == 0 {
		return nil, fmt.Errorf("budget file %s declares no suites", path)
	}

	for name, suite := range budget.Suites {
		if suite.Default <= 0 {
			return nil, fmt.Errorf("suite %q has no positive default budget", name)
		}
		for test, override := range suite.Overrides {
			if override.Budget <= 0 {
				return nil, fmt.Errorf("suite %q: override for %s has no positive budget", name, test)
			}
			if strings.TrimSpace(override.Reason) == "" {
				return nil, fmt.Errorf(
					"suite %q: override for %s has no reason; every allowance must say why the test is slow, "+
						"otherwise the allowlist rots into a list of tests nobody looked at", name, test)
			}
		}
	}

	if scale != 1 {
		for name, suite := range budget.Suites {
			suite.Default = Duration(float64(suite.Default) * scale)
			for test, override := range suite.Overrides {
				override.Budget = Duration(float64(override.Budget) * scale)
				suite.Overrides[test] = override
			}
			budget.Suites[name] = suite
		}
	}

	return &budget, nil
}

// Suite returns the budget for one suite name.
func (b *Budget) Suite(name string) (Suite, error) {
	suite, ok := b.Suites[name]
	if !ok {
		return Suite{}, fmt.Errorf("budget file declares no suite %q (known: %s)", name, strings.Join(b.SuiteNames(), ", "))
	}
	return suite, nil
}

// SuiteNames lists the declared suites, sorted, for error messages and tooling.
func (b *Budget) SuiteNames() []string {
	names := make([]string, 0, len(b.Suites))
	for name := range b.Suites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LimitFor returns the budget a test is held to, plus the reason when that
// budget came from an override rather than the suite default.
func (s Suite) LimitFor(test string) (time.Duration, string) {
	if override, ok := s.Overrides[test]; ok {
		return time.Duration(override.Budget), override.Reason
	}
	return time.Duration(s.Default), ""
}

// Evaluate returns every test that ran longer than its budget, slowest first.
func (s Suite) Evaluate(elapsed map[string]time.Duration) []Violation {
	violations := make([]Violation, 0)
	for test, actual := range elapsed {
		limit, reason := s.LimitFor(test)
		if actual > limit {
			violations = append(violations, Violation{
				Test:         test,
				Actual:       actual,
				Budget:       limit,
				Reason:       reason,
				FromOverride: reason != "",
			})
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Actual != violations[j].Actual {
			return violations[i].Actual > violations[j].Actual
		}
		return violations[i].Test < violations[j].Test
	})
	return violations
}

// StaleOverrides returns override entries whose test never ran. They are not
// errors -- a suite that is only partly run still has a valid budget -- but they
// are how an allowlist dies quietly: a test is renamed or deleted, its
// allowance stays, and the next person to write a slow test finds the precedent
// waiting for them.
func (s Suite) StaleOverrides(ran map[string]bool) []string {
	stale := make([]string, 0)
	for test := range s.Overrides {
		if !ran[test] {
			stale = append(stale, test)
		}
	}
	sort.Strings(stale)
	return stale
}

// Timings accumulates the per-test durations of a run that may span several
// packages, which is what `go test ./...` does.
//
// A budget is written against a test *name*, but two packages are free to
// declare a test with the same name. Recording by name alone would silently keep
// only one of them -- the one way a budget gate can quietly fail to notice a slow
// test -- so measurements are kept package-qualified internally and merged by
// name on the way out, keeping the slowest instance.
type Timings struct {
	qualified map[string]time.Duration
	byName    map[string]time.Duration
}

// NewTimings returns an empty accumulator.
func NewTimings() *Timings {
	return &Timings{
		qualified: map[string]time.Duration{},
		byName:    map[string]time.Duration{},
	}
}

// Record adds one completed test's duration.
func (t *Timings) Record(pkg, test string, elapsed time.Duration) {
	key := test
	if pkg != "" {
		key = pkg + "\x00" + test
	}
	t.qualified[key] = elapsed
	if previous, seen := t.byName[test]; !seen || elapsed > previous {
		t.byName[test] = elapsed
	}
}

// ByName returns the duration budgeted for each test name: the slowest instance
// of that name across the run.
func (t *Timings) ByName() map[string]time.Duration {
	out := make(map[string]time.Duration, len(t.byName))
	for name, elapsed := range t.byName {
		out[name] = elapsed
	}
	return out
}

// LeafElapsed totals the run without counting a second twice.
//
// A parent test's reported duration already contains its subtests, so summing
// every name would count the same wall-clock time many times over. Only tests
// that ran no subtests of their own contribute.
func (t *Timings) LeafElapsed() time.Duration {
	var total time.Duration
	for key, elapsed := range t.qualified {
		if isLeaf(key, t.qualified) {
			total += elapsed
		}
	}
	return total
}

// LeafByName is ByName restricted to tests that ran no subtests of their own,
// which is the honest set to rank when reporting the slowest tests: a parent is
// already the sum of its children, so listing both would show the same
// wall-clock time twice.
func (t *Timings) LeafByName() map[string]time.Duration {
	leaves := make(map[string]time.Duration, len(t.qualified))
	for key, elapsed := range t.qualified {
		if !isLeaf(key, t.qualified) {
			continue
		}
		_, test, _ := strings.Cut(key, "\x00")
		if previous, seen := leaves[test]; !seen || elapsed > previous {
			leaves[test] = elapsed
		}
	}
	return leaves
}

func isLeaf(qualified string, all map[string]time.Duration) bool {
	prefix := qualified + "/"
	for other := range all {
		if strings.HasPrefix(other, prefix) {
			return false
		}
	}
	return true
}
