package tests

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// P3-15's title promises three audit-lifecycle invocations, and every one of
// them needs a session id the item has to supply itself: `audit status` errors
// with "audit status requires --session" before it reads any project state
// (internal/cmd/audit.go:425), and `audit result` refuses without both --session
// and --run (:451). The registry item ran one bare `audit status --format json`,
// so the row was red on every project and could never go green (issue #493).
//
// The item therefore creates its own session first. `audit run --checks
// integrity --format json` is the container-free integrity check — the `audit`
// group declares runtime requirement `none` (internal/cmd/audit.go:194) and the
// run only requires a container runtime for the lint/profiler checks it did not
// select (:250-256) — so the lifecycle stays runnable on a host with no
// container runtime.
//
// The four invocations are pinned in order because a wrong argv here is
// invisible through the exit code: execGovard short-circuits to exit 0 under a
// test binary (internal/verify/exec.go:40-42), so "the item failed" would prove
// nothing about which commands ran.

// auditSessionRunExcerpt is the shape `audit run --format json` prints: a
// RunResult whose first three keys are schema_version, session_id and run_id
// (internal/audit/model.go:90-93).
const auditSessionRunExcerpt = `{"schema_version":1,"session_id":"sess-1","run_id":"run-1"}`

// installAuditSessionFake records EVERY argv the item invokes, in order, and
// answers `audit run` with runExcerpt. The shared captureItemArgvs helper is
// wrong for this item: it keeps only the first invocation, so a four-invocation
// item would look identical whether it ran the whole lifecycle or stopped after
// creating the session.
func installAuditSessionFake(t *testing.T, runExcerpt string) *[][]string {
	t.Helper()
	return installAuditSessionFakeExiting(t, runExcerpt, 0)
}

// installAuditSessionFakeExiting is the same fake with a chosen exit code for the
// creating `audit run`, which the skip reason has to report.
func installAuditSessionFakeExiting(t *testing.T, runExcerpt string, runExit int) *[][]string {
	t.Helper()
	var argvs [][]string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		argvs = append(argvs, append([]string(nil), args...))
		if len(args) > 1 && args[0] == "audit" && args[1] == "run" {
			return verify.Evidence{ExitCode: runExit, OutputExcerpt: runExcerpt, JSONValid: true}, true
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(args, " "), JSONValid: true}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return &argvs
}

// runAuditSessionItem drives P3-15 itself, through the same findItem helper the
// other per-item tests use, so the assertions are about the shipped registry
// item and not about a reimplementation of it.
func runAuditSessionItem(t *testing.T) verify.Evidence {
	t.Helper()
	return runAuditSessionItemWith(t, verify.VerifyOpts{ProjectRoot: t.TempDir()})
}

func runAuditSessionItemWith(t *testing.T, opts verify.VerifyOpts) verify.Evidence {
	t.Helper()
	item, ok := findItem("P3-15")
	if !ok {
		t.Fatal("P3-15 is missing from the registry")
	}
	return item.Run(context.Background(), engine.Config{Framework: "magento2"}, opts)
}

func TestP315CreatesAndUsesItsOwnAuditSession(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := installAuditSessionFake(t, auditSessionRunExcerpt)

	ev := runAuditSessionItem(t)

	want := [][]string{
		{"audit", "run", "--checks", "integrity", "--format", "json"},
		{"audit", "status", "--session", "sess-1", "--format", "json"},
		{"audit", "result", "--session", "sess-1", "--run", "run-1", "--format", "json"},
		{"audit", "rerun", "--session", "sess-1", "--format", "json"},
	}
	if !reflect.DeepEqual(*argvs, want) {
		t.Fatalf("P3-15 invoked\n got %v\nwant %v", *argvs, want)
	}
	if ev.Skipped || ev.ExitCode != 0 {
		t.Fatalf("P3-15 with every child green: exit=%d skipped=%v evidence=%q", ev.ExitCode, ev.Skipped, ev.OutputExcerpt)
	}
}

