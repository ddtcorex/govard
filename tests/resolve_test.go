package tests

import (
	"context"
	"testing"

	"govard/internal/deploy"
	"govard/internal/engine"
)

func TestResolveBaseOptionsResolvesSandbox(t *testing.T) {
	restore := deploy.StubResolveSyntheticSandboxRemoteForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{
			Host: "127.0.0.1", User: deploy.SandboxUser, Sandbox: true,
			Deploy: &engine.DeployConfig{Branch: "main", Repository: deploy.SandboxRepoPath},
		}, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	options, err := deploy.ResolveOptionsForTest(engine.Config{ProjectName: "sample-project"}, "sandbox", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	if options.Branch != "main" || options.Repository != deploy.SandboxRepoPath {
		t.Fatalf("options = %+v, want the synthetic remote's deploy settings", options)
	}
}
