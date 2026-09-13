package tests

import (
	"errors"
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/frameworks"
)

// A recipe with no dump command must refuse --db-backup before the run starts.
//
// Silently skipping `db:backup` was the behaviour until this test existed, and
// it is the dangerous one: the executor skips a task that has neither a command
// nor a core implementation without consulting anything, so an operator who
// asked for a backup before a destructive `db:migrate` got a successful deploy,
// a `db:backup … skipped` line, no dump and no error. The remedy is a refusal
// naming the recipe, which is what the deployment docs already promise.
func TestDBBackupIsRefusedForRecipesWithoutADump(t *testing.T) {
	cases := []struct {
		framework string
		// supported is true when the recipe attaches a dump command, so the
		// flag must be accepted.
		supported bool
	}{
		{"magento2", true},
		{"mageos", true}, // inherits magento2's recipe through FrameworkSpec.Parent
		{"wordpress", true},
		{"laravel", false},
		{"symfony", false},
	}

	for _, tc := range cases {
		t.Run(tc.framework, func(t *testing.T) {
			recipe, ok := frameworks.DeployRecipe(tc.framework)
			if !ok {
				t.Fatalf("%s reports no deploy recipe", tc.framework)
			}

			err := deploy.ValidateDBBackup(recipe, deploy.Options{Remote: "production", DBBackup: true})

			if tc.supported {
				if err != nil {
					t.Fatalf("%s provides a dump command but --db-backup was refused: %v", tc.framework, err)
				}
				return
			}

			if err == nil {
				t.Fatalf("%s has no dump command, so --db-backup must be refused rather than skipped in silence", tc.framework)
			}
			if !errors.Is(err, deploy.ErrInvalidConfiguration) {
				t.Errorf("error = %v, want it to wrap ErrInvalidConfiguration so the command layer reports exit 4", err)
			}
			// The message has to be actionable without the reader opening the
			// recipe: which flag, which recipe, and what to do instead.
			for _, want := range []string{"--db-backup", recipe.ID, "db:backup"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q must name %q", err, want)
				}
			}
		})
	}
}

// The neutral default pipeline carries no dump either: a framework that ships no
// recipe gets the same refusal rather than the same silence.
func TestDBBackupIsRefusedForTheDefaultRecipe(t *testing.T) {
	err := deploy.ValidateDBBackup(deploy.DefaultRecipe(), deploy.Options{Remote: "production", DBBackup: true})
	if err == nil {
		t.Fatal("the default recipe has no dump command, so --db-backup must be refused")
	}
	if !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Errorf("error = %v, want it to wrap ErrInvalidConfiguration", err)
	}
}

// Without the flag there is nothing to refuse: a recipe's missing dump only
// matters when the operator asked for one.
func TestDBBackupValidationIsSilentWithoutTheFlag(t *testing.T) {
	for _, framework := range []string{"laravel", "symfony"} {
		recipe, ok := frameworks.DeployRecipe(framework)
		if !ok {
			t.Fatalf("%s reports no deploy recipe", framework)
		}
		if err := deploy.ValidateDBBackup(recipe, deploy.Options{Remote: "production"}); err != nil {
			t.Fatalf("%s: %v (no --db-backup was requested)", framework, err)
		}
	}
}
