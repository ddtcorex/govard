//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeployRecoveryWorkflow drives the commands an operator reaches for after a
// bad deploy, against a real target: list the releases, see what is live, roll
// back, and roll forward again.
//
// It is the end-to-end proof that rollback moves the target without rebuilding:
// the second revision is never re-uploaded by the rollback, and the code that
// goes live after rolling back is the code from the first release directory.
func TestDeployRecoveryWorkflow(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-recovery")

	origin, revisions := seedDeployRevisions(t, 2)
	deployRoot := t.TempDir()
	writeLocalRemote(t, projectDir, deployRoot, origin)

	first := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revisions[0], "--yes")
	first.AssertSuccess(t)

	second := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revisions[1], "--yes")
	second.AssertSuccess(t)

	currentPath := filepath.Join(deployRoot, "public_html")
	if target := readlink(t, currentPath); !strings.HasSuffix(target, filepath.Join("releases", "2")) {
		t.Fatalf("after two deploys current -> %q, want the second release", target)
	}

	listed := env.RunGovard(t, projectDir, "deploy", "releases", "local")
	listed.AssertSuccess(t)
	for _, want := range []string{"RELEASE", "1", "2", "(live)"} {
		listed.AssertOutputContains(t, want)
	}

	status := env.RunGovard(t, projectDir, "deploy", "status", "local")
	status.AssertSuccess(t)
	status.AssertOutputContains(t, revisions[1][:8])

	rollback := env.RunGovard(t, projectDir, "deploy", "rollback", "local", "--yes")
	rollback.AssertSuccess(t)
	if target := readlink(t, currentPath); !strings.HasSuffix(target, filepath.Join("releases", "1")) {
		t.Fatalf("after rollback current -> %q, want the first release", target)
	}

	rolled := env.RunGovard(t, projectDir, "deploy", "status", "local")
	rolled.AssertSuccess(t)
	rolled.AssertOutputContains(t, revisions[0][:8])

	// Rolling forward names the release explicitly, which is how an operator
	// undoes a rollback once the problem is fixed.
	forward := env.RunGovard(t, projectDir, "deploy", "rollback", "local", "--to", "2", "--yes")
	forward.AssertSuccess(t)
	if target := readlink(t, currentPath); !strings.HasSuffix(target, filepath.Join("releases", "2")) {
		t.Fatalf("after rollback --to 2 current -> %q, want the second release", target)
	}
}

// TestDeployStatusCoversEveryRemoteWhenNoneIsNamed pins the no-argument form: it
// reports a row per configured remote instead of refusing to guess.
func TestDeployStatusCoversEveryRemoteWhenNoneIsNamed(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-status-all")

	origin, revisions := seedDeployRevisions(t, 1)
	deployRoot := t.TempDir()
	writeLocalRemote(t, projectDir, deployRoot, origin)

	env.RunGovard(t, projectDir, "deploy", "local", "--revision", revisions[0], "--yes").AssertSuccess(t)

	result := env.RunGovard(t, projectDir, "deploy", "status")
	result.AssertSuccess(t)
	result.AssertOutputContains(t, "REMOTE")
	result.AssertOutputContains(t, "local")
	result.AssertOutputContains(t, revisions[0][:8])
}

// TestDeployFromAnUnknownTaskIsAUsageError pins the refusal: --from that names
// nothing must not quietly run the whole pipeline.
func TestDeployFromAnUnknownTaskIsAUsageError(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-bad-from")

	origin, revisions := seedDeployRevisions(t, 1)
	writeLocalRemote(t, projectDir, t.TempDir(), origin)

	result := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revisions[0], "--from", "nope", "--yes")
	if result.ExitCode != 2 {
		t.Fatalf("--from nope exited %d, want 2\nstdout: %s\nstderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr+result.Stdout, "does not name a task") {
		t.Fatalf("the usage error must name the problem, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

func writeLocalRemote(t *testing.T, projectDir, deployRoot, origin string) {
	t.Helper()
	override := fmt.Sprintf(`remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: %s/public_html
    deploy_path: %s/.deployer
    branch: main
    repository: %s
    local: true
`, deployRoot, deployRoot, origin)
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(override), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}
}

func readlink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("read current symlink %s: %v", path, err)
	}
	return target
}

// seedDeployRevisions creates a bare origin with `count` commits on main and
// returns the origin path plus each commit, oldest first.
func seedDeployRevisions(t *testing.T, count int) (string, []string) {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	origin := filepath.Join(root, "origin.git")

	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Integration", "GIT_AUTHOR_EMAIL=integration@example.com",
			"GIT_COMMITTER_NAME=Integration", "GIT_COMMITTER_EMAIL=integration@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	runGit(work, "init", "-q", "-b", "main")

	revisions := make([]string, 0, count)
	for index := 0; index < count; index++ {
		content := fmt.Sprintf("deployed revision %d\n", index)
		if err := os.WriteFile(filepath.Join(work, "app.txt"), []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture file: %v", err)
		}
		runGit(work, "add", ".")
		runGit(work, "commit", "-q", "-m", fmt.Sprintf("revision %d", index))
		revisions = append(revisions, runGit(work, "rev-parse", "HEAD"))
	}

	runGit(root, "clone", "-q", "--bare", work, origin)
	return origin, revisions
}

// TestDeployPlanShowsTheFrameworkRecipe proves the recipe really reaches the
// command layer: the plan for a framework with a recipe lists its commands, and
// the plan for a framework without one says so instead of inventing steps.
func TestDeployPlanShowsTheFrameworkRecipe(t *testing.T) {
	env := NewTestEnvironment(t)

	t.Run("a framework with a recipe", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "deploy-plan-magento2")
		writeLocalRemote(t, projectDir, t.TempDir(), "git@example.invalid:project.git")

		result := env.RunGovard(t, projectDir, "deploy", "plan", "local")
		result.AssertSuccess(t)

		for _, want := range []string{
			"bin/magento setup:upgrade",
			"bin/magento setup:static-content:deploy",
			"{{settings.static_content_locales_args}}",
		} {
			result.AssertOutputContains(t, want)
		}
	})

	t.Run("a framework without a recipe", func(t *testing.T) {
		projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-plan-default")
		writeLocalRemote(t, projectDir, t.TempDir(), "git@example.invalid:project.git")

		result := env.RunGovard(t, projectDir, "deploy", "plan", "local")
		result.AssertSuccess(t)

		result.AssertOutputContains(t, "skipped (no implementation for this framework)")
		result.AssertOutputContains(t, "implemented in the engine")
	})
}
