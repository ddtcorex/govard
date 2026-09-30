package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/runtime"
	"govard/internal/verify"

	"github.com/pterm/pterm"
	"github.com/spf13/pflag"
)

// `--lint-jobs`, `--timeout` and `--checks` were bound into VerifyOpts and read
// by nothing: the three audit items hardcoded `--lint-jobs 4` and `--timeout
// auto|0` as literals, so the phase-3 argv was byte-identical with and without
// the flags (issue #472). These tests assert the captured argv, never the exit
// code: under a test binary the exec seam answers 0 whatever the flags say, so a
// wrong argv is invisible through the verdict.
//
// The items under test are the two phase-3 lint modes — P3-10 project scope,
// P3-11 diff scope — the P3-12 profiler, the module-scoped lint audits P3-13 and
// P3-14, the P3-15 lifecycle and the phase-5 re-lint P5-04; each makes one govard
// invocation, so captureItemArgvs' map value is the item's own argv.

// verifyAuditArgvsForTest is what each audit item must invoke under
// VerifyOpts{LintJobs: 8, Timeout: "25m"}. 8 is the spec's value and is
// deliberately above Magento 2's bound for `audit run --lint-jobs` (7, the size
// of its PHP matrix): the assertion is about the threaded value reaching the
// argv, not about the child accepting it — a real run would let `audit run`
// reject 8, which is the child's call to make, not verify's to swallow.
var verifyAuditArgvsForTest = map[string][]string{
	"P3-10": {"audit", "run", "--checks", "lint", "--scope", "project", "--mode", "auto", "--format", "json", "--lint-jobs", "8", "--timeout", "25m"},
	"P3-11": {"audit", "run", "--checks", "lint", "--scope", "diff", "--base", "origin/master", "--format", "json", "--lint-jobs", "8", "--timeout", "25m"},
	"P5-04": {"audit", "run", "--checks", "lint", "--no-lint-result-cache", "--lint-jobs", "8", "--timeout", "25m", "--format", "json"},
}

func TestVerifyHonoursLintJobsAndTimeout(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	argvs := captureItemArgvs(t, engine.Config{Framework: "magento2"}, verify.VerifyOpts{
		ProjectRoot: t.TempDir(),
		LintJobs:    8,
		Timeout:     "25m",
	})

	// Full argv, not a substring: `--lint-jobs 4 --lint-jobs 8` would satisfy a
	// containment check and still run four workers.
	for id, want := range verifyAuditArgvsForTest {
		if got := argvs[id]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s invoked\n got %v\nwant %v", id, got, want)
		}
	}
}

