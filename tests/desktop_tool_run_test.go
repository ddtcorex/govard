package tests

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"govard/internal/desktop"
)

// toolRunFake stands in for the OS process RunTool would spawn: it records
// the argv it was asked to run and serves canned stdout/stderr. It never
// touches the shell, matching the production seam contract (argv only).
type toolRunFake struct {
	mu        sync.Mutex
	calls     int
	dir       string
	argv      []string
	stdout    string
	stderr    string
	block     bool
	cancelled bool
	waitErr   error
}

func (f *toolRunFake) snapshot() (calls int, dir string, argv []string, cancelled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.dir, append([]string(nil), f.argv...), f.cancelled
}

// installToolRunFake replaces the process-spawning seam for the duration of
// a test. Block mode holds wait until cancelTool is called, emulating a
// long run (e.g. setup:di:compile) killable mid-flight.
func installToolRunFake(t *testing.T, fake *toolRunFake) {
	t.Helper()
	desktop.ResetOperationRegistryForTest()
	restore := desktop.SetStartToolProcessForTest(func(ctx context.Context, dir string, argv []string) (io.Reader, io.Reader, func() error, error) {
		fake.mu.Lock()
		fake.calls++
		fake.dir = dir
		fake.argv = append([]string(nil), argv...)
		stdout, stderr, block := fake.stdout, fake.stderr, fake.block
		fake.mu.Unlock()
		return strings.NewReader(stdout), strings.NewReader(stderr), func() error {
			if block {
				<-ctx.Done()
				fake.mu.Lock()
				fake.cancelled = true
				fake.mu.Unlock()
				return ctx.Err()
			}
			fake.mu.Lock()
			defer fake.mu.Unlock()
			return fake.waitErr
		}, nil
	})
	t.Cleanup(restore)
	t.Cleanup(desktop.ResetOperationRegistryForTest)
}

// waitForToolEvents polls the fake platform until at least want events with
// the given name arrive or the timeout lapses.
func waitForToolEvents(t *testing.T, p *desktop.FakePlatform, name string, want int, timeout time.Duration) []any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := p.EventsNamed(name); len(got) >= want {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d %q events, got %d", want, name, len(p.EventsNamed(name)))
	return nil
}

// waitForToolLockFree polls the op registry until the per-project lock is
// released: completion events arrive on the platform before the watcher's
// deferred FinishOp runs, so a test that reuses the project must wait for
// the lock itself, not just the event.
func waitForToolLockFree(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(desktop.ListOps()) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for the tool-run lock to free, ops: %+v", desktop.ListOps())
}

func toolRunOutputText(t *testing.T, p *desktop.FakePlatform) string {
	t.Helper()
	var sb strings.Builder
	for _, payload := range waitForToolEvents(t, p, desktop.EventToolOutput, 1, 5*time.Second) {
		out, ok := payload.(desktop.ToolOutputPayload)
		if !ok {
			t.Fatalf("tool:output payload has type %T, want ToolOutputPayload", payload)
		}
		sb.WriteString(out.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}

// cancelGatedReader yields one marker line only after ctx is done: the
// cancelled process flushing its death rattle after the kill, then EOF.
type cancelGatedReader struct {
	ctx   context.Context
	line  string
	spent bool
}

func (r *cancelGatedReader) Read(p []byte) (int, error) {
	if r.spent {
		return 0, io.EOF
	}
	<-r.ctx.Done()
	r.spent = true
	return copy(p, r.line), nil
}

// A cancelled run's pipe tail must never mix into the rerun's stream: every
// tool:output batch carries its op, and the publish stage drops batches from
// an op that is no longer current for the project.
func TestToolRunCancelledTailNeverMixesIntoRerun(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	desktop.ResetOperationRegistryForTest()
	t.Cleanup(desktop.ResetOperationRegistryForTest)

	var mu sync.Mutex
	calls := 0
	restore := desktop.SetStartToolProcessForTest(func(ctx context.Context, dir string, argv []string) (io.Reader, io.Reader, func() error, error) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			return &cancelGatedReader{ctx: ctx, line: "A-TAIL-MARKER\n"}, strings.NewReader(""), func() error {
				<-ctx.Done()
				return ctx.Err()
			}, nil
		}
		return strings.NewReader("B-MARKER\n"), strings.NewReader(""), func() error { return nil }, nil
	})
	t.Cleanup(restore)
	p := &desktop.FakePlatform{}

	opA, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil)
	if err != nil {
		t.Fatalf("RunTool(A) error: %v", err)
	}
	if !desktop.CancelToolRun(opA) {
		t.Fatalf("CancelToolRun(%q) = false, want true", opA)
	}
	// Immediate rerun on the same project: the lock is already free while
	// A's kill races its pipe tail.
	opB, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil)
	if err != nil {
		t.Fatalf("RunTool(B) error: %v, want the freed lock to admit a rerun", err)
	}
	if opB != opA {
		t.Errorf("rerun opID = %q, want the stable per-project lock %q", opB, opA)
	}

	// A's failed receipt proves its drain finished (dones precede wait),
	// so any leaked tail batch has arrived by now; B's completed proves
	// the rerun streamed.
	waitForToolEvents(t, p, desktop.EventToolFailed, 1, 5*time.Second)
	waitForToolEvents(t, p, desktop.EventToolCompleted, 1, 5*time.Second)

	var texts []string
	var untagged []string
	for _, payload := range p.EventsNamed(desktop.EventToolOutput) {
		out, ok := payload.(desktop.ToolOutputPayload)
		if !ok {
			untagged = append(untagged, fmt.Sprintf("%v", payload))
			continue
		}
		if out.OpID != opB {
			t.Errorf("surviving tool:output batch tagged %q, want the current op %q", out.OpID, opB)
		}
		texts = append(texts, out.Text)
	}
	if len(untagged) > 0 {
		leaked := false
		for _, raw := range untagged {
			if strings.Contains(raw, "A-TAIL-MARKER") {
				leaked = true
			}
		}
		t.Fatalf("tool:output emitted %d untagged batches (cancelled tail leaked into stream: %v): %q", len(untagged), leaked, untagged)
	}
	joined := strings.Join(texts, "\n")
	if strings.Contains(joined, "A-TAIL-MARKER") {
		t.Errorf("rerun stream mixes the cancelled run's tail: %q", joined)
	}
	if !strings.Contains(joined, "B-MARKER") {
		t.Errorf("rerun stream %q missing the current run's %q", joined, "B-MARKER")
	}
}

