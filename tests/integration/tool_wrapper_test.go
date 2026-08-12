//go:build integration
// +build integration

package integration

import (
	"fmt"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestGlobalWrapperCommandsExecIntoOwnContainerForNodeRuntimeFrameworks(t *testing.T) {
	env := NewTestEnvironment(t)

	for _, framework := range []string{"nextjs", "emdash"} {
		t.Run(framework, func(t *testing.T) {
			projectDir := env.CreateNextJSProject(t, "wrapper-"+framework)
			defer env.CleanupProject(t, "wrapper-"+framework)

			CreateGovardConfig(t, projectDir, engine.Config{
				ProjectName: "sample-" + framework,
				Framework:   framework,
				Domain:      "sample-" + framework + ".test",
			})

			shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

			result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "tool", "npm", "--version")
			result.AssertSuccess(t)

			logs := shim.ReadLog(t)
			want := fmt.Sprintf("docker|exec -i -w /app sample-%s-web-1 npm --version", framework)
			assertContains(t, logs, want)
			if strings.Contains(logs, "node:") {
				t.Fatalf("%s must exec into its own application container, not a one-shot Node image:\n%s", framework, logs)
			}
		})
	}
}

func TestFrameworkWrapperFrameworkGuardsForMagentoProject(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "wrapper-guards-m2")

	tests := []struct {
		command           string
		frameworkExpected string
	}{
		{command: "artisan", frameworkExpected: "laravel"},
		{command: "cake", frameworkExpected: "cakephp"},
		{command: "drush", frameworkExpected: "drupal"},
		{command: "shopware", frameworkExpected: "shopware"},
		{command: "symfony", frameworkExpected: "symfony"},
		{command: "wp", frameworkExpected: "wordpress"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			result := env.RunGovard(t, projectDir, "tool", tt.command, "--version")
			if result.Success() {
				t.Fatalf("expected %s command to fail on magento2 project", tt.command)
			}
			output := result.Stdout + result.Stderr
			assertContains(t, output, fmt.Sprintf("the '%s' command is only available for %s projects", tt.command, tt.frameworkExpected))
		})
	}
}

func TestGlobalWrapperCommandsUseConfiguredNodeRuntime(t *testing.T) {
	env := NewTestEnvironment(t)

	tests := []struct {
		command       string
		arg           string
		dockerCommand string
	}{
		{command: "npm", arg: "--version", dockerCommand: "npm --version"},
		{command: "npx", arg: "--version", dockerCommand: "npx --version"},
		{command: "pnpm", arg: "--version", dockerCommand: "corepack pnpm --version"},
		{command: "yarn", arg: "--version", dockerCommand: "yarn --version"},
		{command: "grunt", arg: "--version", dockerCommand: "npx grunt --version"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "wrapper-"+tt.command+"-m2")
			shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

			result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "tool", tt.command, tt.arg)
			result.AssertSuccess(t)

			config, _, err := engine.LoadConfigFromDir(projectDir, true)
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			logs := shim.ReadLog(t)
			want := fmt.Sprintf(
				"docker|run --rm -i --user %d:%d -v %s:/var/www/html -w /var/www/html node:%s-alpine %s",
				config.Stack.UserID, config.Stack.GroupID, projectDir, config.Stack.NodeVersion, tt.dockerCommand,
			)
			assertContains(t, logs, want)
		})
	}
}