// The two module-scoped lint audits run lint too, so the two flags are as
// meaningful there as on P3-10/P3-11 — and these are the two slowest items on a
// large module. They resolve their target from the project tree, so they need
// the module fixture rather than captureItemArgvs' empty ProjectRoot (there they
// skip, which is why verifyAuditArgvsForTest cannot hold them).
//
// P3-14's `--path` is the standalone fixture this run creates under
// os.TempDir(), so its expected argv is built from the captured path and the
// path itself is pinned separately: a title may not promise `/tmp` when TMPDIR
// moves the directory.
func TestVerifyThreadsTheLintFlagsOnTheModuleLintAudits(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	const module = "DemoThreading"
	projectRoot, moduleRoot := magentoProjectWithAppCodeModule(t, module)
	cfg := engine.Config{Framework: "magento2"}

	t.Run("flags set", func(t *testing.T) {
		ev, argvs := captureAuditTargetItem(t, "P3-13", cfg, verify.VerifyOpts{
			ProjectRoot: projectRoot, LintJobs: 8, Timeout: "25m",
		}, nil)
		if len(argvs) != 1 {
			t.Fatalf("P3-13 made %d govard invocations, want 1", len(argvs))
		}
		want := []string{"audit", "run", "--checks", "lint", "--mode", "module_in_project", "--format", "json",
			"--path", moduleRoot, "--lint-jobs", "8", "--timeout", "25m"}
		if !reflect.DeepEqual(argvs[0], want) {
			t.Errorf("P3-13 invoked\n got %v\nwant %v", argvs[0], want)
		}
		if ev.SkipReason != "" {
			t.Errorf("P3-13 skipped (%q) on a project with a module", ev.SkipReason)
		}

		ev14, argvs14 := captureAuditTargetItem(t, "P3-14", cfg, verify.VerifyOpts{
			ProjectRoot: projectRoot, LintJobs: 8, Timeout: "25m",
		}, nil)
		if len(argvs14) != 1 {
			t.Fatalf("P3-14 made %d govard invocations, want 1", len(argvs14))
		}
		fixture := argvs14[0][indexOfArgvValue(argvs14[0], "--path")+1]
		want14 := []string{"audit", "run", "--checks", "lint", "--mode", "standalone", "--format", "json",
			"--path", fixture, "--lint-jobs", "8", "--timeout", "25m"}
		if !reflect.DeepEqual(argvs14[0], want14) {
			t.Errorf("P3-14 invoked\n got %v\nwant %v", argvs14[0], want14)
		}
		if ev14.SkipReason != "" {
			t.Errorf("P3-14 skipped (%q) on a project with a module", ev14.SkipReason)
		}
		wantPrefix := filepath.Join(os.TempDir(), "govard-audit-standalone")
		if filepath.Dir(fixture) != wantPrefix {
			t.Errorf("P3-14 audited %q, want a fixture directly under %q: the title names that directory, and TMPDIR must not move it", fixture, wantPrefix)
		}
	})

	// At verify's own defaults neither flag is a request: `--lint-jobs` stays out
	// so `audit run` keeps engine.AuditRunJobs(), and `--timeout auto` is
	// `audit run`'s own default, so threading it changes nothing the child does.
	t.Run("verify defaults", func(t *testing.T) {
		_, argvs := captureAuditTargetItem(t, "P3-13", cfg, verify.VerifyOpts{
			ProjectRoot: projectRoot, LintJobs: 4, Timeout: "auto",
		}, nil)
		if len(argvs) != 1 {
			t.Fatalf("P3-13 made %d govard invocations, want 1", len(argvs))
		}
		if hasArgvFlag(argvs[0], "--lint-jobs") {
			t.Errorf("P3-13 invoked %v, want no --lint-jobs at verify's default", argvs[0])
		}
		if !containsAdjacent(argvs[0], []string{"--timeout", "auto"}) {
			t.Errorf("P3-13 invoked %v, want --timeout auto, audit run's own default", argvs[0])
		}
		_, argvs14 := captureAuditTargetItem(t, "P3-14", cfg, verify.VerifyOpts{
			ProjectRoot: projectRoot, LintJobs: 4, Timeout: "auto",
		}, nil)
		if len(argvs14) != 1 {
			t.Fatalf("P3-14 made %d govard invocations, want 1", len(argvs14))
		}
		if hasArgvFlag(argvs14[0], "--lint-jobs") {
			t.Errorf("P3-14 invoked %v, want no --lint-jobs at verify's default", argvs14[0])
		}
		if !containsAdjacent(argvs14[0], []string{"--timeout", "auto"}) {
			t.Errorf("P3-14 invoked %v, want --timeout auto, audit run's own default", argvs14[0])
		}
	})
}

// indexOfArgvValue returns the position of value's flag in argv, or -1 when the
// flag is absent — so a missing `--path` fails the comparison that reads it
// rather than panicking on the next index.
func indexOfArgvValue(argv []string, flag string) int {
	for i, a := range argv {
		if a == flag {
			return i
		}
	}
	return -1
}

