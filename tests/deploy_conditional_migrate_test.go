package tests

import (
	"testing"

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
