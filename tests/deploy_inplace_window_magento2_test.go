package tests

import (
	"testing"

	"govard/internal/deploy"
	"govard/internal/frameworks/magento2"
)

// In place the docroot itself is rewritten while it serves, so the window cannot
// depend on the migration probe: the probe answers a question about the schema,
// not about the rewrite. The rule lives in the plan, which is what makes the plan
// an operator reviews with `govard deploy plan` the plan that runs.
//
// (The subject file `deploy_maintenance_magento2_test.go` already covers where
// the window is set and what a first deploy without a served application does;
// this file pins that the probe cannot switch it off in place.)
func TestMagento2InPlaceDeployAlwaysOpensTheMaintenanceWindow(t *testing.T) {
	plan, err := deploy.BuildPlanForTest(magento2.DeployRecipe(), nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	inPlace := plan.ForPublishStrategy(deploy.PublishInPlace)

	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable} {
		if step := conditionalPlanStepByID(t, inPlace, id); step.NeedsMigration {
			t.Errorf("%s must not depend on the migration probe in place", id)
		}
	}
	// Everything else the probe gates stays gated: the schema question is still
	// the probe's to answer.
	for _, id := range []string{deploy.TaskDBMigrate, deploy.TaskAppConfigure, deploy.TaskWorkersPause} {
		if step := conditionalPlanStepByID(t, inPlace, id); !step.NeedsMigration {
			t.Errorf("%s must stay gated on the migration probe", id)
		}
	}
}