// An untouched flag means the operator asked for no count, not for verify's own
// literal. `verify --lint-jobs` defaults to 4 while `audit run --lint-jobs`
// defaults to engine.AuditRunJobs() — min(nproc,4) clamped 2-4, the host's
// auto-tuned worker count — so forwarding verify's 4 would override that tuning
// on every run. At its default the flag stays out of the argv and the child
// chooses.
//
// `--timeout`'s default is per item, because the item's own value differs: the
// phase-3 lint items run with `auto`, and P5-04 — the no-cache full re-lint —
// with 0, no deadline at all. Verify's flag default must not impose a deadline
// on the item that deliberately has none. A `--lint-jobs` below 1 is not a
// request either: verify leaves it out instead of forwarding a count the child
// would reject.
func TestVerifyDoesNotForwardTheLintJobsFlagDefault(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	// VerifyOpts{LintJobs: 4, Timeout: "auto"} is what a bare `govard verify`
	// binds: both are the flags' own defaults (internal/cmd/verify.go).
	argvs := captureItemArgvs(t, engine.Config{Framework: "magento2"}, verify.VerifyOpts{
		ProjectRoot: t.TempDir(),
		LintJobs:    4,
		Timeout:     "auto",
	})

	want := map[string][]string{
		"P3-10": {"audit", "run", "--checks", "lint", "--scope", "project", "--mode", "auto", "--format", "json", "--timeout", "auto"},
		"P3-11": {"audit", "run", "--checks", "lint", "--scope", "diff", "--base", "origin/master", "--format", "json", "--timeout", "auto"},
		"P5-04": {"audit", "run", "--checks", "lint", "--no-lint-result-cache", "--timeout", "0", "--format", "json"},
	}
	for id, expected := range want {
		if got := argvs[id]; !reflect.DeepEqual(got, expected) {
			t.Errorf("%s invoked\n got %v\nwant %v\nverify's default 4 must not override audit run's engine.AuditRunJobs(), and P5-04's own 0 must survive the untouched flag", id, got, expected)
		}
	}

	// A value below 1 is not a request either: `audit run` rejects a count that
	// cannot run, so verify leaves the choice to the child rather than forwarding
	// one (the reference documents this).
	unset := captureItemArgvs(t, engine.Config{Framework: "magento2"}, verify.VerifyOpts{
		ProjectRoot: t.TempDir(),
		LintJobs:    0,
		Timeout:     "auto",
	})
	for _, id := range []string{"P3-10", "P3-11", "P5-04"} {
		if argv := unset[id]; hasArgvFlag(argv, "--lint-jobs") {
			t.Errorf("%s invoked %v, want no --lint-jobs for a count below 1", id, argv)
		}
	}
}

// The resolved argv has to reach the artifact, because RunItem.Command is the
// item's Title (internal/verify/runner.go): a title cannot carry a value the run
// resolves, so the evidence excerpt is the only place the value that actually ran
// is recorded. Every item this task de-froze or wrapped has to show it.
func TestVerifyAuditArgvReachesTheEvidence(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	opts := verify.VerifyOpts{ProjectRoot: t.TempDir(), LintJobs: 8, Timeout: "25m"}
	// Every item whose evidence this task wrapped: the four whose titles lost a
	// resolved value (P3-10, P3-11, P3-12, P5-04) and P3-13, which needs the
	// module fixture below. P3-12 is the profiler item and needs no fixture — it
	// resolves nothing but cfg.Domain, defaulting to localhost — so it is asserted
	// here rather than reasoned about.
	for _, id := range []string{"P3-10", "P3-11", "P3-12", "P5-04"} {
		item, ok := findItem(id)
		if !ok {
			t.Fatalf("%s is missing from the registry", id)
		}
		var argvs [][]string
		verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
			argv := append([]string(nil), args...)
			argvs = append(argvs, argv)
			return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(argv, " ")}, true
		})

		ev := item.Run(context.Background(), engine.Config{Framework: "magento2", Domain: "sample.test"}, opts)
		verify.SetExecGovardFakeForTest(nil)

		if len(argvs) != 1 {
			t.Fatalf("%s made %d govard invocations, want 1", id, len(argvs))
		}
		line, _, _ := strings.Cut(ev.OutputExcerpt, "\n")
		if want := "argv: " + strings.Join(argvs[0], " "); line != want {
			t.Errorf("%s evidence names %q, want %q: the title no longer carries the resolved values, so the artifact must", id, line, want)
		}
	}

	// P3-13 resolves its module directory from the project tree, so it needs the
	// fixture the discovery walks; its evidence prefix is the same contract.
	const module = "DemoEvidence"
	projectRoot, _ := magentoProjectWithAppCodeModule(t, module)
	ev, argvs := captureAuditTargetItem(t, "P3-13", engine.Config{Framework: "magento2"}, verify.VerifyOpts{ProjectRoot: projectRoot}, nil)
	if len(argvs) != 1 {
		t.Fatalf("P3-13 made %d govard invocations, want 1", len(argvs))
	}
	line, _, _ := strings.Cut(ev.OutputExcerpt, "\n")
	if want := "argv: " + strings.Join(argvs[0], " "); line != want {
		t.Errorf("P3-13 evidence names %q, want %q", line, want)
	}
}

