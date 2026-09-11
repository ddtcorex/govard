package deploy

import (
	"context"
	"io"
)

// Stage is one phase of the pipeline. Stages are fixed and ordered; a task
// belongs to exactly one.
type Stage string

const (
	StagePrepare Stage = "prepare"
	StageBuild   Stage = "build"
	StagePublish Stage = "publish"
	StageVerify  Stage = "verify"
	StageCleanup Stage = "cleanup"
)

// StageOrder is the canonical execution order of the stages.
var StageOrder = []Stage{StagePrepare, StageBuild, StagePublish, StageVerify, StageCleanup}

// RunOn selects where a step executes.
type RunOn string

const (
	// RunRemote executes on the target host. It is the default.
	RunRemote RunOn = "remote"
	// RunLocal executes on the machine that invoked govard: a developer laptop
	// or a CI runner.
	RunLocal RunOn = "local"
)

// The neutral task ids. They carry no framework name: a framework recipe fills
// the ones it supports and leaves the rest empty, and the executor reports an
// empty task as skipped rather than failing.
const (
	TaskCheck    = "deploy:check"
	TaskLock     = "deploy:lock"
	TaskRelease  = "deploy:release"
	TaskCode     = "deploy:code"
	TaskShared   = "deploy:shared"
	TaskWritable = "deploy:writable"

	TaskVendors  = "build:vendors"
	TaskPatches  = "build:patches"
	TaskCompile  = "build:compile"
	TaskFrontend = "build:frontend"
	TaskAssets   = "build:assets"
	TaskArtifact = "deploy:artifact"

	TaskMaintenanceEnable  = "maintenance:enable"
	TaskWorkersPause       = "app:workers:pause"
	TaskDBBackup           = "db:backup"
	TaskAppConfigure       = "app:configure"
	TaskDBMigrate          = "db:migrate"
	TaskActivate           = "publish:activate"
	TaskAppCacheFlush      = "app:cache:flush"
	TaskWorkersResume      = "app:workers:resume"
	TaskMaintenanceDisable = "maintenance:disable"
	TaskRecord             = "deploy:record"

	TaskVerify  = "deploy:verify"
	TaskCleanup = "deploy:cleanup"
	TaskUnlock  = "deploy:unlock"
)

// taskOrder is the canonical execution order of every task id. The pipeline is
// defined by this slice, not by the order a recipe happens to declare tasks in,
// so a framework cannot reorder the lifecycle by accident.
var taskOrder = []string{
	TaskCheck, TaskLock, TaskRelease, TaskCode, TaskShared, TaskWritable,
	TaskVendors, TaskPatches, TaskCompile, TaskFrontend, TaskAssets, TaskArtifact,
	TaskMaintenanceEnable, TaskWorkersPause, TaskDBBackup, TaskAppConfigure, TaskDBMigrate,
	TaskActivate, TaskAppCacheFlush, TaskWorkersResume, TaskMaintenanceDisable, TaskRecord,
	TaskVerify, TaskCleanup, TaskUnlock,
}

// taskStages assigns every task id its stage.
var taskStages = map[string]Stage{
	TaskCheck: StagePrepare, TaskLock: StagePrepare, TaskRelease: StagePrepare,
	TaskCode: StagePrepare, TaskShared: StagePrepare, TaskWritable: StagePrepare,

	TaskVendors: StageBuild, TaskPatches: StageBuild, TaskCompile: StageBuild,
	TaskFrontend: StageBuild, TaskAssets: StageBuild, TaskArtifact: StageBuild,

	TaskMaintenanceEnable: StagePublish, TaskWorkersPause: StagePublish, TaskDBBackup: StagePublish,
	TaskAppConfigure: StagePublish, TaskDBMigrate: StagePublish, TaskActivate: StagePublish,
	TaskAppCacheFlush: StagePublish, TaskWorkersResume: StagePublish,
	TaskMaintenanceDisable: StagePublish, TaskRecord: StagePublish,

	TaskVerify: StageVerify,

	TaskCleanup: StageCleanup, TaskUnlock: StageCleanup,
}

// TaskIDList returns every task id in canonical execution order.
func TaskIDList() []string {
	ids := make([]string, len(taskOrder))
	copy(ids, taskOrder)
	return ids
}

