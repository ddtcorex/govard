//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStopCommandRunsHooksWithShims(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "stop-m2")
	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})

	localOverride := `hooks:
  pre_stop:
    - name: pre stop marker
      run: "echo pre >> .govard-stop-hooks.log"
  post_stop:
    - name: post stop marker
      run: "echo post >> .govard-stop-hooks.log"
`
	overridePath := filepath.Join(projectDir, ".govard.local.yml")
	if err := os.WriteFile(overridePath, []byte(localOverride), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "stop")
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|compose --project-directory")
	assertContains(t, logs, " stop")

	content, err := os.ReadFile(filepath.Join(projectDir, ".govard-stop-hooks.log"))
	if err != nil {
		t.Fatalf("failed to read stop hook log: %v", err)
	}
	if strings.TrimSpace(string(content)) != "pre\npost" {
		t.Fatalf("expected stop hook order pre->post, got:\n%s", string(content))
	}
}

func TestUpQuickstartWithShims(t *testing.T) {
	SkipIfNoDocker(t)

	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "up-m2")
	CopyBlueprints(t, filepath.Join(projectDir, "blueprints"))

	shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
	result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "env", "up", "--quickstart")
	result.AssertSuccess(t)

	logs := shim.ReadLog(t)
	assertContains(t, logs, "docker|compose --project-directory")
	assertContains(t, logs, " up -d")
}