// A title is RunItem.Command (internal/verify/runner.go), so a flag a title names
// must be one the item always runs. `--allow-xdebug` is appended only when the
// run waived the guard, and P3-12/P3-13 named it unconditionally: the row
// described an argv that did not run — the defect this task removed from the
// `--lint-jobs`/`--timeout` titles, still on the two items it rewrote.
//
// The sweep is generic rather than a list of the two: any flag that appears in
// an item's argv only when the run opts in is conditional by construction, and
// naming one in the title is the same defect wherever it appears.
func TestConditionalFlagsStayOutOfTitles(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	cfg := engine.Config{Framework: "magento2"}

	base := captureItemArgvs(t, cfg, verify.VerifyOpts{ProjectRoot: t.TempDir()})
	waived := captureItemArgvs(t, cfg, verify.VerifyOpts{ProjectRoot: t.TempDir(), AllowXdebug: true})

	for id, argv := range base {
		item, ok := findItem(id)
		if !ok {
			continue
		}
		for _, flag := range conditionalFlagsForTest(argv, waived[id]) {
			if strings.Contains(item.Title, flag) {
				t.Errorf("%s names %s in its title, but that flag is only in the argv when the run opts in; the title is RunItem.Command and must not describe an argv that did not run\ntitle: %q\nbase argv: %v\nopted-in argv: %v",
					id, flag, item.Title, argv, waived[id])
			}
		}
	}
}

// conditionalFlagsForTest returns the flags an item's argv only carries when the
// run opts in — present in the opted-in argv, absent from the base one.
func conditionalFlagsForTest(base, waived []string) []string {
	var conditional []string
	for _, arg := range waived {
		if !strings.HasPrefix(arg, "--") || hasArgvFlag(base, arg) {
			continue
		}
		conditional = append(conditional, arg)
	}
	return conditional
}

