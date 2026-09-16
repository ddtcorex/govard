package tests

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

func conditionalPlanStepByID(t *testing.T, plan deploy.Plan, id string) deploy.Step {
	t.Helper()
	for _, step := range plan.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("plan has no step %q", id)
	return deploy.Step{}
}

func TestConditionalMigratePlanCarriesProbeAndFlag(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	recipe.ID = "sample-project"
	recipe.MigrationProbe = &deploy.MigrationProbe{
		Title:   "database schema is current",
		Command: "cd {{release_path}} && php bin/console doctrine:migrations:up-to-date",
	}
	task := recipe.Task(deploy.TaskDBMigrate)
	task.Command = "echo migrate"
	task.NeedsMigration = true
	recipe.ReplaceTask(task)

	plan, err := deploy.BuildPlanForTest(recipe, nil, "sample-remote")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.MigrationProbe == nil || plan.MigrationProbe.Command == "" {
		t.Fatal("plan dropped the migration probe")
	}
	step := conditionalPlanStepByID(t, plan, deploy.TaskDBMigrate)
	if !step.NeedsMigration {
		t.Fatal("db:migrate lost NeedsMigration through BuildPlan")
	}
}

// probeStubRunner answers the probe command with a canned exit code and runs
// everything else for real, so the test asserts what the executor chose to
// run rather than what a mock claims.
type probeStubRunner struct {
	inner     deploy.Runner
	probeExit int
	failOn    string
	ran       []string
}

func (r *probeStubRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	r.ran = append(r.ran, command)
	if r.failOn != "" && strings.Contains(command, r.failOn) {
		return deploy.Result{ExitCode: 1}, &deploy.CommandError{Command: command, ExitCode: 1, Err: errors.New("stubbed step failure")}
	}
	if strings.Contains(command, "probe-stub") {
		if r.probeExit == 0 {
			return deploy.Result{}, nil
		}
		return deploy.Result{ExitCode: r.probeExit}, &deploy.CommandError{Command: command, ExitCode: r.probeExit, Err: errors.New("probe reports drift")}
	}
	return r.inner.Run(ctx, command, opts)
}

