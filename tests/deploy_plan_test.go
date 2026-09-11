package tests

import (
	"errors"
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