// `--checks` was bound and ignored as well. A run that selected `integrity` must
// not execute the items whose subject is the lint or the profiler check, and the
// row must say why instead of vanishing — an absent row hides coverage. An item
// that declares no check at all is not check-specific: nothing about it belongs
// to one check, so it runs whatever the selection is.
//
// The declaration rule is what an item's *subject* verifies, not every check its
// argv happens to pass: P3-13/P3-14 audit a module with the lint check, so they
// declare `lint`; P3-15 runs the integrity audit and its own lifecycle over that
// session, so it declares `integrity` and is left out of a lint or profiler run.
func TestVerifyFiltersItemsByCheck(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(args, " ")}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	projectRoot := t.TempDir()
	cfg := engine.Config{Framework: "magento2"}

	// record replaces each named item's Run with a counter, so the test can tell
	// "the row says skipped" from "the item did not execute".
	record := func(ids ...string) map[string]*int {
		counts := make(map[string]*int, len(ids))
		for _, id := range ids {
			counts[id] = recordItemRun(t, id)
		}
		return counts
	}

	t.Run("integrity selection", func(t *testing.T) {
		runs := record("P3-07", "P3-10", "P3-12", "P3-13", "P3-14", "P3-15")

		res, err := verify.RunPhase(context.Background(), cfg, 3, verify.VerifyOpts{ProjectRoot: projectRoot, Checks: []string{"integrity"}})
		if err != nil {
			t.Fatalf("RunPhase: %v", err)
		}

		if *runs["P3-10"] != 0 || *runs["P3-12"] != 0 {
			t.Errorf("a --checks integrity run executed items it does not select: P3-10 ran %d time(s), P3-12 %d", *runs["P3-10"], *runs["P3-12"])
		}
		if *runs["P3-13"] != 0 || *runs["P3-14"] != 0 {
			t.Errorf("a --checks integrity run executed the module lint audits: P3-13 ran %d time(s), P3-14 %d", *runs["P3-13"], *runs["P3-14"])
		}
		if *runs["P3-15"] != 1 {
			t.Errorf("P3-15 ran %d time(s), want 1: its --checks integrity create run is the session factory its subject needs, not a check it verifies", *runs["P3-15"])
		}
		if *runs["P3-07"] != 1 {
			t.Errorf("P3-07 ran %d time(s), want 1: an item that declares no check is not check-specific and must run", *runs["P3-07"])
		}
		for _, id := range []string{"P3-10", "P3-12", "P3-13", "P3-14"} {
			row, ok := findRunItem(res, id)
			if !ok {
				t.Fatalf("%s left no row in the phase-3 report (%d rows); exclude it from running, not from the report", id, len(res.Items))
			}
			if !row.Skipped {
				t.Errorf("%s row Skipped = false, want true: %+v", id, *row)
			}
			if !strings.Contains(row.SkipReason, "--checks integrity") {
				t.Errorf("%s skip reason = %q, want it to name the selection that left the item out", id, row.SkipReason)
			}
		}
		if row, ok := findRunItem(res, "P3-07"); !ok {
			t.Fatal("P3-07 left no row in the phase-3 report")
		} else if row.Skipped {
			t.Errorf("P3-07 was skipped (%q) under a --checks selection it is not part of", row.SkipReason)
		}
		if row, ok := findRunItem(res, "P3-15"); !ok {
			t.Fatal("P3-15 left no row in the phase-3 report")
		} else if row.Skipped {
			t.Errorf("P3-15 was skipped (%q) under --checks integrity, which it declares", row.SkipReason)
		}
	})

	t.Run("lint selection", func(t *testing.T) {
		runs := record("P3-10", "P3-12", "P3-13", "P3-15")

		res, err := verify.RunPhase(context.Background(), cfg, 3, verify.VerifyOpts{ProjectRoot: projectRoot, Checks: []string{"lint"}})
		if err != nil {
			t.Fatalf("RunPhase: %v", err)
		}

		if *runs["P3-10"] != 1 || *runs["P3-13"] != 1 {
			t.Errorf("a --checks lint run must execute the lint items it selects: P3-10 ran %d time(s), P3-13 %d", *runs["P3-10"], *runs["P3-13"])
		}
		if *runs["P3-12"] != 0 {
			t.Errorf("a --checks lint run executed the profiler item %d time(s)", *runs["P3-12"])
		}
		if *runs["P3-15"] != 0 {
			t.Errorf("P3-15 ran %d time(s) under --checks lint: its subject is the integrity lifecycle and it builds its own session", *runs["P3-15"])
		}
		row, ok := findRunItem(res, "P3-12")
		if !ok {
			t.Fatal("P3-12 left no row in the phase-3 report")
		}
		if !row.Skipped || !strings.Contains(row.SkipReason, "--checks lint") {
			t.Errorf("P3-12 row = skipped %v reason %q, want it skipped by the --checks lint selection", row.Skipped, row.SkipReason)
		}
	})

	// P5-04 is the third lint-audit item, and phase 5 is where it lives. Plan
	// mode keeps the run free of side effects while the filter still decides
	// which rows are marked skipped.
	plan, err := verify.RunPhase(context.Background(), cfg, 5, verify.VerifyOpts{ProjectRoot: projectRoot, Plan: true, Checks: []string{"integrity"}})
	if err != nil {
		t.Fatalf("RunPhase plan: %v", err)
	}
	row, ok := findRunItem(plan, "P5-04")
	if !ok {
		t.Fatal("P5-04 left no row in the phase-5 report")
	}
	if !row.Skipped || !strings.Contains(row.SkipReason, "--checks integrity") {
		t.Errorf("P5-04 row = skipped %v reason %q, want it skipped by the --checks integrity selection", row.Skipped, row.SkipReason)
	}
}

