package tests

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestBuildPlanKeepsRecipeOrderWithoutHooks(t *testing.T) {
	plan, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), nil, "staging")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Remote != "staging" {
		t.Fatalf("plan remote = %q, want staging", plan.Remote)
	}
	ids := plan.StepIDs()
	want := deploy.TaskIDList()
	if len(ids) != len(want) {
		t.Fatalf("plan has %d steps, want %d: %v", len(ids), len(want), ids)
	}
	for idx := range want {
		if ids[idx] != want[idx] {
			t.Fatalf("step %d = %q, want %q", idx, ids[idx], want[idx])
		}
	}
	for _, step := range plan.Steps {
		if step.Kind != deploy.StepTask {
			t.Fatalf("step %q kind = %q, want task", step.ID, step.Kind)
		}
		if step.Stage == "" {
			t.Fatalf("step %q has no stage", step.ID)
		}
	}
}

func TestPlanHookInsertionOrderAndTieBreak(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	hooks := []deploy.Hook{
		{Name: "first", On: "publish:activate", Position: deploy.PositionAfter, Order: 1, Run: "echo first"},
		{Name: "zeroth", On: "publish:activate", Position: deploy.PositionAfter, Order: 0, Run: "echo zeroth"},
		{Name: "before-activate", On: "publish:activate", Position: deploy.PositionBefore, Run: "echo before"},
		{Name: "on-stage", On: "stage:build", Position: deploy.PositionAfter, Run: "echo stage"},
		{Name: "on-hook", On: "hook:zeroth", Position: deploy.PositionAfter, Run: "echo nested"},
	}
	plan, err := deploy.BuildPlanForTest(recipe, hooks, "staging")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	ids := plan.StepIDs()
	index := map[string]int{}
	for idx, id := range ids {
		index[id] = idx
	}
	activate, ok := index["publish:activate"]
	if !ok {
		t.Fatalf("plan lost publish:activate: %v", ids)
	}
	if index["hook:before-activate"] != activate-1 {
		t.Fatalf("before-activate at %d, want immediately before publish:activate at %d: %v", index["hook:before-activate"], activate, ids)
	}
	if index["hook:zeroth"] != activate+1 {
		t.Fatalf("zeroth at %d, want immediately after publish:activate at %d: %v", index["hook:zeroth"], activate, ids)
	}
	// "zeroth" (order 0) sorts before "first" (order 1), but "on-hook" is
	// anchored on "zeroth" and an `after` anchor means immediately after, so it
	// lands between the two: the nested anchor wins on immediacy while the
	// Order tie-break still holds relative to the task.
	if index["hook:on-hook"] != index["hook:zeroth"]+1 {
		t.Fatalf("nested hook at %d, want immediately after hook:zeroth at %d: %v", index["hook:on-hook"], index["hook:zeroth"], ids)
	}
	if index["hook:first"] <= index["hook:on-hook"] {
		t.Fatalf("first at %d must run after the order-0 chain ending at %d: %v", index["hook:first"], index["hook:on-hook"], ids)
	}
	// A stage anchor means "after the whole stage", so it lands after the last
	// step of that stage — deploy:artifact, not build:assets — and before the
	// first step of the next stage.
	if index["hook:on-stage"] != index["deploy:artifact"]+1 {
		t.Fatalf("stage hook at %d, want immediately after the last build step deploy:artifact at %d: %v", index["hook:on-stage"], index["deploy:artifact"], ids)
	}
	if index["hook:on-stage"] >= index["maintenance:enable"] {
		t.Fatalf("stage hook must run before the publish stage starts: %v", ids)
	}
	for _, step := range plan.Steps {
		if step.Kind == deploy.StepHook && step.Stage == "" {
			t.Fatalf("hook %q has no stage", step.ID)
		}
	}
}

func TestPlanRejectsBadHooks(t *testing.T) {
	cases := []struct {
		name  string
		hooks []deploy.Hook
		want  error
	}{
		{"unknown task anchor", []deploy.Hook{{Name: "a", On: "publish:nope", Run: "true"}}, deploy.ErrUnknownAnchor},
		{"unknown hook anchor", []deploy.Hook{{Name: "a", On: "hook:ghost", Run: "true"}}, deploy.ErrUnknownAnchor},
		{"duplicate name", []deploy.Hook{
			{Name: "dup", On: "deploy:code", Run: "true"},
			{Name: "dup", On: "deploy:code", Run: "true"},
		}, deploy.ErrDuplicateHook},
		{"cycle", []deploy.Hook{
			{Name: "a", On: "hook:b", Run: "true"},
			{Name: "b", On: "hook:a", Run: "true"},
		}, deploy.ErrHookCycle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), tc.hooks, "staging")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// buildStageSteps are the tasks a server build runs and an artifact build
// replaces with deploy:artifact. They are the exact set the plan must flip.
var buildStageSteps = []string{
	deploy.TaskVendors, deploy.TaskPatches, deploy.TaskCompile,
	deploy.TaskFrontend, deploy.TaskAssets,
}

func TestPlanForBuildModeSwitchesTheBuildBranch(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	// The neutral recipe leaves the framework steps empty, so give them a
	// command: the mode must flip an implemented step, not an empty one.
	for _, id := range buildStageSteps {
		deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Command: "build " + id})
	}
	deploy.OverrideTaskForTest(&recipe, deploy.TaskArtifact, deploy.Task{ID: deploy.TaskArtifact, Core: func(context.Context, *deploy.StepContext) error { return nil }})

	base, err := deploy.BuildPlanForTest(recipe, nil, "staging")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	server := base.ForBuildMode(deploy.BuildServer)
	assertBuildBranch(t, server, deploy.BuildServer)
	artifact := base.ForBuildMode(deploy.BuildArtifact)
	assertBuildBranch(t, artifact, deploy.BuildArtifact)

	// The mode never changes the shape of the pipeline: every step id is still
	// there, in the same order. A mode that dropped a step would make `--from`
	// and the resume bookkeeping disagree with the timeline.
	if got, want := server.StepIDs(), base.StepIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("server mode changed the step list:\n got %v\nwant %v", got, want)
	}
	if got, want := artifact.StepIDs(), base.StepIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact mode changed the step list:\n got %v\nwant %v", got, want)
	}
}

