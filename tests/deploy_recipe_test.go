package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestDefaultRecipeDeclaresEveryNeutralTaskInStageOrder(t *testing.T) {
	want := []string{
		"deploy:check", "deploy:lock", "deploy:release", "deploy:code", "deploy:shared", "deploy:writable",
		"build:vendors", "build:patches", "build:compile", "build:frontend", "build:assets", "deploy:artifact",
		"maintenance:enable", "app:workers:pause", "db:backup", "app:configure", "db:migrate",
		"publish:activate", "app:cache:flush", "app:workers:resume", "maintenance:disable", "deploy:record",
		"deploy:verify", "deploy:cleanup", "deploy:unlock",
	}
	got := deploy.TaskIDList()
	if len(got) != len(want) {
		t.Fatalf("TaskIDList() has %d ids, want %d: %v", len(got), len(want), got)
	}
	for idx, id := range want {
		if got[idx] != id {
			t.Fatalf("TaskIDList()[%d] = %q, want %q", idx, got[idx], id)
		}
		if !deploy.IsKnownTaskID(id) {
			t.Fatalf("IsKnownTaskID(%q) = false", id)
		}
	}

	recipe := deploy.DefaultRecipe()
	if recipe.ID != "default" || recipe.Extends != "" {
		t.Fatalf("default recipe = %+v, want id=default with no parent", recipe)
	}
	for _, id := range want {
		task := recipe.Task(id)
		if task.ID != id {
			t.Fatalf("default recipe does not declare %q", id)
		}
		if task.Stage == "" {
			t.Fatalf("task %q has no stage", id)
		}
		if task.Title == "" {
			t.Fatalf("task %q has no title for plan output", id)
		}
	}

	// The build stage is deliberately empty until a framework recipe fills it,
	// and an empty task must be reported as skipped rather than failing.
	if !recipe.Task(deploy.TaskCompile).IsEmpty() {
		t.Fatal("default recipe must leave build:compile empty for a framework recipe to fill")
	}
	if _, ok := deploy.StageForTask(deploy.TaskActivate); !ok {
		t.Fatal("publish:activate must have a stage")
	}
	if stage, _ := deploy.StageForTask(deploy.TaskActivate); stage != deploy.StagePublish {
		t.Fatalf("publish:activate stage = %q, want publish", stage)
	}
}

func TestNoFrameworkNameAppearsInDeployPackage(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "internal", "deploy", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob internal/deploy: %v (%d files)", err, len(files))
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		lowered := strings.ToLower(string(content))
		for _, name := range []string{"magento", "laravel", "symfony", "wordpress", "prestashop", "django"} {
			if strings.Contains(lowered, name) {
				t.Errorf("%s mentions framework %q; framework behaviour belongs in internal/frameworks", file, name)
			}
		}
	}
}