// IsKnownTaskID reports whether id is one of the neutral task ids.
func IsKnownTaskID(id string) bool {
	_, ok := taskStages[id]
	return ok
}

// StageForTask returns the stage of a known task id.
func StageForTask(id string) (Stage, bool) {
	stage, ok := taskStages[id]
	return stage, ok
}

// StepContext is the state one step executes against. A step only ever reads
// what the executor set.
type StepContext struct {
	Host    Host
	Runner  Runner
	Vars    Vars
	Release *Release
	Opts    Options
	Out     io.Writer
	// WorkDir is the local checkout a step inspects (for example the
	// `.gitmodules` probe); empty means the process working directory.
	WorkDir string
	// Checks are the recipe's verifications, carried by the verify step. The
	// executor fills this from the step; a direct call (rollback) can too.
	Checks []Check
	// Notes collects human-readable findings a step wants the operator to see
	// (`govard deploy check` prints them).
	Notes []string
}

// TaskFunc is a core step implemented in Go rather than as a shell command.
// Core steps exist for work that needs decisions (which publish strategy, which
// release to prune) or that must happen on the machine running govard.
type TaskFunc func(ctx context.Context, sc *StepContext) error

// Task is one step of the pipeline as a recipe declares it.
type Task struct {
	// ID is one of the neutral task ids.
	ID string
	// Stage is derived from the id; a recipe does not choose it.
	Stage Stage
	// Title describes the step for `govard deploy plan` output.
	Title string
	// Command is a shell template. Empty means the step is not implemented by
	// this recipe and will be reported as skipped.
	Command string
	// Core is a Go implementation. It takes precedence over Command.
	Core TaskFunc
	// RunOn selects local or remote execution. The zero value is RunRemote.
	RunOn RunOn
	// Optional marks a step whose failure must not fail the deploy.
	Optional bool
}

// IsEmpty reports whether the recipe left this step unimplemented.
func (t Task) IsEmpty() bool {
	return t.Command == "" && t.Core == nil
}

// Check is one recipe-provided verification, run by the verify stage after the
// engine's own revision and shared-file checks (spec 11).
//
// The core cannot know how to touch a framework's real dependencies: the check
// that matters is one that needs the application to answer with its database and
// its configuration in place, not a binary that exits zero without either. The
// recipe therefore declares the command, and the engine only runs it, records it
// and fails on it.
type Check struct {
	// ID names the check in the release record and in the failure message.
	ID string
	// Title is the human description carried by the recorded result.
	Title string
	// Command is a shell template expanded with the deploy variables, exactly
	// like a recipe task command.
	Command string
	// OnlyForPublishStrategy limits the check to one strategy; empty means
	// every run. An in-place check has no meaning on a symlink target.
	OnlyForPublishStrategy string
}

// Recipe is a task list plus the defaults a framework contributes. Extends is
// documentation for `plan` output; the resolved recipe is built by the caller
// that owns the framework registry, because internal/deploy must not import
// internal/frameworks.
type Recipe struct {
	ID      string
	Extends string
	Tasks   []Task
	Hooks   []Hook
	// Checks are the framework's post-publish verifications. The core runs them
	// in `deploy:verify`, in declaration order, after its own checks.
	Checks []Check
	// Defaults are layered under the project's deploy settings (see
	// WithRecipeDefaults). A value may be an ArgsSpec, which is rendered into
	// "<key>_args" rather than stored.
	Defaults map[string]any
	// Restore is the command template that loads the release's recorded
	// database dump back. It is empty for a framework with no dump support, and
	// `rollback --with-db` refuses rather than guessing.
	Restore string
	// Sandbox is what the framework needs a `govard deploy sandbox` container to
	// provide beyond its profile: the extensions and services the recipe's own
	// commands depend on. The core renders them and never interprets them.
	Sandbox SandboxRequirements
}

// Task returns the declared task with the given id, or the zero Task when this
// recipe does not declare it. A zero Task has an empty ID, which is how callers
// tell "not declared" from "declared but unimplemented".
func (r Recipe) Task(id string) Task {
	for _, task := range r.Tasks {
		if task.ID == id {
			return task
		}
	}
	return Task{}
}