func assertBuildBranch(t *testing.T, plan deploy.Plan, mode string) {
	t.Helper()
	for _, id := range buildStageSteps {
		step := stepForTest(t, plan, id)
		wantSkipped := mode == deploy.BuildArtifact
		if step.Skipped != wantSkipped {
			t.Errorf("%s mode: %s skipped = %v, want %v", mode, id, step.Skipped, wantSkipped)
		}
		if step.Implemented() == wantSkipped {
			t.Errorf("%s mode: %s implemented = %v, want %v", mode, id, step.Implemented(), !wantSkipped)
		}
		if wantSkipped && step.SkipReason == "" {
			t.Errorf("%s mode: %s is skipped with no reason for the operator", mode, id)
		}
	}

	artifactStep := stepForTest(t, plan, deploy.TaskArtifact)
	wantArtifactSkipped := mode == deploy.BuildServer
	if artifactStep.Skipped != wantArtifactSkipped {
		t.Errorf("%s mode: deploy:artifact skipped = %v, want %v", mode, artifactStep.Skipped, wantArtifactSkipped)
	}
	if artifactStep.Implemented() == wantArtifactSkipped {
		t.Errorf("%s mode: deploy:artifact implemented = %v, want %v", mode, artifactStep.Implemented(), !wantArtifactSkipped)
	}
	if wantArtifactSkipped && artifactStep.SkipReason == "" {
		t.Errorf("%s mode: deploy:artifact is skipped with no reason", mode)
	}
}

func stepForTest(t *testing.T, plan deploy.Plan, id string) deploy.Step {
	t.Helper()
	for _, step := range plan.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("%s is not in the plan %v", id, plan.StepIDs())
	return deploy.Step{}
}

func TestPlanForBuildModeLeavesHooksAlone(t *testing.T) {
	hooks := []deploy.Hook{
		{Name: "before-build", On: "build:vendors", Position: deploy.PositionBefore, Run: "true"},
		{Name: "after-build", On: "stage:build", Run: "true"},
	}
	base, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), hooks, "staging")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	for _, mode := range []string{deploy.BuildServer, deploy.BuildArtifact} {
		plan := base.ForBuildMode(mode)
		for _, step := range plan.Steps {
			if step.Kind != deploy.StepHook {
				continue
			}
			if step.Skipped || !step.Implemented() {
				t.Errorf("%s mode: hook %s was disabled by the build mode", mode, step.ID)
			}
		}
	}
}

// Spec 11: a deploy that runs `db:migrate` with no HTTP check configured must
// say so. "The CLI ran" is not "the site serves", and SSH-only verification
// cannot tell the difference.
func TestMissingVerifyWarningNamesTheSetting(t *testing.T) {
	migrating := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&migrating, deploy.TaskDBMigrate, deploy.Task{
		ID: deploy.TaskDBMigrate, Stage: deploy.StagePublish, Command: "bin/migrate",
	})
	plan, err := deploy.BuildPlanForTest(migrating, nil, "production")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	warning := deploy.MissingVerifyWarning(plan, deploy.Options{Remote: "production"})
	if warning == "" {
		t.Fatal("a deploy that migrates without a verify URL must warn")
	}
	if !strings.Contains(warning, "deploy.verify.url") {
		t.Fatalf("the warning must name the setting, got %q", warning)
	}

	if got := deploy.MissingVerifyWarning(plan, deploy.Options{VerifyURL: "https://shop.test"}); got != "" {
		t.Fatalf("a configured verify URL must silence the warning, got %q", got)
	}

	// No migration in the plan, no warning: the stage's SSH-only checks are
	// proportionate for a code-only deploy.
	codeOnly, err := deploy.BuildPlanForTest(deploy.RecipeForTest("plain", []deploy.Task{
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "true"},
	}), nil, "production")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got := deploy.MissingVerifyWarning(codeOnly, deploy.Options{}); got != "" {
		t.Fatalf("a deploy with no migration must not warn, got %q", got)
	}

	// A recipe that leaves db:migrate empty does not migrate.
	empty, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), nil, "production")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got := deploy.MissingVerifyWarning(empty, deploy.Options{}); got != "" {
		t.Fatalf("an unimplemented db:migrate is not a migration, got %q", got)
	}
}

