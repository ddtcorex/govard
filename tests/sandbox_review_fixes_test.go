package tests

import (
	"bytes"
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

// Docker picks a fresh web port for every new container, so the base URL stored
// in a kept database names a port nothing listens on any more.
func TestPopulatedVolumeStillPointsTheDatabaseAtTheNewWebPort(t *testing.T) {
	request := seededMediaRequest(t)
	request.SeedEnvSource = "app/etc/env.php"
	var gotBaseURL string
	request.DBRewrite = func(_ []byte, baseURL string) []string {
		gotBaseURL = baseURL
		return []string{"UPDATE core_config_data SET value='x'"}
	}
	fake := freshSandboxFake()
	fake.answers["80/tcp"] = "127.0.0.1:32771\n"
	fake.answers["cat app/etc/env.php"] = "<?php return [];\n"
	fake.answers["information_schema.tables"] = "371\n"
	existingVolume(fake, request.ProjectName, "")
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if fake.has("command -v mariadb-dump") {
		t.Fatal("the populated database must not be re-imported")
	}
	if gotBaseURL != "http://127.0.0.1:32771/" {
		t.Fatalf("the kept database must be pointed at the new web port, got %q", gotBaseURL)
	}
}

func TestPopulatedVolumeWithAStoppedOriginSaysTheBaseURLMayBeStale(t *testing.T) {
	request := seededMediaRequest(t)
	request.SeedOriginRunning = false
	request.DBRewrite = func([]byte, string) []string { return []string{"UPDATE t SET v=1"} }
	var out bytes.Buffer
	request.Out = &out
	fake := freshSandboxFake()
	fake.answers["80/tcp"] = "127.0.0.1:32771\n"
	fake.answers["information_schema.tables"] = "371\n"
	existingVolume(fake, request.ProjectName, "")
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("a stopped origin must not fail a sandbox that has its data: %v", err)
	}
	if !strings.Contains(out.String(), "base URL") || !strings.Contains(out.String(), "--reseed") {
		t.Fatalf("want a note about the stale base URL naming --reseed, got %q", out.String())
	}
}

func TestRefusedSeriesOnRecreateKeepsTheWorkingContainer(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.Recreate = true
	request.DB = "mariadb:10.11"
	fake := reusedFullSandbox()
	existingVolume(fake, request.ProjectName, "mariadb:10.6")
	_, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request)
	if err == nil {
		t.Fatal("a 10.6 volume must refuse 10.11")
	}
	if fake.has("rm --force") || fake.has("container rm") || fake.has(" rm ") {
		t.Fatalf("the working container must survive a refused --recreate: %v", fake.calls)
	}
}
