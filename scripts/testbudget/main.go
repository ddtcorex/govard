// Command testbudget runs `go test -json` and fails the run when a single test
// outlasts the budget its suite allows.
//
// It is a wrapper rather than a separate measurement pass on purpose: the budget
// is checked against the Elapsed field that `go test -json` already reports, so
// enforcing it adds no wall time to the suite. The alternative — run the tests,
// then run them again under a proctor — doubles the slowest part of the build
// for a number a developer has to look at anyway.
//
// Usage:
//
//	testbudget -suite unit -- go test ./tests -short -json
//
// Everything after `--` is handed to go test verbatim.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"govard/internal/testbudget"
)

// scaleEnv widens or tightens every budget for a slower or faster machine
// without editing the budget file. A shared CI runner is routinely slower than a
// developer laptop, and a budget that only holds on one of them trains people to
// ignore the gate.
const scaleEnv = "GOVARD_TEST_TIME_SCALE"

// defaultBudgetFile is resolved from the working directory, which is the module
// root for every make target that calls this.
const defaultBudgetFile = "tests/test-time-budget.yml"

// event is one line of `go test -json` output. Only the fields this gate needs
// are declared; the rest of the line is preserved by re-encoding Output.
type event struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

func main() {
	suite := flag.String("suite", "", "budget suite to enforce (required)")
	budgetPath := flag.String("budget", defaultBudgetFile, "path to the budget file")
	scale := flag.String("scale", scaleFromEnv(), "multiply every budget by this factor (default 1, or $"+scaleEnv+")")
	quiet := flag.Bool("quiet", false, "suppress per-test chatter, keeping failures, violations and the summary")
	stale := flag.Bool("stale", false, "report budget overrides whose test did not run (enable on full-suite runs only)")
	flag.Parse()

	if *suite == "" {
		fail("testbudget: -suite is required (known suites are declared in %s)", *budgetPath)
	}

	args := flag.Args()
	if len(args) == 0 {
		fail("testbudget: no command to run; put the go test command after --")
	}

	multiplier, err := strconv.ParseFloat(*scale, 64)
	if err != nil {
		fail("testbudget: -scale must be a positive number, got %q", *scale)
	}

	budget, err := testbudget.Load(*budgetPath, multiplier)
	if err != nil {
		fail("testbudget: %v", err)
	}
	budgetSuite, err := budget.Suite(*suite)
	if err != nil {
		fail("testbudget: %v", err)
	}

	report, runErr := run(args, budgetSuite, *quiet, *stale)
	report.print(os.Stdout)

	// A budget breach is a build failure even when every test passed: a suite
	// that quietly doubled in wall clock is the regression this exists to catch.
	if len(report.violations) > 0 {
		if runErr == nil {
			os.Exit(1)
		}
		os.Exit(exitCodeOf(runErr))
	}
	if runErr != nil {
		os.Exit(exitCodeOf(runErr))
	}
}

// result is everything one run produced.
type result struct {
	violations []testbudget.Violation
	stale      []string
	testCount  int
	failCount  int
	skipCount  int
	total      time.Duration
	slowest    []testbudget.Violation
	ran        map[string]bool
}

// slowestToKeep bounds the "slowest tests" table. It is printed on every run
// because it is the part that tells a developer where the next slow test is
// coming from, and a gate nobody consults is a gate people route around.
const slowestToKeep = 10

func run(args []string, budgetSuite testbudget.Suite, quiet, reportStale bool) (result, error) {
	cmd := exec.Command(args[0], args[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result{}, fmt.Errorf("pipe go test output: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return result{}, fmt.Errorf("start %s: %w", args[0], err)
	}

	report := result{ran: map[string]bool{}}
	timings := testbudget.NewTimings()
	scanner := bufio.NewScanner(stdout)
	// go test output lines are short, but a failing test can print a stack trace
	// in one Output event; give the scanner room rather than dying on it.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			// Not an event: a build failure printed before the test binary ever
			// started, or a stray write to stdout. Pass it through untouched so
			// the real error is not swallowed by a JSON gate.
			fmt.Fprintf(os.Stderr, "%s\n", line)
			continue
		}

		switch e.Action {
		case "output":
			// go test already emits `=== RUN` and `--- PASS` for every test in
			// JSON mode, so those lines are passed straight through -- printing
			// a second per-test line here would just duplicate them. Quiet mode
			// drops them and keeps only what a failure needs.
			if quiet && isRoutineTestOutput(e.Output) {
				continue
			}
			fmt.Fprint(os.Stdout, e.Output)
		case "pass", "fail", "skip":
			if e.Test == "" {
				continue // package-level result, not a test
			}
			duration := time.Duration(e.Elapsed * float64(time.Second))
			timings.Record(e.Package, e.Test, duration)
			report.ran[e.Test] = true
			report.testCount++

			switch e.Action {
			case "fail":
				report.failCount++
			case "skip":
				report.skipCount++
			}
		}
	}

	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if scanErr != nil {
		return report, fmt.Errorf("read go test output: %w", scanErr)
	}
	if waitErr != nil {
		return report, waitErr
	}

	byName := timings.ByName()
	report.total = timings.LeafElapsed()
	report.violations = budgetSuite.Evaluate(byName)
	report.slowest = slowest(timings.LeafByName(), budgetSuite, slowestToKeep)
	if reportStale {
		report.stale = budgetSuite.StaleOverrides(report.ran)
	}
	return report, nil
}