// P6 and 9.1: an atomic symlink activation needs no maintenance window at all,
// because nothing serving the site is being rewritten. 7.2/10.4 keep the
// maintenance tasks in the neutral pipeline because a *migration* does need one,
// so the window follows what the plan actually does.
func TestPlanForPublishStrategySkipsTheMaintenanceWindowOnASymlinkActivation(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable} {
		stage, _ := deploy.StageForTask(id)
		deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Stage: stage, Command: "bin/maintenance " + id})
	}
	plan, err := deploy.BuildPlanForTest(recipe, nil, "staging")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	symlink := plan.ForPublishStrategy(deploy.PublishSymlink)
	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable} {
		step := symlink.Steps[symlink.IndexOf(id)]
		if !step.Skipped {
			t.Errorf("%s must be skipped for a symlink activation with nothing to migrate", id)
		}
		if step.SkipReason == "" {
			t.Errorf("%s must say why it was skipped", id)
		}
	}

	inPlace := plan.ForPublishStrategy(deploy.PublishInPlace)
	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable} {
		if inPlace.Steps[inPlace.IndexOf(id)].Skipped {
			t.Errorf("%s must run in place: the docroot itself is rewritten while serving", id)
		}
	}
}

func TestPlanForPublishStrategyKeepsTheWindowWhenTheDeployMigrates(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable, deploy.TaskDBMigrate} {
		stage, _ := deploy.StageForTask(id)
		deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Stage: stage, Command: "true # " + id})
	}
	plan, err := deploy.BuildPlanForTest(recipe, nil, "staging")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	symlink := plan.ForPublishStrategy(deploy.PublishSymlink)
	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable} {
		if symlink.Steps[symlink.IndexOf(id)].Skipped {
			t.Errorf("%s must run: a schema migration against a serving site is what the window is for", id)
		}
	}
}

// The window has to close *before* the swap for a symlink activation, not after
// it: the flag lives in the release being served, and the swap changes which
// release that is. Closing it afterwards removes a flag the incoming release
// never had and leaves one in the release the swap replaced — so the window would
// protect nothing but the release it just retired, and a rollback onto that
// release would serve maintenance mode to every visitor.
//
// In place there is one directory throughout: the docroot is rewritten while it
// is being served, so the window must stay open across the activation.
func TestPlanClosesTheMaintenanceWindowBeforeASymlinkSwap(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	for _, id := range []string{
		deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable,
		deploy.TaskDBMigrate, deploy.TaskActivate,
	} {
		stage, _ := deploy.StageForTask(id)
		deploy.OverrideTaskForTest(&recipe, id, deploy.Task{ID: id, Stage: stage, Command: "true # " + id})
	}
	plan, err := deploy.BuildPlanForTest(recipe, nil, "staging")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	symlink := plan.ForPublishStrategy(deploy.PublishSymlink)
	enable, disable, activate := symlink.IndexOf(deploy.TaskMaintenanceEnable),
		symlink.IndexOf(deploy.TaskMaintenanceDisable),
		symlink.IndexOf(deploy.TaskActivate)
	if enable >= disable || disable >= activate {
		t.Fatalf("the symlink window must close before the swap: enable=%d disable=%d activate=%d",
			enable, disable, activate)
	}

	inPlace := plan.ForPublishStrategy(deploy.PublishInPlace)
	enable, disable, activate = inPlace.IndexOf(deploy.TaskMaintenanceEnable),
		inPlace.IndexOf(deploy.TaskMaintenanceDisable),
		inPlace.IndexOf(deploy.TaskActivate)
	if enable >= activate || activate >= disable {
		t.Fatalf("the in-place window must stay open across the activation: enable=%d activate=%d disable=%d",
			enable, activate, disable)
	}
}

// The skip has to reach the executor, not just the plan: the maintenance command
// would otherwise run on a target that needs no window.
func TestExecutorDoesNotRunASkippedMaintenanceStep(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "maintenance-ran")
	recipe := deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskMaintenanceEnable, Stage: deploy.StagePublish, Command: "touch " + marker},
		{ID: deploy.TaskRecord, Stage: deploy.StagePublish, Command: "true"},
	})
	plan, err := deploy.BuildPlanForTest(recipe, nil, "staging")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	plan = plan.ForPublishStrategy(deploy.PublishSymlink)

	if _, err := deploy.NewExecutor(
		deploy.HostForTest(t.TempDir(), deploy.LocalRunner{}),
		deploy.Options{Remote: "local", Publish: deploy.PublishSymlink},
		io.Discard,
	).Run(context.Background(), plan, deploy.NewVars(), deploy.NewReleaseForTest("1", "abc", "local")); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a symlink activation with nothing to migrate ran maintenance:enable")
	}
}
