package tests

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// A verify row's verdict must describe what ran. These tests pin the rows that
// used to report a pass (or a permanent red) for work they never did.

// recordingExec installs an exec fake that records every argv and answers with
// reply (nil means exit 0). It is restored on cleanup.
func recordingExec(t *testing.T, reply func(args []string) verify.Evidence) *[][]string {
	t.Helper()
	var argvs [][]string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		argvs = append(argvs, append([]string(nil), args...))
		if reply != nil {
			return reply(args), true
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(args, " ")}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return &argvs
}

func runRow(t *testing.T, id string, cfg engine.Config, opts verify.VerifyOpts) verify.Evidence {
	t.Helper()
	item, ok := findItem(id)
	if !ok {
		t.Fatalf("%s is missing from the registry", id)
	}
	return item.Run(context.Background(), cfg, opts)
}

func argvJoined(argvs [][]string) []string {
	out := make([]string, 0, len(argvs))
	for _, a := range argvs {
		out = append(out, strings.Join(a, " "))
	}
	return out
}

func TestP106SkipsWhenNoLockFile(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := recordingExec(t, nil)

	ev := runRow(t, "P1-06", engine.Config{}, verify.VerifyOpts{ProjectRoot: t.TempDir()})

	if !ev.Skipped {
		t.Fatalf("P1-06 with no lock file: skipped = false, evidence %+v", ev)
	}
	if !strings.Contains(ev.SkipReason, "govard lock generate") {
		t.Fatalf("skip reason %q must name `govard lock generate`", ev.SkipReason)
	}
	if len(*argvs) != 0 {
		t.Fatalf("P1-06 ran %v with no lock file; a verify phase must not run lock commands against nothing or write a tracked file", argvJoined(*argvs))
	}
}