// verifyLintJobsDefault/verifyTimeoutDefault are how the audit items tell an
// untouched flag from an explicit value (internal/verify/registry.go), and they
// duplicate the flag defaults the command registers
// (internal/cmd/verify.go). Nothing else checks the two agree: if `--lint-jobs`
// defaulted to 8 tomorrow, the sentinel would forward 8 and defeat the
// auto-tuning rule silently, and `--timeout` would fall back to a value the
// operator never had.
//
// Flag.DefValue is the declared default, where GetInt/GetString would report
// whatever the last test in this process left behind.
func TestVerifyFlagDefaultsMatchTheCommandTree(t *testing.T) {
	verifyCmd, _, err := cmd.RootCommandForTest().Find([]string{"verify"})
	if err != nil {
		t.Fatalf("find verify: %v", err)
	}
	lintJobs := verifyCmd.Flags().Lookup("lint-jobs")
	timeout := verifyCmd.Flags().Lookup("timeout")
	if lintJobs == nil || timeout == nil {
		t.Fatal("verify is missing --lint-jobs or --timeout")
	}

	wantLintJobs, wantTimeout := verify.FlagDefaultsForTest()
	if lintJobs.DefValue != strconv.Itoa(wantLintJobs) {
		t.Errorf("the registry's --lint-jobs sentinel is %d but the command's default is %q; they must agree or an untouched flag stops meaning \"no preference\"", wantLintJobs, lintJobs.DefValue)
	}
	if timeout.DefValue != wantTimeout {
		t.Errorf("the registry's --timeout sentinel is %q but the command's default is %q; they must agree or an untouched flag stops meaning \"no preference\"", wantTimeout, timeout.DefValue)
	}
}

// An unknown check name used to be ignored, and once the runner honoured
// `--checks` it became a silent green: `--checks lints` selected nothing that
// declares a check, every skipped row is excluded from Counts(), and the run
// reported `passed`. The name is therefore validated before any work starts,
// against the same three names `audit run --checks` accepts.
func TestVerifyRejectsAnUnknownCheckName(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	_, err := runVerifyCommandForTest(t, "verify", "--plan", "--project", t.TempDir(), "--checks", "lints")
	if err == nil {
		t.Fatal("verify --checks lints was accepted; a name no check implements must be a usage error, not a silently narrowed run")
	}
	if got := cli.Code(err); got != cli.CodeUsage {
		t.Errorf("verify --checks lints exited %d, want %d (usage): %v", got, cli.CodeUsage, err)
	}
	// The child's own wording, so the two commands cannot drift apart.
	if want := `audit check "lints" is not implemented`; !strings.Contains(err.Error(), want) {
		t.Errorf("verify --checks lints failed with %q, want it to carry %q", err.Error(), want)
	}
}

