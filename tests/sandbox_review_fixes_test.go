package tests

import (
	"context"
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/engine"
)

func TestSandboxReuseSettingIsADeclaredSetting(t *testing.T) {
	err := deploy.ValidateSettings(deploy.DefaultRecipe(), map[string]any{"sandbox_reuse_assets": false})
	if err != nil {
		t.Fatalf("the documented off switch must validate, got %v", err)
	}
	if err := deploy.ValidateSettings(deploy.DefaultRecipe(), map[string]any{"sandbox_reuse_assets": "no"}); err == nil {
		t.Fatal("a non-boolean value must be refused")
	}
}

func TestOnlyTheSandboxRemoteMaySetTheSandboxFlag(t *testing.T) {
	remote := engine.RemoteConfig{Host: "prod.example.com", User: "deploy", Path: "/var/www", Port: 22,
		Auth: engine.RemoteAuth{Method: "ssh-agent"}, Sandbox: true}
	config := engine.Config{
		ProjectName: "demo", Framework: "magento2", FrameworkVersion: "2.4.8-p4", Domain: "demo.test",
		Stack: engine.Stack{
			PHPVersion: "8.4", DBVersion: "11.4", NodeVersion: "24", SearchVersion: "3.0",
			Services: engine.Services{DB: "mariadb", Search: "opensearch", WebServer: "nginx", Cache: "none", Queue: "none"},
		},
		Remotes: map[string]engine.RemoteConfig{"prod": remote},
	}
	err := engine.ValidateConfig(config)
	if err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("a real remote carrying sandbox: true must be refused, got %v", err)
	}
	remote.Sandbox = false
	config.Remotes["prod"] = remote
	if err := engine.ValidateConfig(config); err != nil {
		t.Fatalf("an ordinary remote stays valid: %v", err)
	}
}

// A server build has no artifact manifest. The fingerprint then has no basis, so
// the assets task must run every time: two revisions render the same pinned
// command, and reusing the first one's static content would ship stale assets.
func TestServerBuildNeverReusesStaticContent(t *testing.T) {
	hermeticPHP(t)
	work, revision := seedBuildRepo(t)
	enterSandboxCheckout(t, work)
	origin := seedOriginFromCheckout(t, work)
	recipe := assetsRecipe(stampCommand)
	root := t.TempDir()

	run := func() deploy.StepResult {
		host := deploy.HostForTest(root, deploy.LocalRunner{})
		host.Remote.Sandbox = true
		options := deploy.Options{
			Branch: "main", Revision: revision, Repository: origin, Publish: deploy.PublishSymlink,
			KeepReleases: 5, Build: deploy.BuildServer, CommandTimeout: deploy.DefaultCommandTimeout,
			Force: true, Settings: map[string]any{},
		}
		plan, err := deploy.BuildPlanForTest(recipe, nil)
		if err != nil {
			t.Fatal(err)
		}
		vars := deploy.NewVars().Set("revision", revision).Set("branch", "main").SetPath("release_path", "").SetPath("deploy_path", root)
		var out strings.Builder
		outcome, err := deploy.NewExecutor(host, options, &out).Run(context.Background(), plan, vars, deploy.NewRelease("", revision, "main"))
		if err != nil {
			t.Fatalf("deploy: %v\n%s", err, out.String())
		}
		for _, step := range outcome.Steps {
			if step.ID == deploy.TaskAssets {
				return step
			}
		}
		t.Fatal("no assets step in the outcome")
		return deploy.StepResult{}
	}
	run()
	if second := run(); second.Status != deploy.StepOK {
		t.Fatalf("a server build must run the assets task every time, got %s", second.Status)
	}
}
