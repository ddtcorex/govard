package deploy

import (
	"errors"
	"fmt"
)

// StepKind distinguishes a pipeline task from an inserted hook.
type StepKind string

const (
	StepTask StepKind = "task"
	StepHook StepKind = "hook"
)

// Plan-build failures. They are detected before anything runs, never halfway
// through a deploy.
var (
	// ErrUnknownAnchor is returned when a hook anchors on a task, stage or hook
	// that does not exist in the plan.
	ErrUnknownAnchor = errors.New("unknown hook anchor")
	// ErrDuplicateHook is returned when two hooks share a name.
	ErrDuplicateHook = errors.New("duplicate hook name")
	// ErrHookCycle is returned when hooks anchor on each other in a way that
	// has no valid ordering.
	ErrHookCycle = errors.New("hook ordering cycle")
)

// Step is one entry of the resolved execution plan.
type Step struct {
	ID       string
	Kind     StepKind
	Stage    Stage
	Title    string
	Command  string
	RunOn    RunOn
	Optional bool
	// Source records which layer contributed the step: "recipe" or "config".
	Source string

	// order is the hook ordering key. It is unexported because it only exists
	// to break ties between hooks that share an anchor and a position.
	order int
	// core is the Go implementation of a framework-agnostic task. It travels
	// with the step so the executor does not need the recipe.
	core TaskFunc
}

// Implemented reports whether a step will actually do something: a shell
// command, a Go implementation, or a hook. An unimplemented task is reported as
// skipped by the executor, and `govard deploy plan` shows it as such.
func (s Step) Implemented() bool {
	return s.Command != "" || s.core != nil || s.Kind == StepHook
}

// Plan is the ordered list of steps one deploy will run.
type Plan struct {
	Remote string
	Steps  []Step
}

// StepIDs returns the step ids in execution order, which is what tests and
// `govard deploy plan` both display.
func (p Plan) StepIDs() []string {
	ids := make([]string, 0, len(p.Steps))
	for _, step := range p.Steps {
		ids = append(ids, step.ID)
	}
	return ids
}

// IndexOf returns the position of a step, or -1. A hook id ("hook:<name>") is
// addressable exactly like a task id, which is what makes `--from hook:x` work.
func (p Plan) IndexOf(id string) int {
	for idx, step := range p.Steps {
		if step.ID == id {
			return idx
		}
	}
	return -1
}

// From returns the sub-plan that starts at the first step with the given id,
// including that step. found is false when the id is not in the plan.
//
// It is how a recovery command re-runs part of a pipeline — rollback re-runs
// the publish tail from the existing release directory instead of rebuilding.
func (p Plan) From(id string) (Plan, bool) {
	index := p.IndexOf(id)
	if index < 0 {
		return Plan{}, false
	}
	steps := make([]Step, len(p.Steps)-index)
	copy(steps, p.Steps[index:])
	return Plan{Remote: p.Remote, Steps: steps}, true
}

// Only returns the sub-plan holding just the named steps, in plan order. A name
// that is not in the plan contributes nothing, so a caller that wants an
// optional step to run can ask for it unconditionally.
func (p Plan) Only(ids ...string) Plan {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	steps := make([]Step, 0, len(ids))
	for _, step := range p.Steps {
		if wanted[step.ID] {
			steps = append(steps, step)
		}
	}
	return Plan{Remote: p.Remote, Steps: steps}
}

// TaskPlan returns a one-step plan for a task id, used by commands that run a
// single task outside the pipeline. An id the recipe does not declare produces
// an empty plan rather than an error: the caller decides whether that is a
// refusal.
func TaskPlan(recipe Recipe, id, remote string) Plan {
	plan, err := BuildPlan(recipe, nil, remote)
	if err != nil {
		return Plan{Remote: remote}
	}
	return plan.Only(id)
}

// BuildPlan composes a recipe with hooks into one deterministic plan.
//
// The plan carries exactly the tasks the recipe declares, ordered by the
// canonical task order rather than by declaration order, so a recipe cannot
// reorder the lifecycle. The default recipe declares all 25 ids; a recipe that
// declares fewer simply produces a shorter plan.
func BuildPlan(recipe Recipe, hooks []Hook, remote string) (Plan, error) {
	steps := make([]Step, 0, len(recipe.Tasks))
	for _, id := range taskOrder {
		task := recipe.Task(id)
		if task.ID == "" {
			continue
		}
		stage := task.Stage
		if stage == "" {
			stage = taskStages[id]
		}
		title := task.Title
		if title == "" {
			title = defaultTaskTitles[id]
		}
		steps = append(steps, Step{
			ID:       id,
			Kind:     StepTask,
			Stage:    stage,
			Title:    title,
			Command:  task.Command,
			RunOn:    task.RunOn,
			Optional: task.Optional,
			Source:   "recipe",
			core:     task.Core,
		})
	}

	recipeHooks := make([]Hook, 0, len(recipe.Hooks))
	for _, hook := range recipe.Hooks {
		hook.Source = "recipe"
		recipeHooks = append(recipeHooks, hook)
	}
	all := append(recipeHooks, hooks...)

	ordered, err := insertHooks(steps, all)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Remote: remote, Steps: ordered}, nil
}

// BuildPlanForTest exposes BuildPlan to the tests/ package.
func BuildPlanForTest(recipe Recipe, hooks []Hook, remote string) (Plan, error) {
	return BuildPlan(recipe, hooks, remote)
}