// Terminal events share the stable per-project OpID, so a stale dead-run
// "failed (cancelled)" is indistinguishable from the current run's outcome
// by ID alone. Each carries its run generation; consumers apply a terminal
// event only when its generation is current for the project.
func TestToolRunStaleTerminalEventsIgnoredByGeneration(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	desktop.ResetOperationRegistryForTest()
	t.Cleanup(desktop.ResetOperationRegistryForTest)

	var mu sync.Mutex
	calls := 0
	restore := desktop.SetStartToolProcessForTest(func(ctx context.Context, dir string, argv []string) (io.Reader, io.Reader, func() error, error) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			return strings.NewReader(""), strings.NewReader(""), func() error {
				<-ctx.Done()
				return ctx.Err()
			}, nil
		}
		return strings.NewReader(""), strings.NewReader(""), func() error { return nil }, nil
	})
	t.Cleanup(restore)
	p := &desktop.FakePlatform{}

	opA, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil)
	if err != nil {
		t.Fatalf("RunTool(A) error: %v", err)
	}
	if !desktop.CancelToolRun(opA) {
		t.Fatalf("CancelToolRun(%q) = false, want true", opA)
	}
	opB, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil)
	if err != nil {
		t.Fatalf("RunTool(B) error: %v", err)
	}
	if opB != opA {
		t.Fatalf("rerun opID = %q, want the stable per-project lock %q", opB, opA)
	}

	failed := waitForToolEvents(t, p, desktop.EventToolFailed, 1, 5*time.Second)
	completed := waitForToolEvents(t, p, desktop.EventToolCompleted, 1, 5*time.Second)
	aFailed, ok := failed[0].(desktop.ToolResultPayload)
	if !ok {
		t.Fatalf("tool:failed payload has type %T, want ToolResultPayload", failed[0])
	}
	bDone, ok := completed[0].(desktop.ToolResultPayload)
	if !ok {
		t.Fatalf("tool:completed payload has type %T, want ToolResultPayload", completed[0])
	}
	if bDone.Gen != aFailed.Gen+1 {
		t.Errorf("terminal generations = A:%d B:%d, want B exactly one admission after A", aFailed.Gen, bDone.Gen)
	}

	// The consumer rule: apply a terminal event only for the current
	// generation, so A's late cancelled receipt can never mark B failed.
	if desktop.IsCurrentToolGeneration(opB, aFailed.Gen) {
		t.Errorf("stale A failed (gen %d) still applies after B admitted; it would wrongly mark B failed", aFailed.Gen)
	}
	if !desktop.IsCurrentToolGeneration(opB, bDone.Gen) {
		t.Errorf("current B completed (gen %d) does not apply", bDone.Gen)
	}
	for _, payload := range p.EventsNamed(desktop.EventToolFailed) {
		if f, ok := payload.(desktop.ToolResultPayload); ok && f.Gen == bDone.Gen {
			t.Errorf("failure surfaced for the current run B: %q", f.Message)
		}
	}
}