// The other half, and the trap in the fix: audit.NormalizeChecks resolves an
// empty slice to ["lint"], so validating with it unconditionally would narrow
// every bare `govard verify` to the lint items — a silent regression much larger
// than the typo it guards against. A bare run must select everything, and a valid
// name must not be rejected.
func TestVerifyDoesNotNarrowABareOrValidChecksRun(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	projectRoot := t.TempDir()

	stdout, err := runVerifyCommandForTest(t, "verify", "--plan", "--json", "--project", projectRoot)
	if err != nil {
		t.Fatalf("a bare verify --plan: %v", err)
	}
	var result struct {
		Items []struct {
			ID         string `json:"id"`
			Skipped    bool   `json:"skipped"`
			SkipReason string `json:"skip_reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("verify --json printed no readable report: %v\n%s", err, stdout)
	}
	if len(result.Items) == 0 {
		t.Fatal("verify --plan --json reported no items at all")
	}
	for _, item := range result.Items {
		if strings.Contains(item.SkipReason, "--checks ") {
			t.Errorf("%s was excluded by a --checks selection (%q) on a bare run: an empty --checks is not a request, and NormalizeChecks would have resolved it to lint", item.ID, item.SkipReason)
		}
	}

	if _, err := runVerifyCommandForTest(t, "verify", "--plan", "--project", projectRoot, "--checks", "lint,profiler"); err != nil {
		t.Fatalf("verify --checks lint,profiler was rejected: %v", err)
	}
}

// runVerifyCommandForTest drives the real command tree, so the assertions above
// are about shipped behaviour rather than a copy of the rule.
//
// The verify flags and its writers live on a process-wide command object, so
// every flag is reset to its declared default before each case and again on
// cleanup: pflag appends to a slice flag on every Set after the first (Replace is
// the one operation that clears it), and a bool such as --plan would otherwise
// stay set for whichever test runs next — which is how this file's --plan case
// turned the JSON-contract test's run into a dry run that cannot fail.
func runVerifyCommandForTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := cmd.RootCommandForTest()
	verifyCmd, _, err := root.Find([]string{"verify"})
	if err != nil {
		t.Fatalf("find verify: %v", err)
	}

	reset := func() {
		verifyCmd.Flags().VisitAll(func(flag *pflag.Flag) {
			// Reported rather than fatal: this also runs from t.Cleanup, and a
			// Goexit there would skip the cleanups that follow it.
			if slice, ok := flag.Value.(pflag.SliceValue); ok {
				if err := slice.Replace([]string{}); err != nil {
					t.Errorf("reset --%s: %v", flag.Name, err)
				}
			} else if err := flag.Value.Set(flag.DefValue); err != nil {
				t.Errorf("reset --%s: %v", flag.Name, err)
			}
			flag.Changed = false
		})
		root.SetOut(nil)
		root.SetErr(nil)
	}
	reset()
	t.Cleanup(reset)

	// `verify` is not in alwaysRunnableNames, so the root PersistentPreRunE gates
	// on the container-runtime probe before RunE runs at all. Without this stub
	// the assertions below would be about the host instead of the flag
	// validation: with the docker lookup masked (`runtime.StubLookPathForTest`)
	// the typo case answers exit 3, `cannot run "govard verify": missing
	// capability "docker"`, where it must answer 2. Only this helper's tests take
	// the stub — the package's other command-level tests carry the same host
	// dependency today, and widening them is not this task's change.
	restoreCapabilities := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	t.Cleanup(restoreCapabilities)

	// The human table renders through pterm's process-global default output, so
	// discard it for this case and restore production's os.Stdout after: this test
	// then neither depends on nor worsens that global. os.Stdout, not nil —
	// pterm's default writer is initialised to os.Stdout and SetDefaultOutput has
	// no inverse, which is why the cleanups in verify_all_phases_test.go and
	// verify_json_stdout_test.go name os.Stdout too.
	pterm.SetDefaultOutput(io.Discard)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	// Execute first: `return stdout.String(), root.Execute()` would read the
	// buffer before the command wrote to it.
	err = root.Execute()
	return stdout.String(), err
}

// recordItemRun replaces one registry item's Run with a counter, so a test can
// tell "the row says skipped" from "the item did not execute". The registry is a
// package-level slice; RunPhase composes from it per call, so the replacement is
// what that call sees. Restored on cleanup, like the other registry fixtures in
// this suite.
func recordItemRun(t *testing.T, id string) *int {
	t.Helper()
	for i := range verify.Registry {
		if verify.Registry[i].ID != id {
			continue
		}
		calls := 0
		orig := verify.Registry[i].Run
		verify.Registry[i].Run = func(context.Context, engine.Config, verify.VerifyOpts) verify.Evidence {
			calls++
			return verify.Evidence{ExitCode: 0, OutputExcerpt: "recorded"}
		}
		t.Cleanup(func() { verify.Registry[i].Run = orig })
		return &calls
	}
	t.Fatalf("registry has no %s", id)
	return nil
}

// findRunItem returns the row a run reported for one item id.
func findRunItem(res verify.RunResult, id string) (*verify.RunItem, bool) {
	for i := range res.Items {
		if res.Items[i].ID == id {
			return &res.Items[i], true
		}
	}
	return nil, false
}