func TestP106StaysRedForAPresentInconsistentLock(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(engine.LockFilePath(root), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	argvs := recordingExec(t, func(args []string) verify.Evidence {
		if strings.Join(args, " ") == "lock check" {
			return verify.Evidence{ExitCode: 1, OutputExcerpt: "drift"}
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "diff output"}
	})

	ev := runRow(t, "P1-06", engine.Config{}, verify.VerifyOpts{ProjectRoot: root})

	if ev.Skipped || ev.ExitCode != 1 {
		t.Fatalf("P1-06 with an inconsistent lock: skipped=%v exit=%d, want red", ev.Skipped, ev.ExitCode)
	}
	if got := argvJoined(*argvs); !reflect.DeepEqual(got, []string{"lock check", "lock diff"}) {
		t.Fatalf("P1-06 ran %v, want check then diff", got)
	}
}

func TestP309SkipsWhenFrontendSyncOff(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := recordingExec(t, nil)

	off := engine.Config{Framework: "magento2"}
	res, err := verify.RunPhase(context.Background(), off, 3, verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	row, ok := findRunItem(res, "P3-09")
	if !ok {
		t.Fatal("P3-09 left no row")
	}
	if !row.Skipped || !strings.Contains(row.SkipReason, "frontend_sync") {
		t.Fatalf("P3-09 with frontend_sync off: skipped=%v reason %q, want a skip naming stack.features.frontend_sync", row.Skipped, row.SkipReason)
	}
	for _, a := range argvJoined(*argvs) {
		if strings.HasPrefix(a, "frontend") {
			t.Fatalf("P3-09 ran %q although frontend_sync is off", a)
		}
	}

	var on engine.Config
	on.Framework = "magento2"
	on.Stack.Features.FrontendSync = true
	res, err = verify.RunPhase(context.Background(), on, 3, verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	if row, _ := findRunItem(res, "P3-09"); row == nil || row.Skipped {
		t.Fatalf("P3-09 with frontend_sync on must run, got %+v", row)
	}

	// A non-Magento project keeps the framework reason.
	res, err = verify.RunPhase(context.Background(), engine.Config{Framework: "laravel"}, 3, verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase: %v", err)
	}
	if row, _ := findRunItem(res, "P3-09"); row == nil || !row.Skipped || !strings.Contains(row.SkipReason, "framework") {
		t.Fatalf("P3-09 on laravel = %+v, want a framework-gate skip", row)
	}
}

func TestP312SkipsWithoutDomain(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := recordingExec(t, nil)

	ev := runRow(t, "P3-12", engine.Config{}, verify.VerifyOpts{ProjectRoot: t.TempDir()})

	if !ev.Skipped || !strings.HasPrefix(ev.SkipReason, "no domain configured:") {
		t.Fatalf("P3-12 without a domain: skipped=%v reason %q", ev.Skipped, ev.SkipReason)
	}
	if len(*argvs) != 0 {
		t.Fatalf("P3-12 audited a guessed host: %v", argvJoined(*argvs))
	}

	argvs = recordingExec(t, nil)
	ev = runRow(t, "P3-12", engine.Config{Domain: "shop.test"}, verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if ev.Skipped || len(*argvs) != 1 || !strings.Contains(strings.Join((*argvs)[0], " "), "https://shop.test/") {
		t.Fatalf("P3-12 with a domain: skipped=%v argv %v", ev.Skipped, *argvs)
	}
}

func TestP315RedOnCreateExit3And4(t *testing.T) {
	for _, tc := range []struct {
		name    string
		exit    int
		excerpt string
	}{
		{"capability missing with ids", 3, auditSessionRunExcerpt},
		{"configuration error with ids", 4, auditSessionRunExcerpt},
		{"failure with no JSON", 1, "boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			argvs := installAuditSessionFakeExiting(t, tc.excerpt, tc.exit)

			ev := runAuditSessionItem(t)

			if ev.Skipped || ev.ExitCode != tc.exit {
				t.Fatalf("P3-15 creating run exit %d: skipped=%v exit=%d, want red with that exit (reason %q)", tc.exit, ev.Skipped, ev.ExitCode, ev.SkipReason)
			}
			if len(*argvs) != 1 {
				t.Fatalf("P3-15 went on after a failed creating run: %v", argvJoined(*argvs))
			}
		})
	}

	t.Run("exit 0 without ids is a skip", func(t *testing.T) {
		t.Setenv("GOVARD_HOME_DIR", t.TempDir())
		installAuditSessionFakeExiting(t, "no json here", 0)
		ev := runAuditSessionItem(t)
		if !ev.Skipped {
			t.Fatalf("P3-15 exit 0 and no ids must skip, got %+v", ev)
		}
	})

	t.Run("findings exit 1 with ids continues", func(t *testing.T) {
		t.Setenv("GOVARD_HOME_DIR", t.TempDir())
		argvs := installAuditSessionFakeExiting(t, auditSessionRunExcerpt, 1)
		ev := runAuditSessionItem(t)
		if ev.Skipped || len(*argvs) != 4 {
			t.Fatalf("findings verdict must reach the lifecycle: skipped=%v argvs=%v", ev.Skipped, argvJoined(*argvs))
		}
	})
}

// fakeSnapshotCreate answers `snapshot create` by running create against the
// real snapshot store so P4-08 sees a before/after difference.
func fakeSnapshotCreate(t *testing.T, root string, create func()) *[][]string {
	t.Helper()
	return recordingExec(t, func(args []string) verify.Evidence {
		if strings.Join(args, " ") == "snapshot create" && create != nil {
			create()
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(args, " ")}
	})
}

func TestP408RecordsTheNewSnapshotNotAnOlderOne(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	// An older, usable snapshot sits in the store; the newer one is created by
	// the row. LatestSnapshotName would pick by CreatedAt, so make the OLD one
	// the newest by timestamp to prove the row records what it created.
	makeSnapshot(t, root, "older", "2030-01-01T00:00:00Z")
	fakeSnapshotCreate(t, root, func() { makeSnapshot(t, root, "created-now", "2026-01-01T00:00:00Z") })

	ev := runRow(t, "P4-08", engine.Config{}, verify.VerifyOpts{ProjectRoot: root})

	if ev.ExitCode != 0 || !reflect.DeepEqual(ev.Artifacts, []string{"created-now"}) {
		t.Fatalf("P4-08 = exit %d artifacts %v, want [created-now]", ev.ExitCode, ev.Artifacts)
	}
}

func TestP408FailsWhenCreatedSnapshotUnusable(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "older", "2026-01-01T00:00:00Z")
	// The created snapshot is a bare directory: no metadata, no dump.
	fakeSnapshotCreate(t, root, func() {
		if err := os.MkdirAll(filepath.Join(engine.SnapshotRoot(root), "broken"), 0o755); err != nil {
			t.Fatal(err)
		}
	})

	ev := runRow(t, "P4-08", engine.Config{}, verify.VerifyOpts{ProjectRoot: root})

	if ev.ExitCode == 0 || len(ev.Artifacts) != 0 {
		t.Fatalf("P4-08 with an unusable new snapshot = exit %d artifacts %v, want red and no artifact (the older usable one must not stand in)", ev.ExitCode, ev.Artifacts)
	}
}

func TestP408FailsWhenNoNewSnapshotAppears(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "older", "2026-01-01T00:00:00Z")
	fakeSnapshotCreate(t, root, nil)

	ev := runRow(t, "P4-08", engine.Config{}, verify.VerifyOpts{ProjectRoot: root})

	if ev.ExitCode == 0 || len(ev.Artifacts) != 0 {
		t.Fatalf("P4-08 when create added nothing = exit %d artifacts %v, want red", ev.ExitCode, ev.Artifacts)
	}
}

func TestP408FailsWhenSeveralSnapshotsAppear(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	fakeSnapshotCreate(t, root, func() {
		makeSnapshot(t, root, "a", "2026-01-01T00:00:00Z")
		makeSnapshot(t, root, "b", "2026-01-02T00:00:00Z")
	})

	ev := runRow(t, "P4-08", engine.Config{}, verify.VerifyOpts{ProjectRoot: root})

	if ev.ExitCode == 0 || len(ev.Artifacts) != 0 {
		t.Fatalf("P4-08 with two new snapshots = exit %d artifacts %v, want red: which one is ours is unknowable", ev.ExitCode, ev.Artifacts)
	}
}

func TestP502FailsWhenDownVolumesFails(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := recordingExec(t, func(args []string) verify.Evidence {
		if strings.Join(args, " ") == "env down -v" {
			return verify.Evidence{ExitCode: 1, OutputExcerpt: "cannot remove volume"}
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "up ok"}
	})

	ev := runRow(t, "P5-02", engine.Config{}, verify.VerifyOpts{ProjectRoot: t.TempDir()})

	if ev.ExitCode != 1 || !strings.Contains(ev.OutputExcerpt, "cannot remove volume") {
		t.Fatalf("P5-02 = exit %d %q, want the failed `env down -v`", ev.ExitCode, ev.OutputExcerpt)
	}
	if got := argvJoined(*argvs); !reflect.DeepEqual(got, []string{"env down -v"}) {
		t.Fatalf("P5-02 ran %v after a failed `env down -v`; bringing the environment up proves nothing about the volume", got)
	}

	argvs = recordingExec(t, nil)
	ev = runRow(t, "P5-02", engine.Config{}, verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if ev.ExitCode != 0 || !reflect.DeepEqual(argvJoined(*argvs), []string{"env down -v", "env up"}) {
		t.Fatalf("P5-02 happy path = exit %d argvs %v", ev.ExitCode, argvJoined(*argvs))
	}
}

// acceptedCompoundTitles names every title that joins steps with `->`, `+`,
// `x4`, `/` or `OR`, and the number of steps the row must actually issue when
// every child succeeds. A new compound title has to be added here, which is
// the point: a title is a claim, and it is checked against the argv count.
//
// minCalls is the lower bound of exec calls under an all-green fake. P1-06 is
// listed with 1 because its diff step runs only when the check fails.
var acceptedCompoundTitles = map[string]int{
	"P1-06": 1, // "govard lock check (+ lock diff on failure)": diff only on a failed check
	"P2-01": 3,
	"P2-07": 3,
	"P3-15": 4,
	"P4-08": 2,
	"P5-02": 2,
}

// slashToken matches a whitespace-delimited word of two letter-only parts joined
// by one slash, the way a title spells `pull/push`.
var slashToken = regexp.MustCompile(`(^|\s)[A-Za-z-]+/[A-Za-z-]+($|\s)`)

// titleSlashTokensThatArePaths are slash tokens that are one path, not two steps.
var titleSlashTokensThatArePaths = map[string]bool{"web/tailwind": true}

func TestEveryTitleMatchesItsArgvCount(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	markers := []string{" -> ", " + ", "(+ ", " x4", " / ", " OR ", " or "}
	for _, it := range verify.Registry {
		compound := false
		for _, m := range markers {
			if strings.Contains(it.Title, m) {
				compound = true
			}
		}
		// A bare `pull/push` token names two subcommands without any spaces.
		// Path-like tokens are listed in titleSlashTokensThatArePaths.
		for _, tok := range slashToken.FindAllString(it.Title, -1) {
			if !titleSlashTokensThatArePaths[strings.TrimSpace(tok)] {
				compound = true
			}
		}
		want, accepted := acceptedCompoundTitles[it.ID]
		if !compound {
			if accepted {
				t.Errorf("%s is listed as compound but its title %q has no compound marker", it.ID, it.Title)
			}
			continue
		}
		if !accepted {
			t.Errorf("%s title %q claims several steps; shrink it to what Run issues or add the count to acceptedCompoundTitles", it.ID, it.Title)
			continue
		}

		// Run under an all-green fake against a project that can satisfy every
		// precondition (a lock file for P1-06, a module for nothing else).
		root := t.TempDir()
		_ = os.WriteFile(engine.LockFilePath(root), []byte("{}"), 0o644)
		argvs := recordingExec(t, func(args []string) verify.Evidence {
			if len(args) > 1 && args[0] == "audit" && args[1] == "run" {
				return verify.Evidence{OutputExcerpt: auditSessionRunExcerpt}
			}
			return verify.Evidence{OutputExcerpt: "ok"}
		})
		if it.ID == "P4-08" {
			argvs = fakeSnapshotCreate(t, root, func() { makeSnapshot(t, root, "s1", "2026-01-01T00:00:00Z") })
		}
		remote := "staging"
		it.Run(context.Background(), engine.Config{Framework: "magento2", Domain: "x.test"}, verify.VerifyOpts{ProjectRoot: root, Remote: remote})
		if len(*argvs) < want {
			t.Errorf("%s title %q promises more steps than Run issued: %d calls %v, want at least %d", it.ID, it.Title, len(*argvs), argvJoined(*argvs), want)
		}
	}

	// A step named in a title that no longer exists is the reverse lie.
	for id := range acceptedCompoundTitles {
		if _, ok := findItem(id); !ok {
			t.Errorf("acceptedCompoundTitles names %s, which is not in the registry", id)
		}
	}
}

// Every Checks value the command accepts must be declared by an item, or
// --checks <name> selects rows that do nothing for that name.
func TestEveryAcceptedCheckNameIsDeclaredByAnItem(t *testing.T) {
	declared := map[string]bool{}
	for _, it := range verify.Registry {
		for _, c := range it.Checks {
			declared[c] = true
		}
	}
	for _, name := range []string{"lint", "profiler", "integrity"} {
		if !declared[name] {
			t.Errorf("--checks accepts %q but no registry item declares it", name)
		}
	}
}