// A run streams sanitized output events, reports completion, and frees the
// per-project lock so the next run can start.
func TestToolRunStreamsSanitizedOutput(t *testing.T) {
	project := seedToolRunnerProject(t, "magento2")
	fake := &toolRunFake{stdout: "\x1b[31mhello\x1b[0m\x00world\n\n   \nline2\rprogress-part\n"}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	opID, err := desktop.RunTool(context.Background(), p, project, "magento-cache-flush", map[string]string{"type": "full_page"})
	if err != nil {
		t.Fatalf("RunTool() error: %v", err)
	}
	if want := "tool:" + filepath.Clean(project); opID != want {
		t.Errorf("RunTool() opID = %q, want %q", opID, want)
	}

	calls, dir, argv, _ := fake.snapshot()
	if calls != 1 {
		t.Fatalf("process seam called %d times, want exactly 1", calls)
	}
	if dir != filepath.Clean(project) {
		t.Errorf("process dir = %q, want project root %q", dir, filepath.Clean(project))
	}
	wantArgv := []string{"bin/magento", "cache:flush", "full_page"}
	if strings.Join(argv, "\x00") != strings.Join(wantArgv, "\x00") {
		t.Errorf("process argv = %q, want %q", argv, wantArgv)
	}

	output := toolRunOutputText(t, p)
	if !strings.Contains(output, "helloworld") {
		t.Errorf("streamed output %q missing sanitized %q", output, "helloworld")
	}
	if strings.Contains(output, "\x1b") {
		t.Errorf("streamed output %q still contains ANSI escapes", output)
	}
	if strings.Contains(output, "\x00") {
		t.Errorf("streamed output %q still contains control bytes", output)
	}
	if !strings.Contains(output, "line2") {
		t.Errorf("streamed output %q missing %q", output, "line2")
	}

	completed := waitForToolEvents(t, p, desktop.EventToolCompleted, 1, 5*time.Second)
	done, ok := completed[0].(desktop.ToolResultPayload)
	if !ok {
		t.Fatalf("tool:completed payload has type %T, want ToolResultPayload", completed[0])
	}
	if done.OpID != opID {
		t.Errorf("tool:completed OpID = %q, want %q", done.OpID, opID)
	}

	waitForToolLockFree(t, 5*time.Second)
}

// Cancelling mid-flight kills the process (context done), reports failure
// as cancelled, and frees the per-project lock so a later run starts clean.
func TestToolRunCancelKillsProcessAndFreesLock(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	fake := &toolRunFake{block: true, stdout: "warming up\n"}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	opID, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil)
	if err != nil {
		t.Fatalf("RunTool() error: %v", err)
	}
	if ops := desktop.ListOps(); len(ops) != 1 || ops[0].ID != opID {
		t.Fatalf("ListOps() = %+v, want exactly the running op %q", ops, opID)
	}

	if !desktop.CancelToolRun(opID) {
		t.Fatalf("CancelToolRun(%q) = false, want true for the running op", opID)
	}

	failed := waitForToolEvents(t, p, desktop.EventToolFailed, 1, 5*time.Second)
	result, ok := failed[0].(desktop.ToolResultPayload)
	if !ok {
		t.Fatalf("tool:failed payload has type %T, want ToolResultPayload", failed[0])
	}
	if result.OpID != opID {
		t.Errorf("tool:failed OpID = %q, want %q", result.OpID, opID)
	}
	if !strings.Contains(strings.ToLower(result.Message), "cancel") {
		t.Errorf("tool:failed message %q does not report cancellation", result.Message)
	}

	if _, _, _, cancelled := fake.snapshot(); !cancelled {
		t.Error("cancelled run did not observe context done; the process was not killed")
	}
	waitForToolLockFree(t, 5*time.Second)
	if desktop.CancelToolRun(opID) {
		t.Errorf("CancelToolRun(%q) after completion = true, want false (harmless no-op)", opID)
	}

	// The freed lock admits a fresh run.
	fake.mu.Lock()
	fake.block = false
	fake.mu.Unlock()
	if _, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil); err != nil {
		t.Errorf("RunTool() after cancel error: %v, want the freed lock to admit a new run", err)
	}
	waitForToolEvents(t, p, desktop.EventToolCompleted, 1, 5*time.Second)
}