// An id that never comes back must not become an empty flag: the item skips and
// names which half of the identity is missing instead of running a lifecycle
// command that could only fail for a reason the operator cannot act on. The skip
// is not a failure — it carries exit 0 and the runner counts it in neither tally
// — and it must not spawn a single lifecycle child.
//
// Both ids are required. Guarding the session id alone left the half identity
// reachable exactly where the parser's truncation handling says it is: a decode
// that stops between the two keys yields session_id without run_id, and
// `audit result --session <id> --run ""` is rejected by the command
// (internal/cmd/audit.go:451) — a red where the brief requires a skip.
func TestP315SkipsWhenNoSessionIdComesBack(t *testing.T) {
	for _, tc := range []struct {
		name       string
		excerpt    string
		runExit    int
		wantReason string
	}{
		{
			name:       "unparseable output",
			excerpt:    "audit run: no project config found",
			wantReason: "audit run produced no session id or run id (exit 0)",
		},
		{
			name:       "json without a session id",
			excerpt:    `{"schema_version":1,"run_id":"run-1"}`,
			wantReason: "audit run produced no session id (exit 0)",
		},
		{
			name:       "decode stopped between session_id and run_id",
			excerpt:    `{"schema_version":1,"session_id":"sess-1","run_`,
			wantReason: "audit run produced no run id (exit 0)",
		},
		// A creating run that exits non-zero without ids is red, not a skip:
		// see TestP315RedOnCreateExit3And4.
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			argvs := installAuditSessionFakeExiting(t, tc.excerpt, tc.runExit)

			ev := runAuditSessionItem(t)

			if !ev.Skipped {
				t.Fatalf("P3-15 with excerpt %q (creating run exit %d): skipped=%v evidence=%q; want a skip",
					tc.excerpt, tc.runExit, ev.Skipped, ev.OutputExcerpt)
			}
			if ev.SkipReason != tc.wantReason {
				t.Fatalf("P3-15 skip reason = %q, want %q", ev.SkipReason, tc.wantReason)
			}
			if ev.ExitCode != 0 {
				t.Fatalf("P3-15 skip carries exit code %d; a skip is not a failure", ev.ExitCode)
			}
			if len(*argvs) != 1 {
				t.Fatalf("P3-15 ran %v without a complete identity; only the audit run may reach a child", *argvs)
			}
		})
	}
}

// execGovard keeps only the first 500 characters of a child's output
// (internal/verify/exec.go:66-69), and a real `audit run --checks integrity
// --format json` is much longer than that — measured against a minimal Magento
// project skeleton it was 1996 bytes, with each job's evidence map inlined. The
// identity is encoded before everything else, so the item has to read the ids
// out of the truncated prefix; a plain json.Unmarshal would reject the cut
// document and turn the whole item into a permanent skip on every real project.
func TestP315ReadsTheSessionIdFromATruncatedExcerpt(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	const sessionID = "20260929T040421Z-c014a30b"
	full := `{"schema_version":1,"session_id":"` + sessionID + `","run_id":"run-0001","project_id":"project-a72a0a239640d1b0","scope":"project","status":"failed","started_at":"2026-09-29T04:04:21.335554902Z","finished_at":"2026-09-29T04:04:21.335554952Z","environment":{"framework":"magento2","govard_version":"dev","web_server":"nginx"},"source":{"git_dirty":false,"digest":"sha256:7ec838779edeaf1e588799b8507cf4e6b822a3dc9e65365e468a169fa5d8d044"},"jobs":[{"id":"integrity","kind":"host-analysis","status":"failed","evidence":{"findings":[` +
		strings.Repeat(`{"rule":"COMPOSER_LOCK_MISSING","path":"composer.lock"},`, 20) + `]}}]}`
	if len(full) <= 500 {
		t.Fatalf("the fixture is only %d bytes: it cannot exercise the 500-character truncation", len(full))
	}
	excerpt := full[:500]
	if json.Valid([]byte(excerpt)) {
		t.Fatalf("the excerpt is still valid JSON, so it cannot exercise the truncation: %q", excerpt)
	}
	argvs := installAuditSessionFake(t, excerpt)

	ev := runAuditSessionItem(t)

	want := [][]string{
		{"audit", "run", "--checks", "integrity", "--format", "json"},
		{"audit", "status", "--session", sessionID, "--format", "json"},
		{"audit", "result", "--session", sessionID, "--run", "run-0001", "--format", "json"},
		{"audit", "rerun", "--session", sessionID, "--format", "json"},
	}
	if !reflect.DeepEqual(*argvs, want) {
		t.Fatalf("P3-15 invoked\n got %v\nwant %v", *argvs, want)
	}
	if ev.Skipped {
		t.Fatalf("P3-15 skipped on a truncated but readable excerpt: %q", ev.SkipReason)
	}
}

