package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// A release the deploy command published carries .dep/ (release record). A
// clone bootstrap must not copy that deploy metadata into the local checkout.
func TestBootstrapClonePlanExcludesTheDeployMetadataDirectory(t *testing.T) {
	opts := cmd.DefaultBootstrapRuntimeOptionsForTest()
	opts.Clone = true
	opts.Source = "staging"
	for _, framework := range []string{"magento2", "laravel", "symfony", "wordpress"} {
		plan, err := cmd.BuildBootstrapRemotePlanForTest(engine.Config{Framework: framework, ProjectName: "p", Domain: "p.test"}, opts)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, command := range plan.Commands {
			if strings.Contains(command, "--file") && strings.Contains(command, "--exclude /.dep") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the clone sync must exclude /.dep, plan: %v", framework, plan.Commands)
		}
	}
}