// Unknown preset IDs are refused before any execution: no process, no lock,
// no events.
func TestToolRunRefusesUnknownPreset(t *testing.T) {
	project := seedToolRunnerProject(t, "magento2")
	fake := &toolRunFake{}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	if _, err := desktop.RunTool(context.Background(), p, project, "no-such-tool", nil); err == nil {
		t.Error("RunTool(unknown preset) = nil error, want refusal")
	} else if !strings.Contains(err.Error(), "unknown tool preset") {
		t.Errorf("RunTool(unknown preset) error = %q, want it to name the unknown preset", err.Error())
	}
	if _, err := desktop.RunTool(context.Background(), p, "", "npm-install", nil); err == nil {
		t.Error("RunTool(empty project) = nil error, want the resolution failure surfaced")
	}

	if calls, _, _, _ := fake.snapshot(); calls != 0 {
		t.Errorf("process seam called %d times for refused runs, want 0 (never executed)", calls)
	}
	if ops := desktop.ListOps(); len(ops) != 0 {
		t.Errorf("ListOps() holds %d entries after refused runs, want none", len(ops))
	}
	if events := p.Events(); len(events) != 0 {
		t.Errorf("refused runs emitted %d events, want none", len(events))
	}
}

// One tool op per project: a second run while one is in flight is refused
// without spawning another process.
func TestToolRunSecondRunWhileRunningIsRefused(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	fake := &toolRunFake{block: true}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	opID, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil)
	if err != nil {
		t.Fatalf("RunTool() error: %v", err)
	}
	if _, err := desktop.RunTool(context.Background(), p, project, "npm-run-build", nil); err == nil {
		t.Error("second RunTool() while running = nil error, want the per-project lock to refuse it")
	} else if !strings.Contains(err.Error(), "already running") {
		t.Errorf("second RunTool() error = %q, want it to report the lock held", err.Error())
	}
	if calls, _, _, _ := fake.snapshot(); calls != 1 {
		t.Errorf("process seam called %d times, want 1 (refused run never executed)", calls)
	}
	if !desktop.CancelToolRun(opID) {
		t.Fatalf("CancelToolRun(%q) = false, want true", opID)
	}
	waitForToolEvents(t, p, desktop.EventToolFailed, 1, 5*time.Second)
}

// Validated args render into argv in ArgSpec order with no shell; invalid
// args are rejected before any execution.
func TestToolRunBuildsArgvFromValidatedArgs(t *testing.T) {
	project := seedToolRunnerProject(t, "laravel")
	fake := &toolRunFake{}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	cases := []struct {
		name     string
		preset   string
		args     map[string]string
		wantArgv []string
	}{
		{"artisan flags", "laravel-migrate", map[string]string{"database": "mysql", "force": "true"}, []string{"php", "artisan", "migrate", "--database=mysql", "--force"}},
		{"empty flag means set", "laravel-migrate", map[string]string{"force": ""}, []string{"php", "artisan", "migrate", "--force"}},
		{"false flag omitted", "laravel-migrate", map[string]string{"force": "false"}, []string{"php", "artisan", "migrate"}},
		{"positional value", "magento-cache-flush", map[string]string{"type": "full_page"}, []string{"bin/magento", "cache:flush", "full_page"}},
		{"composer flag", "composer-install", map[string]string{"no-dev": ""}, []string{"composer", "install", "--no-dev"}},
		{"no args", "npm-install", nil, []string{"npm", "install"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake.mu.Lock()
			fake.calls = 0
			fake.argv = nil
			fake.mu.Unlock()
			if _, err := desktop.RunTool(context.Background(), p, project, tc.preset, tc.args); err != nil {
				t.Fatalf("RunTool(%s) error: %v", tc.preset, err)
			}
			waitForToolEvents(t, p, desktop.EventToolCompleted, 1, 5*time.Second)
			waitForToolLockFree(t, 5*time.Second)
			if _, _, argv, _ := fake.snapshot(); strings.Join(argv, "\x00") != strings.Join(tc.wantArgv, "\x00") {
				t.Errorf("process argv = %q, want %q", argv, tc.wantArgv)
			}
		})
	}

	before, _, _, _ := fake.snapshot()
	if err := desktop.ValidateToolArgs("magento-cache-flush", map[string]string{"type": "; rm -rf /"}); err == nil {
		t.Fatal("ValidateToolArgs(injection) = nil, want rejection")
	}
	injectionCalls := before
	if _, err := desktop.RunTool(context.Background(), p, project, "magento-cache-flush", map[string]string{"type": "; rm -rf /"}); err == nil {
		t.Error("RunTool(injection) = nil error, want rejection before execution")
	}
	if calls, _, _, _ := fake.snapshot(); calls != injectionCalls {
		t.Error("rejected injection reached the process seam; validation must run before execution")
	}
}

// Slow presets get the long ceiling so a 15-minute compile is never killed
// by the short default; fast ones keep it.
func TestToolRunTimeoutCeilings(t *testing.T) {
	if got := desktop.ToolRunTimeoutForTest("magento-setup-di-compile"); got != 15*time.Minute {
		t.Errorf("timeout for setup:di:compile = %v, want 15m", got)
	}
	if got := desktop.ToolRunTimeoutForTest("magento-cache-flush"); got != 2*time.Minute {
		t.Errorf("timeout for cache:flush = %v, want 2m", got)
	}
}
