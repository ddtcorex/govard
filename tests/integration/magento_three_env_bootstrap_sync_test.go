//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
	"govard/internal/engine"
)

func mergeEnvSlices(base []string, extra []string) []string {
	merged := append([]string{}, base...)
	merged = append(merged, extra...)
	return merged
}

func addTestRemotesToGovardConfig(t *testing.T, projectDir string) {
	t.Helper()

	config, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err != nil {
		t.Fatalf("load govard config: %v", err)
	}

	config.Remotes = map[string]engine.RemoteConfig{
		"dev": {
			Host: "dev.example.com",
			User: "deploy",
			Path: "/var/www/html",
			Capabilities: engine.RemoteCapabilities{
				Files:  true,
				Media:  true,
				DB:     true,
				Deploy: false,
			},
		},
		"staging": {
			Host: "staging.example.com",
			User: "deploy",
			Path: "/srv/www/staging",
			Capabilities: engine.RemoteCapabilities{
				Files:  true,
				Media:  true,
				DB:     true,
				Deploy: false,
			},
		},
	}

	encoded, err := yaml.Marshal(&config)
	if err != nil {
		t.Fatalf("marshal updated govard config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), encoded, 0o644); err != nil {
		t.Fatalf("write .govard.yml remotes: %v", err)
	}
}

func TestMagentoThreeEnvironmentInitBootstrapAndSyncOptions(t *testing.T) {
	env := NewTestEnvironment(t)

	localProject := env.CreateProjectFromFixture(t, "magento2/options-local", "m2-local")
	devProject := env.CreateProjectFromFixture(t, "magento2/options-dev", "m2-dev")
	stagingProject := env.CreateProjectFromFixture(t, "magento2/options-staging", "m2-staging")

	projects := []string{localProject, devProject, stagingProject}
	for _, projectDir := range projects {
		initResult := env.RunGovard(t, projectDir, "init", "--recipe", "magento2")
		initResult.AssertSuccess(t)

		configPath := filepath.Join(projectDir, ".govard.yml")
		configData, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("expected .govard.yml after init at %s: %v", configPath, err)
		}
		assertContains(t, string(configData), "recipe: magento2")

		addTestRemotesToGovardConfig(t, projectDir)

		autoloadPath := filepath.Join(projectDir, "vendor", "autoload.php")
		if err := os.MkdirAll(filepath.Dir(autoloadPath), 0o755); err != nil {
			t.Fatalf("create vendor dir: %v", err)
		}
		if err := os.WriteFile(autoloadPath, []byte("<?php\n"), 0o644); err != nil {
			t.Fatalf("write vendor/autoload.php fixture: %v", err)
		}
	}

	seedDumpPath := filepath.Join(stagingProject, "fixtures", "seed.sql")
	if err := os.MkdirAll(filepath.Dir(seedDumpPath), 0o755); err != nil {
		t.Fatalf("create dump fixture dir: %v", err)
	}
	if err := os.WriteFile(seedDumpPath, []byte("CREATE TABLE seed_env(id INT);\n"), 0o644); err != nil {
		t.Fatalf("write dump fixture: %v", err)
	}

	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	extraEnv := mergeEnvSlices(shim.Env(), isolatedHomeEnv(t))

	localBootstrap := env.RunGovardWithEnv(
		t,
		localProject,
		extraEnv,
		"bootstrap",
		"--clone",
		"--code-only",
		"--environment", "dev",
		"--skip-up",
		"--no-composer",
		"--no-admin",
	)
	localBootstrap.AssertSuccess(t)

	devBootstrap := env.RunGovardWithEnv(
		t,
		devProject,
		extraEnv,
		"bootstrap",
		"--clone",
		"--code-only",
		"--environment", "staging",
		"--skip-up",
		"--no-composer",
		"--no-admin",
	)
	devBootstrap.AssertSuccess(t)

	stagingBootstrap := env.RunGovardWithEnv(
		t,
		stagingProject,
		extraEnv,
		"bootstrap",
		"--clone",
		"--environment", "dev",
		"--skip-up",
		"--no-composer",
		"--no-admin",
		"--no-media",
		"--db-dump", "fixtures/seed.sql",
	)
	stagingBootstrap.AssertSuccess(t)

	bootstrapLogs := shim.ReadLog(t)
	assertContains(t, bootstrapLogs, "deploy@dev.example.com:/var/www/html/")
	assertContains(t, bootstrapLogs, "deploy@staging.example.com:/srv/www/staging/")
	assertContains(t, bootstrapLogs, "docker|exec -i -e MYSQL_PWD=magento")

	syncPlan := env.RunGovardWithEnv(
		t,
		localProject,
		extraEnv,
		"sync",
		"--source", "staging",
		"--destination", "local",
		"--full",
		"--path", "app/code",
		"--include", "app/*",
		"--exclude", "vendor/",
		"--delete",
		"--plan",
	)
	syncPlan.AssertSuccess(t)
	planOut := syncPlan.Stdout
	assertContains(t, planOut, "Sync Plan Summary")
	assertContains(t, planOut, "scopes: files, media, db")
	assertContains(t, planOut, "path filter: app/code")
	assertContains(t, planOut, "include patterns: app/*")
	assertContains(t, planOut, "exclude patterns: vendor/")
	assertContains(t, planOut, "delete mode: enabled")

	syncRun := env.RunGovardWithEnv(
		t,
		localProject,
		extraEnv,
		"sync",
		"--source", "dev",
		"--destination", "local",
		"--file",
		"--path", "app/code",
		"--include", "app/*",
		"--exclude", "vendor/",
		"--delete",
	)
	syncRun.AssertSuccess(t)

	syncLogs := shim.ReadLog(t)
	assertContains(t, syncLogs, "rsync|")
	assertContains(t, syncLogs, "--include app/*")
	assertContains(t, syncLogs, "--exclude vendor/")
	assertContains(t, syncLogs, "--delete")
	assertContains(t, syncLogs, "deploy@dev.example.com:/var/www/html/app/code/")
}