// --allow-xdebug is a persistent flag on the `audit` group
// (internal/cmd/audit.go:211), so all four subcommands accept it, but only `run`
// and `rerun` enforce the guard (:264, :384). P3-15 forwards it to exactly those
// two, the way P3-12/P3-13/P5-04 forward it to their own audit run (P3-14 does
// not, which is a rider for the task that owns P3-13/P3-14): measured on
// an xdebug-enabled project, the creating run without the flag exits 1 with the
// guard error and no JSON (so the item would skip), and the rerun without it
// exits 1 the same way — waiving only the creating run would just move the red
// one call later.
func TestP315ForwardsAllowXdebugToTheGuardedCalls(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := installAuditSessionFake(t, auditSessionRunExcerpt)

	ev := runAuditSessionItemWith(t, verify.VerifyOpts{ProjectRoot: t.TempDir(), AllowXdebug: true})

	want := [][]string{
		{"audit", "run", "--checks", "integrity", "--format", "json", "--allow-xdebug"},
		{"audit", "status", "--session", "sess-1", "--format", "json"},
		{"audit", "result", "--session", "sess-1", "--run", "run-1", "--format", "json"},
		{"audit", "rerun", "--session", "sess-1", "--format", "json", "--allow-xdebug"},
	}
	if !reflect.DeepEqual(*argvs, want) {
		t.Fatalf("P3-15 with --allow-xdebug invoked\n got %v\nwant %v", *argvs, want)
	}
	if ev.Skipped || ev.ExitCode != 0 {
		t.Fatalf("P3-15 with every child green: exit=%d skipped=%v evidence=%q", ev.ExitCode, ev.Skipped, ev.OutputExcerpt)
	}
}

// The item's verdict is the lifecycle's, not the last child's: a failing command
// that a later green `audit rerun` overwrote would report a broken lifecycle as
// a pass. The rows below are the measured exit-code semantics of this branch
// (fix round 1), and the table is the ruling they produced:
//
//   - `status`/`result` never exit non-zero for a findings verdict — they exit 0
//     and print the session or the run — so any non-zero is the command failing
//     to do its job (measured: `status --session does-not-exist` → exit 1 with
//     an empty stdout; an unknown flag → exit 2).
//   - `run`/`rerun` exit 1 for "the checks reported findings" *with* the
//     RunResult on stdout and for a genuine failure with nothing on stdout, so
//     the exit code alone cannot separate the two and the item does not try:
//     every non-zero is red, whatever it means.
//
// All four declared red codes are pinned here — 1 execution, 2 usage, 3
// capability, 4 config — because a declaration no row exercises is a comment.
func TestP315ReportsAFailingLifecycleCommand(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failing []string
		exit    int
		payload string
	}{
		{
			name:    "status cannot read the session it just created",
			failing: []string{"audit", "status", "--session", "sess-1", "--format", "json"},
			exit:    1,
			payload: `read session manifest "sess-1"`,
		},
		{
			name:    "status rejects the argv",
			failing: []string{"audit", "status", "--session", "sess-1", "--format", "json"},
			exit:    2,
			payload: "unknown flag",
		},
		{
			name:    "status runs on a host without a container runtime",
			failing: []string{"audit", "status", "--session", "sess-1", "--format", "json"},
			exit:    3,
			payload: "CAPABILITY_MISSING",
		},
		{
			name:    "status hits a project configuration error",
			failing: []string{"audit", "status", "--session", "sess-1", "--format", "json"},
			exit:    4,
			payload: "config_error",
		},
		{
			name:    "rerun adjudicated findings",
			failing: []string{"audit", "rerun", "--session", "sess-1", "--format", "json"},
			exit:    1,
			payload: `"status":"failed"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			want := strings.Join(tc.failing, " ")
			ran := 0
			verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
				if len(args) > 1 && args[0] == "audit" && args[1] == "run" {
					return verify.Evidence{ExitCode: 0, OutputExcerpt: auditSessionRunExcerpt, JSONValid: true}, true
				}
				if strings.Join(args, " ") == want {
					ran++
					return verify.Evidence{ExitCode: tc.exit, OutputExcerpt: tc.payload}, true
				}
				return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(args, " ")}, true
			})
			t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

			ev := runAuditSessionItem(t)

			if ran == 0 {
				t.Fatalf("the fake never saw `%s`: P3-15's argv no longer reaches this case", want)
			}
			if ev.Skipped {
				t.Fatalf("P3-15 skipped (reason %q) instead of reporting the failing command", ev.SkipReason)
			}
			if ev.ExitCode == 0 {
				t.Fatalf("P3-15 passed although `%s` exited %d: %q", want, tc.exit, ev.OutputExcerpt)
			}
			if ev.ExitCode != tc.exit {
				t.Fatalf("P3-15 reported exit %d, want the failing child's %d: %q", ev.ExitCode, tc.exit, ev.OutputExcerpt)
			}
			if !strings.Contains(ev.OutputExcerpt, tc.payload) {
				t.Fatalf("P3-15 evidence does not come from the failing command: %q", ev.OutputExcerpt)
			}
		})
	}
}
