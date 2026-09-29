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

// TestResolveBaseOptionsLayersConfiguredSandboxDeploySettings pins the allowlist
// at its most consequential key: a configured `deploy:` block may state the
// rehearsal's retention, but it may not restate the PHP series the container
// actually shipped.
func TestResolveBaseOptionsLayersConfiguredSandboxDeploySettings(t *testing.T) {
	restore := deploy.StubResolveSyntheticSandboxRemoteForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{
			Host: "127.0.0.1", User: deploy.SandboxUser, Sandbox: true,
			Deploy: &engine.DeployConfig{
				Branch:     "main",
				Repository: deploy.SandboxRepoPath,
				Settings:   map[string]any{"php_version": "8.4", "owner": "1000:1000"},
			},
		}, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	cfg := engine.Config{
		ProjectName: "sample-project",
		Remotes: engine.RemoteConfigMap{"sandbox": {
			Deploy: &engine.DeployConfig{KeepReleases: 7, Settings: map[string]any{"php_version": "8.9", "owner": "0:0"}},
		}},
	}

	options, err := deploy.ResolveOptionsForTest(cfg, "sandbox", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	if options.KeepReleases != 7 {
		t.Errorf("options.KeepReleases = %d, want 7 from the configured deploy block", options.KeepReleases)
	}
	if options.Settings["php_version"] != "8.4" {
		t.Errorf("options.Settings[php_version] = %v, want the container's 8.4: the PHP series is pinned, not configurable", options.Settings["php_version"])
	}
	if options.Settings["owner"] != "1000:1000" {
		t.Errorf("options.Settings[owner] = %v, want the container's 1000:1000", options.Settings["owner"])
	}
}