// isRoutineTestOutput recognises the per-test bookkeeping go test prints for
// every single test, which is all a passing run is made of. `--- FAIL` is
// deliberately absent: a failure is the one line quiet mode must never eat.
func isRoutineTestOutput(output string) bool {
	trimmed := strings.TrimSpace(output)
	// Subtest lines arrive indented; trimming puts them on the same footing as
	// their top-level counterparts.
	return strings.HasPrefix(trimmed, "=== RUN") ||
		strings.HasPrefix(trimmed, "--- PASS") ||
		strings.HasPrefix(trimmed, "--- SKIP") ||
		strings.HasPrefix(trimmed, "=== PAUSE") ||
		strings.HasPrefix(trimmed, "=== CONT") ||
		strings.HasPrefix(trimmed, "PASS") ||
		strings.HasPrefix(trimmed, "ok  ")
}

// slowest ranks the run's leaf tests against their own budgets, so the table a
// developer reads on every run already shows which ones are close to the line.
func slowest(elapsed map[string]time.Duration, suite testbudget.Suite, limit int) []testbudget.Violation {
	var all []testbudget.Violation
	for name, duration := range elapsed {
		budget, reason := suite.LimitFor(name)
		all = append(all, testbudget.Violation{
			Test: name, Actual: duration, Budget: budget, Reason: reason, FromOverride: reason != "",
		})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Actual > all[j].Actual })
	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

func (r result) print(w io.Writer) {
	fmt.Fprintf(w, "\n%d tests, %d failed, %d skipped, %.1fs of test time\n",
		r.testCount, r.failCount, r.skipCount, r.total.Seconds())

	if len(r.slowest) > 0 {
		fmt.Fprintf(w, "\nslowest tests:\n")
		for _, s := range r.slowest {
			note := ""
			if s.FromOverride {
				note = "  (allowlisted)"
			}
			fmt.Fprintf(w, "  %8.3fs  budget %-8s  %s%s\n", s.Actual.Seconds(), s.Budget, s.Test, note)
		}
	}

	if len(r.stale) > 0 {
		fmt.Fprintf(w, "\nstale budget overrides (these tests did not run — delete them or fix the name):\n")
		for _, name := range r.stale {
			fmt.Fprintf(w, "  %s\n", name)
		}
	}

	if len(r.violations) == 0 {
		return
	}

	fmt.Fprintf(w, "\n%d test(s) exceeded their time budget:\n", len(r.violations))
	for _, v := range r.violations {
		fmt.Fprintf(w, "  %-64s %8.3fs  (budget %s)\n", v.Test, v.Actual.Seconds(), v.Budget)
		if v.Reason != "" {
			fmt.Fprintf(w, "      allowance: %s\n", v.Reason)
			continue
		}
		fmt.Fprintf(w, "      no allowance on file. Either make the test faster, or — if the slowness is\n")
		fmt.Fprintf(w, "      intrinsic to what it covers — add an entry with a reason to %s\n", defaultBudgetFile)
	}
}

// scaleFromEnv reads the headroom multiplier, treating "unset" as "no scaling"
// rather than as a parse error -- an unset variable is the normal case, and it
// must not need a default in the environment to work.
func scaleFromEnv() string {
	if value := strings.TrimSpace(os.Getenv(scaleEnv)); value != "" {
		return value
	}
	return "1"
}

// exitCodeOf preserves the go test exit code so the gate never masks a red suite
// with its own status.
func exitCodeOf(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