// insertHooks places every hook immediately before or after its anchor.
//
// Ordering is deterministic: hooks sharing an anchor and position are sorted by
// their Order field and then by declaration order. Hooks anchored on other
// hooks are placed after their anchor exists, so hook-on-hook chains resolve
// without the caller having to order them.
func insertHooks(steps []Step, hooks []Hook) ([]Step, error) {
	seen := make(map[string]bool, len(hooks))
	for _, hook := range hooks {
		if hook.Name == "" {
			return nil, fmt.Errorf("%w: a hook has no name", ErrDuplicateHook)
		}
		if seen[hook.HookID()] {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateHook, hook.Name)
		}
		seen[hook.HookID()] = true
	}

	pending := make([]Hook, len(hooks))
	copy(pending, hooks)

	for len(pending) > 0 {
		inserted := 0
		remaining := pending[:0:0]
		for _, hook := range pending {
			index, found, err := anchorIndex(steps, hook)
			if err != nil {
				return nil, err
			}
			if !found {
				remaining = append(remaining, hook)
				continue
			}
			step, err := hookStep(hook, steps)
			if err != nil {
				return nil, err
			}
			steps = insertStepAt(steps, step, index, hook)
			inserted++
		}
		if inserted == 0 {
			// Nothing could be placed: the remaining hooks either anchor on
			// something that does not exist, or anchor on each other. Tell the
			// two apart so the message is actionable.
			pendingNames := make(map[string]bool, len(remaining))
			for _, hook := range remaining {
				pendingNames[hook.Name] = true
			}
			for _, hook := range remaining {
				kind, name, err := parseAnchor(hook.On)
				if err != nil {
					return nil, fmt.Errorf("%w: hook %q: %s", ErrUnknownAnchor, hook.Name, err)
				}
				if kind == AnchorHook && (pendingNames[name] || hookExists(steps, name)) {
					continue
				}
				if kind == AnchorTask {
					return nil, fmt.Errorf("%w: hook %q anchors on unknown task %q", ErrUnknownAnchor, hook.Name, name)
				}
				return nil, fmt.Errorf("%w: hook %q anchors on %q, which is not in the plan", ErrUnknownAnchor, hook.Name, hook.On)
			}
			names := make([]string, 0, len(remaining))
			for _, hook := range remaining {
				names = append(names, hook.Name)
			}
			return nil, fmt.Errorf("%w involving hooks %v", ErrHookCycle, names)
		}
		pending = remaining
	}
	return steps, nil
}

func hookExists(steps []Step, name string) bool {
	wanted := "hook:" + name
	for _, step := range steps {
		if step.ID == wanted {
			return true
		}
	}
	return false
}

// anchorIndex resolves where a hook's anchor currently sits. found is false when
// the anchor is a hook that has not been placed yet.
func anchorIndex(steps []Step, hook Hook) (int, bool, error) {
	kind, name, err := parseAnchor(hook.On)
	if err != nil {
		return 0, false, fmt.Errorf("%w: hook %q: %s", ErrUnknownAnchor, hook.Name, err)
	}
	switch kind {
	case AnchorTask:
		if !IsKnownTaskID(name) {
			return 0, false, fmt.Errorf("%w: hook %q anchors on unknown task %q", ErrUnknownAnchor, hook.Name, name)
		}
		for idx, step := range steps {
			if step.ID == name {
				return idx, true, nil
			}
		}
		return 0, false, nil
	case AnchorHook:
		wanted := "hook:" + name
		for idx, step := range steps {
			if step.ID == wanted {
				return idx, true, nil
			}
		}
		return 0, false, nil
	case AnchorStage:
		stage, _ := stageByName(name)
		return stageAnchorIndex(steps, stage, hook.Position), true, nil
	default:
		return 0, false, fmt.Errorf("%w: hook %q has an unusable anchor", ErrUnknownAnchor, hook.Name)
	}
}

// stageAnchorIndex returns the index where a stage-anchored hook belongs: after
// the last step of the stage, or before the first one.
func stageAnchorIndex(steps []Step, stage Stage, position Position) int {
	first, last := -1, -1
	for idx, step := range steps {
		if step.Stage != stage {
			continue
		}
		if first == -1 {
			first = idx
		}
		last = idx
	}
	if first == -1 {
		return len(steps)
	}
	if position == PositionBefore {
		return first
	}
	return last
}

func hookStep(hook Hook, steps []Step) (Step, error) {
	kind, name, err := parseAnchor(hook.On)
	if err != nil {
		return Step{}, fmt.Errorf("%w: hook %q: %s", ErrUnknownAnchor, hook.Name, err)
	}
	stage := Stage("")
	switch kind {
	case AnchorTask:
		stage = taskStages[name]
	case AnchorHook:
		wanted := "hook:" + name
		for _, step := range steps {
			if step.ID == wanted {
				stage = step.Stage
				break
			}
		}
	case AnchorStage:
		stage, _ = stageByName(name)
	}
	source := hook.Source
	if source == "" {
		source = "config"
	}
	return Step{
		ID:       hook.HookID(),
		Kind:     StepHook,
		Stage:    stage,
		Title:    hook.Name,
		Command:  hook.Run,
		RunOn:    hook.RunOn,
		Optional: hook.Optional,
		Source:   source,
		order:    hook.Order,
	}, nil
}

// insertStepAt places a new step at the anchor, keeping hooks that share an
// anchor and position ordered by Order and then by declaration order.
func insertStepAt(steps []Step, step Step, anchor int, hook Hook) []Step {
	if hook.Position == PositionBefore {
		steps = append(steps, Step{})
		copy(steps[anchor+1:], steps[anchor:])
		steps[anchor] = step
		return steps
	}

	at := anchor + 1
	for at < len(steps) {
		candidate := steps[at]
		if candidate.Kind != StepHook {
			break
		}
		if candidate.order > hook.Order {
			break
		}
		at++
	}
	steps = append(steps, Step{})
	copy(steps[at+1:], steps[at:])
	steps[at] = step
	return steps
}
