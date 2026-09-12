package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"govard/internal/deploy"
)

// A runner in verbose mode hands the command's output to a live writer while the
// command runs, and still returns the same buffered Result. Nothing printed a byte
// of a running step before this: `setup:di:compile`, `npm ci` and an in-place rsync
// of a large release showed nothing until they finished.
func TestVerboseStreamsCommandOutputLive(t *testing.T) {
	live := &syncBuffer{}
	result, err := (deploy.LocalRunner{}).Run(context.Background(), "printf 'compiling\\n'; echo 'warning: slow' >&2",
		deploy.RunOptions{Out: live})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(live.String(), "compiling") || !strings.Contains(live.String(), "warning: slow") {
		t.Fatalf("live writer = %q, want the command's stdout and stderr", live.String())
	}
	// The buffered half is unchanged: the Result still carries everything, which is
	// what every parser and the failure message depend on.
	if !strings.Contains(result.Stdout, "compiling") || !strings.Contains(result.Stderr, "warning: slow") {
		t.Fatalf("result = %+v, want the buffered output to survive", result)
	}
}

// Presence is not liveness: output has to arrive *while* the command runs, or the
// flag buys nothing over waiting for the step to end.
func TestVerboseOutputStreamsBeforeTheCommandFinishes(t *testing.T) {
	first := make(chan string, 1)
	live := writerFunc(func(p []byte) (int, error) {
		select {
		case first <- string(p):
		default:
		}
		return len(p), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (deploy.LocalRunner{}).Run(ctx, "printf 'first\\n'; sleep 30; printf 'second\\n'", deploy.RunOptions{Out: live})
		done <- err
	}()

	select {
	case got := <-first:
		if !strings.Contains(got, "first") {
			t.Fatalf("streamed %q, want the first line", got)
		}
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("nothing reached the live writer while the command was still running")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the command did not stop after its context expired")
	}
}

// The raw output is nested under the task it belongs to, and `--json` keeps stdout
// to the single parseable document M9 guarantees: a verbose run may not mix log
// lines into machine output.
func TestVerboseOutputIsIndentedAndSuppressedByJSON(t *testing.T) {
	recipe := deploy.RecipeForTest("stream", []deploy.Task{
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "printf 'raw-line\\n'"},
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})

	run := func(options deploy.Options) string {
		t.Helper()
		out := &syncBuffer{}
		plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		options.Remote = "local"
		options.CommandTimeout = time.Minute
		if _, err := deploy.NewExecutor(host, options, out).
			Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abcdef", "main")); err != nil {
			t.Fatalf("run: %v", err)
		}
		return out.String()
	}

	verbose := run(deploy.Options{Verbose: true})
	if !strings.Contains(verbose, "raw-line") {
		t.Fatalf("verbose output = %q, want the command's own output", verbose)
	}
	if !strings.Contains(verbose, "  │ raw-line") {
		t.Fatalf("verbose output = %q, want the raw line nested under the task", verbose)
	}

	if plain := run(deploy.Options{}); strings.Contains(plain, "raw-line") {
		t.Fatalf("without --verbose the raw output must stay buffered, got %q", plain)
	}
	if machine := run(deploy.Options{Verbose: true, JSON: true}); strings.Contains(machine, "raw-line") {
		t.Fatalf("--json must win over --verbose, got %q", machine)
	}
}

// A step that says nothing looks exactly like a step that is hung. The heartbeat
// covers both, so it must not depend on the command printing anything.
func TestHeartbeatReportsAStepThatIsStillRunning(t *testing.T) {
	restore := deploy.SetHeartbeatForTest(40 * time.Millisecond)
	defer restore()

	recipe := deploy.RecipeForTest("heartbeat", []deploy.Task{
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "sleep 0.4"},
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	out := &syncBuffer{}
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if _, err := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, out).
		Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abcdef", "main")); err != nil {
		t.Fatalf("run: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "still running") || !strings.Contains(output, deploy.TaskRecord) {
		t.Fatalf("output = %q, want a heartbeat naming the running task", output)
	}

	// A step that finishes inside the interval produces none: the timeline stays as
	// quiet as it was for a fast deploy.
	fastRecipe := deploy.RecipeForTest("quick", []deploy.Task{
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "true"},
	})
	fastPlan, err := deploy.BuildPlanForTest(fastRecipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	fast := &syncBuffer{}
	if _, err := deploy.NewExecutor(host, deploy.Options{Remote: "local", CommandTimeout: time.Minute}, fast).
		Run(context.Background(), fastPlan, deploy.NewVars(), deploy.NewReleaseForTest("2", "abcdef", "main")); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(fast.String(), "still running") {
		t.Fatalf("a step that finishes inside the interval must not report progress, got %q", fast.String())
	}
}

