package tests

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
)

// runCIGenerate runs `govard ci generate` with the given args and returns the
// combined output. It follows the repo's CLI test pattern
// (RootCommandForTest + SetArgs + buffers, see runPlanJSON in
// deploy_plan_json_test.go).
func runCIGenerate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	root := cmd.RootCommandForTest()
	root.SetArgs(append([]string{"ci", "generate"}, args...))
	root.SetOut(out)
	root.SetErr(io.Discard)
	t.Cleanup(func() { root.SetArgs(nil) })
	err := root.Execute()
	return out.String(), err
}

func TestCIGenerateUnknownProvider(t *testing.T) {
	out, err := runCIGenerate(t, "--provider", "jenkins")
	if err == nil {
		t.Fatalf("expected usage error for unknown provider, got output %q", out)
	}
	if !strings.Contains(out+err.Error(), "jenkins") {
		t.Fatalf("error should name the provider, got %q", out)
	}
}

func TestRenderGitLabPipelineStagesAndRules(t *testing.T) {
	cfg := engine.Config{
		Remotes: map[string]engine.RemoteConfig{
			"dev1":       {Branch: "develop", Path: "/home/dev/public_html"},
			"production": {Branch: "master", Path: "/home/prod/public_html"},
		},
	}
	out, err := deploy.RenderGitLabPipelineForTest(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"stages: [integrity, lint, ship, rollback]",
		"ship:dev1", "ship:production",
		"develop", "master",
		"resource_group: dev1", "resource_group: production",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered pipeline missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderGitLabPipelineJobBodies(t *testing.T) {
	// The settings are what WithCILintDefaults produces for a Magento
	// project: recipe defaults under the project's own keys.
	cfg := engine.Config{
		Deploy: engine.DeployConfig{Settings: map[string]any{
			"mage_mode":              "developer",
			"ci_lint_phpcs_standard": "Magento2",
			"ci_lint_paths":          "app/code app/design",
		}},
		Remotes: map[string]engine.RemoteConfig{
			"dev1": {Branch: "develop"},
		},
	}
	out, err := deploy.RenderGitLabPipelineForTest(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"integrity:", "lint:quick", "lint:custom", "app/code", "app/design",
		"rollback:dev1", "when: manual",
		"$SSH_KEY", "$SSH_KNOWN_HOSTS",
		"govard deploy --remote dev1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered pipeline missing %q\n---\n%s", want, out)
		}
	}
	for _, notWant := range []string{"TODO", "TBD", "latest"} {
		if strings.Contains(out, notWant) {
			t.Errorf("rendered pipeline must not contain %q", notWant)
		}
	}
}

// A project whose settings declare no lint scope gets the neutral fallback:
// the language default standard and no scoped paths. Framework specifics
// arrive through recipe defaults (see TestMagento2RecipeDeclaresCILintDefaults),
// never as literals in the emitter.
func TestRenderGitLabPipelineCILintFallback(t *testing.T) {
	cfg := engine.Config{
		Remotes: map[string]engine.RemoteConfig{
			"dev1": {Branch: "develop"},
		},
	}
	out, err := deploy.RenderGitLabPipelineForTest(cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"lint:quick", "lint:custom",
		"--standard=PSR-12",
		"phpstan analyse",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered pipeline missing %q\n---\n%s", want, out)
		}
	}
	for _, notWant := range []string{"app/code", "app/design", "Magento", "TODO", "TBD", "latest"} {
		if strings.Contains(out, notWant) {
			t.Errorf("fallback pipeline must not contain %q", notWant)
		}
	}
}

func TestWithCILintDefaultsLayersRecipeUnderProject(t *testing.T) {
	cfg := engine.Config{
		Deploy: engine.DeployConfig{Settings: map[string]any{
			"ci_lint_paths": "custom/scope",
		}},
	}
	got := deploy.WithCILintDefaults(cfg, map[string]any{
		"ci_lint_phpcs_standard": "Magento2",
		"ci_lint_paths":          "app/code app/design",
		"unrelated":              "ignored",
	})
	if got.Deploy.Settings["ci_lint_phpcs_standard"] != "Magento2" {
		t.Errorf("recipe default not applied: %v", got.Deploy.Settings)
	}
	if got.Deploy.Settings["ci_lint_paths"] != "custom/scope" {
		t.Errorf("project setting must win over the recipe default: %v", got.Deploy.Settings)
	}
	if _, ok := got.Deploy.Settings["unrelated"]; ok {
		t.Errorf("non-lint recipe keys must not leak into the render: %v", got.Deploy.Settings)
	}
}