// ReplaceTask swaps a declared task for a modified copy. Replacing rather than
// appending is what keeps a recipe that starts from DefaultRecipe() equivalent
// to one that declares its tasks explicitly: the task is still declared once.
// The receiver is a pointer because a recipe is assembled into a copy of the
// default pipeline.
func (r *Recipe) ReplaceTask(task Task) {
	for idx := range r.Tasks {
		if r.Tasks[idx].ID == task.ID {
			r.Tasks[idx] = task
			return
		}
	}
	r.Tasks = append(r.Tasks, task)
}

// DefaultRecipe declares every neutral task. The framework-specific steps are
// left empty on purpose: a framework recipe fills them, and an unfilled step is
// skipped instead of failing.
func DefaultRecipe() Recipe {
	tasks := make([]Task, 0, len(taskOrder))
	for _, id := range taskOrder {
		tasks = append(tasks, Task{
			ID:    id,
			Stage: taskStages[id],
			Title: defaultTaskTitles[id],
			RunOn: RunRemote,
		})
	}
	recipe := Recipe{ID: "default", Tasks: tasks}
	wireCoreTasks(&recipe)
	return recipe
}

// wireCoreTasks attaches the Go implementations that are framework-agnostic.
// Everything a framework must supply stays empty, so an unimplemented step is
// reported as skipped rather than failing the deploy.
func wireCoreTasks(recipe *Recipe) {
	core := map[string]TaskFunc{
		TaskCheck:    CoreCheck,
		TaskLock:     CoreLock,
		TaskUnlock:   CoreUnlock,
		TaskRelease:  CoreRelease,
		TaskCode:     CoreCode,
		TaskShared:   CoreShared,
		TaskWritable: CoreWritable,
		TaskActivate: CoreActivate,
		TaskArtifact: CoreArtifact,
		TaskRecord:   CoreRecord,
		TaskVerify:   CoreVerify,
		TaskCleanup:  CoreCleanup,
	}
	for idx := range recipe.Tasks {
		if fn, ok := core[recipe.Tasks[idx].ID]; ok {
			recipe.Tasks[idx].Core = fn
		}
	}
}

// RecipeForTest builds a recipe from an explicit task list. Tests use it to
// exercise the executor without a framework recipe.
func RecipeForTest(id string, tasks []Task) Recipe {
	resolved := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Stage == "" {
			task.Stage = taskStages[task.ID]
		}
		if task.RunOn == "" {
			task.RunOn = RunRemote
		}
		resolved = append(resolved, task)
	}
	return Recipe{ID: id, Tasks: resolved}
}

// OverrideTaskForTest replaces one task in a recipe. Tests use it to inject a
// deliberate failure at a chosen point in the pipeline.
func OverrideTaskForTest(recipe *Recipe, id string, task Task) {
	for idx := range recipe.Tasks {
		if recipe.Tasks[idx].ID == id {
			recipe.Tasks[idx] = task
			return
		}
	}
	recipe.Tasks = append(recipe.Tasks, task)
}

// defaultTaskTitles are the human descriptions shown by `govard deploy plan`.
var defaultTaskTitles = map[string]string{
	TaskCheck:              "preflight: connectivity, permissions, layout",
	TaskLock:               "acquire the deploy lock",
	TaskRelease:            "create the release directory",
	TaskCode:               "materialise the revision from the git mirror",
	TaskShared:             "link shared files and directories",
	TaskWritable:           "apply write permissions and ownership",
	TaskVendors:            "install dependencies",
	TaskPatches:            "apply patches",
	TaskCompile:            "generate code",
	TaskFrontend:           "build frontend assets",
	TaskAssets:             "deploy static assets",
	TaskArtifact:           "receive a prebuilt artifact",
	TaskMaintenanceEnable:  "enable maintenance mode",
	TaskWorkersPause:       "pause background workers",
	TaskDBBackup:           "back up the database",
	TaskAppConfigure:       "import application configuration",
	TaskDBMigrate:          "run schema and data migrations",
	TaskActivate:           "activate the new release",
	TaskAppCacheFlush:      "flush caches",
	TaskWorkersResume:      "resume background workers",
	TaskMaintenanceDisable: "disable maintenance mode",
	TaskRecord:             "write the release record and history",
	TaskVerify:             "verify the live release",
	TaskCleanup:            "prune old releases",
	TaskUnlock:             "release the deploy lock",
}