// rsync's own progress output needs a terminal it can redraw: without one every
// update is another line in a log, and without --verbose nothing is streamed to
// read it in. Both gates are rendered into the command, so both are asserted on the
// command the step actually builds.
func TestRsyncProgressOnlyWhenATerminalIsWatching(t *testing.T) {
	origin, revision := seedGitRepo(t)
	deployPath := t.TempDir()
	release := filepath.Join(deployPath, "releases", "2")
	if err := os.MkdirAll(filepath.Join(release, "vendor"), 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	writeFile(t, filepath.Join(release, "vendor", "autoload.php"), "built\n")

	for _, testCase := range []struct {
		name      string
		verbose   bool
		json      bool
		terminal  bool
		wantFlag  bool
		expectWhy string
	}{
		{name: "a watching terminal", verbose: true, terminal: true, wantFlag: true},
		{name: "not verbose", verbose: false, terminal: true, wantFlag: false},
		{name: "not a terminal", verbose: true, terminal: false, wantFlag: false},
		{name: "json wins", verbose: true, json: true, terminal: true, wantFlag: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			host := deploy.HostForTest(deployPath, deploy.LocalRunner{})
			host.CurrentPath = filepath.Join(deployPath, "public_html")
			if _, err := (deploy.LocalRunner{}).Run(context.Background(),
				"rm -rf "+host.CurrentPath+" && git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
				t.Fatalf("clone docroot: %v", err)
			}
			seen := new([]string)
			sc := deploy.StepContextForTest(host, deploy.Options{
				Remote:   "local",
				Publish:  deploy.PublishInPlace,
				Revision: revision,
				Verbose:  testCase.verbose,
				JSON:     testCase.json,
				Settings: map[string]any{"sync_paths": []string{"vendor"}},
			})
			sc.Runner = captureRunner{base: deploy.LocalRunner{}, seen: seen}
			sc.Release = deploy.NewReleaseForTest("2", revision, "main")
			sc.Release.Path = release
			sc.Terminal = testCase.terminal

			if err := deploy.CoreActivate(context.Background(), sc); err != nil {
				t.Fatalf("activate: %v", err)
			}
			joined := strings.Join(*seen, "\n")
			if testCase.wantFlag && !strings.Contains(joined, "--info=progress2") {
				t.Fatalf("commands = %q, want rsync progress for a watching terminal", joined)
			}
			if !testCase.wantFlag && strings.Contains(joined, "--info=progress2") {
				t.Fatalf("commands = %q, want no progress output here", joined)
			}
		})
	}

	// And the flag is one the installed rsync accepts.
	source, target := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(source, "file.txt"), "content\n")
	if _, err := (deploy.LocalRunner{}).Run(context.Background(),
		"rsync -a --delete --info=progress2 "+source+"/ "+target+"/", deploy.RunOptions{}); err != nil {
		t.Fatalf("the progress flag must be valid for the target's rsync: %v", err)
	}
}

// syncBuffer is a writer safe to use from the goroutines a streamed command and the
// heartbeat write from.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

var _ io.Writer = writerFunc(nil)

// A rollback runs the same steps through `deploy.RunStep` rather than the executor,
// and its longest ones are the ones worth watching (a database restore, a
// re-activation). It has to offer the same visibility instead of being the one path
// where a long command says nothing.
func TestRollbackStepsStreamAndHeartbeatToo(t *testing.T) {
	restore := deploy.SetHeartbeatForTest(40 * time.Millisecond)
	defer restore()

	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("1", "abcdef", "main")
	release.Path = host.ReleasePath("1")
	step := deploy.RecipeStepForTest("deploy:verify", "printf 'restoring\\n'; sleep 0.3")

	out := &syncBuffer{}
	if err := deploy.RunStep(context.Background(), host,
		deploy.Options{Remote: "local", CommandTimeout: time.Minute, Verbose: true},
		deploy.NewVars(), step, release, out); err != nil {
		t.Fatalf("run step: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "  │ restoring") {
		t.Fatalf("output = %q, want the step's own output streamed and indented", output)
	}
	if !strings.Contains(output, "still running") {
		t.Fatalf("output = %q, want a heartbeat for a step run outside the executor", output)
	}
}
