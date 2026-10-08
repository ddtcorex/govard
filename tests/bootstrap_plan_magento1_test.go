package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestBuildBootstrapRemotePlanIncludesConfigAutoForMagento1(t *testing.T) {
	plan, err := cmd.BuildBootstrapRemotePlanForTest(engine.Config{
		Framework:   "magento1",
		ProjectName: "sample-project",
		Domain:      "sample.test",
	}, cmd.DefaultBootstrapRuntimeOptionsForTest())
	if err != nil {
		t.Fatalf("build bootstrap remote plan: %v", err)
	}

	found := false
	for _, command := range plan.Commands {
		if strings.Contains(command, "govard config auto") {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected Magento 1 bootstrap plan to include govard config auto")
	}
}

func TestBootstrapPlanNotesPrefixResolvedAtRunTime(t *testing.T) {
	for _, stream := range []bool{true, false} {
		opts := cmd.DefaultBootstrapRuntimeOptionsForTest()
		opts.Source = "staging"
		opts.StreamDB = stream
		opts.NoPII = true
		opts.NoNoise = true
		plan, err := cmd.BuildBootstrapRemotePlanForTest(engine.Config{Framework: "wordpress", ProjectName: "sample-project", Domain: "sample.test"}, opts)
		if err != nil {
			t.Fatalf("build plan: %v", err)
		}
		joined := strings.Join(plan.Descriptions, "\n")
		for _, want := range []string{"table prefix is resolved from the remote at run time", "--no-pii run is refused"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("stream=%v: plan must say %q:\n%s", stream, want, joined)
			}
		}

		opts.NoPII, opts.NoNoise = false, false
		plan, _ = cmd.BuildBootstrapRemotePlanForTest(engine.Config{Framework: "wordpress", ProjectName: "sample-project", Domain: "sample.test"}, opts)
		if strings.Contains(strings.Join(plan.Descriptions, "\n"), "table prefix") {
			t.Fatalf("stream=%v: no filter requested, no prefix note expected", stream)
		}
		opts.NoPII = true
		plan, _ = cmd.BuildBootstrapRemotePlanForTest(engine.Config{Framework: "magento2", ProjectName: "sample-project", Domain: "sample.test"}, opts)
		if strings.Contains(strings.Join(plan.Descriptions, "\n"), "table prefix") {
			t.Fatalf("stream=%v: unprefixed framework needs no prefix note", stream)
		}
	}
}