func conditionalMigrateTestPlan(t *testing.T) deploy.Plan {
	t.Helper()
	recipe := deploy.RecipeForTest("test", []deploy.Task{
		// The release directory has to exist before the probe can ask the
		// application anything: the real probe starts with
		// `cd {{release_path}}`, which fails in a directory that was never
		// created — and a failed cd exits 1, which the probe reads as
		// "drifted". The ordering assertion below locks this in.
		{ID: deploy.TaskRelease, Command: "mkdir -p {{release_path}} && echo release-marker"},
		{ID: deploy.TaskMaintenanceEnable, Command: "echo maintenance-enable-marker"},
		{ID: deploy.TaskDBMigrate, Command: "echo db-migrate-marker"},
		{ID: deploy.TaskMaintenanceDisable, Command: "echo maintenance-disable-marker"},
		{ID: deploy.TaskAppCacheFlush, Command: "echo cache-flush-marker"},
	})
	recipe.MigrationProbe = &deploy.MigrationProbe{
		Title:   "database schema is current",
		Command: "cd {{release_path}} && echo probe-stub",
	}
	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskDBMigrate, deploy.TaskMaintenanceDisable} {
		task := recipe.Task(id)
		task.NeedsMigration = true
		recipe.ReplaceTask(task)
	}
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func runConditionalMigratePlan(t *testing.T, stub *probeStubRunner, plan deploy.Plan, opts deploy.Options) (deploy.Outcome, error) {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	host = host.WithRunner(stub)
	return deploy.NewExecutor(host, opts, io.Discard).Run(
		context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local"))
}

func ranMarker(ran []string, marker string) bool {
	for _, command := range ran {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func markerIndex(ran []string, marker string) int {
	for idx, command := range ran {
		if strings.Contains(command, marker) {
			return idx
		}
	}
	return -1
}

func TestConditionalMigrateExecutorGatesOnProbeExit(t *testing.T) {
	rows := []struct {
		name      string
		probeExit int
		wantRun   bool
	}{
		{"exit 0 skips the downtime block", 0, false},
		{"exit 1 migrates", 1, true},
		{"exit 2 migrates", 2, true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			stub := &probeStubRunner{inner: deploy.LocalRunner{}, probeExit: row.probeExit}
			plan := conditionalMigrateTestPlan(t)
			opts := deploy.Options{Remote: "local", CommandTimeout: time.Minute}
			outcome, err := runConditionalMigratePlan(t, stub, plan, opts)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := ranMarker(stub.ran, "db-migrate-marker"); got != row.wantRun {
				t.Errorf("db:migrate ran = %v, want %v (commands: %v)", got, row.wantRun, stub.ran)
			}
			if got := ranMarker(stub.ran, "maintenance-enable-marker"); got != row.wantRun {
				t.Errorf("maintenance:enable ran = %v, want %v (commands: %v)", got, row.wantRun, stub.ran)
			}
			if !ranMarker(stub.ran, "cache-flush-marker") {
				t.Errorf("app:cache:flush must always run (commands: %v)", stub.ran)
			}
			if !ranMarker(stub.ran, "probe-stub") {
				t.Errorf("the probe must run before the gated block (commands: %v)", stub.ran)
			}
			if idxRelease, idxProbe := markerIndex(stub.ran, "release-marker"), markerIndex(stub.ran, "probe-stub"); idxRelease < 0 || idxProbe < 0 || idxRelease > idxProbe {
				t.Errorf("the probe must run after the release exists (release at %d, probe at %d): %v", idxRelease, idxProbe, stub.ran)
			}
			_ = outcome
		})
	}
}

func TestConditionalMigrateUninterpretableProbeFails(t *testing.T) {
	stub := &probeStubRunner{inner: deploy.LocalRunner{}, probeExit: 3}
	plan := conditionalMigrateTestPlan(t)
	opts := deploy.Options{Remote: "local", CommandTimeout: time.Minute}
	if _, err := runConditionalMigratePlan(t, stub, plan, opts); err == nil {
		t.Fatal("a probe exit outside 0/1/2 must fail the deploy, not guess")
	}
	if ranMarker(stub.ran, "db-migrate-marker") || ranMarker(stub.ran, "maintenance-enable-marker") {
		t.Errorf("no gated task may run when the probe is uninterpretable (commands: %v)", stub.ran)
	}
}

// runConditionalMigrateResume runs the plan twice on one host: the first run
// is expected to fail (failOn names the step that breaks it), and the second
// runs with Resume and a possibly changed probe answer. It returns the
// commands each run issued, so the test can tell a re-probe from an adoption.
func runConditionalMigrateResume(t *testing.T, stub *probeStubRunner, plan deploy.Plan, first, second deploy.Options, between func()) (firstRan, secondRan []string) {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	host = host.WithRunner(stub)
	release := deploy.NewReleaseForTest("1", "abc", "local")
	if _, err := deploy.NewExecutor(host, first, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release); err == nil {
		t.Fatal("the first run must fail so the second has something to resume")
	}
	firstRan = append([]string(nil), stub.ran...)
	stub.ran = nil
	between()
	release = deploy.NewReleaseForTest("1", "abc", "local")
	if _, err := deploy.NewExecutor(host, second, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release); err != nil {
		t.Fatalf("resume: %v", err)
	}
	return firstRan, append([]string(nil), stub.ran...)
}

func resumeOpts() deploy.Options {
	return deploy.Options{Remote: "local", CommandTimeout: time.Minute, Resume: true}
}

// A recorded migrate verdict is sticky across resume: the database was
// drifted when the first run probed it, so the resumed run must re-run the
// migration (and its teardown) instead of trusting a fresh probe — the drift
// may have been healed out of band after the failure, and a re-probe would
// then excuse the very teardown the first run already started (maintenance
// left enabled, workers left paused). Found live: a manually completed
// setup:upgrade flipped the re-probe to exit 0 and stranded the window.
func TestResumeAdoptsStoredMigrateVerdict(t *testing.T) {
	stub := &probeStubRunner{inner: deploy.LocalRunner{}, probeExit: 2, failOn: "db-migrate-marker"}
	plan := conditionalMigrateTestPlan(t)
	opts := deploy.Options{Remote: "local", CommandTimeout: time.Minute}
	_, secondRan := runConditionalMigrateResume(t, stub, plan, opts, resumeOpts(), func() {
		// The drift was healed out of band after the failure (the operator
		// finished the upgrade by hand): a re-probe would now say skip.
		stub.probeExit = 0
		stub.failOn = ""
	})
	if ranMarker(secondRan, "probe-stub") {
		t.Errorf("resume must adopt the recorded migrate verdict, not re-probe (commands: %v)", secondRan)
	}
	if !ranMarker(secondRan, "db-migrate-marker") {
		t.Errorf("resume must re-run the failed migration (commands: %v)", secondRan)
	}
	if !ranMarker(secondRan, "maintenance-disable-marker") {
		t.Errorf("resume must run the gated teardown the first run never reached (commands: %v)", secondRan)
	}
	if ranMarker(secondRan, "maintenance-enable-marker") {
		t.Errorf("resume must not repeat the teardown's already-succeeded setup (commands: %v)", secondRan)
	}
}

// A recorded skip verdict is NOT sticky: drift may have appeared out of band
// after the first run, and adopting an old skip would publish code over a
// drifted database. A resume with no recorded migrate verdict re-probes.
func TestResumeReprobesAfterStoredSkip(t *testing.T) {
	stub := &probeStubRunner{inner: deploy.LocalRunner{}, probeExit: 0, failOn: "cache-flush-marker"}
	plan := conditionalMigrateTestPlan(t)
	opts := deploy.Options{Remote: "local", CommandTimeout: time.Minute}
	_, secondRan := runConditionalMigrateResume(t, stub, plan, opts, resumeOpts(), func() {
		// Drift appeared out of band after the first run's skip.
		stub.probeExit = 2
		stub.failOn = ""
	})
	if !ranMarker(secondRan, "probe-stub") {
		t.Errorf("resume with no recorded migrate verdict must re-probe (commands: %v)", secondRan)
	}
	if !ranMarker(secondRan, "db-migrate-marker") {
		t.Errorf("the fresh probe says migrate, so resume must run the migration (commands: %v)", secondRan)
	}
}
