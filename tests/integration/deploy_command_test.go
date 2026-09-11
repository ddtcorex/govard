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

// TestDeployHooksExecute proves the pre_deploy/post_deploy lifecycle hooks still
// run, in order, around a real deploy. The command used to run the hooks and
// nothing else; it now performs a deployment, so the test drives one against a
// local remote instead of relying on a remote being optional.
//
// The fixture is deliberately a framework with no deploy recipe: the pipeline
// under test here is the neutral one (release, code, publish, verify), and a
// Magento fixture would now also exercise the framework recipe's build steps,
// which need a real application to run against.
func TestDeployHooksExecute(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-hooks")

	origin, revision := seedDeployOrigin(t)
	deployRoot := t.TempDir()

	localOverride := fmt.Sprintf(`hooks:
  pre_deploy:
    - name: pre deploy marker
      run: "echo pre >> .govard-deploy-hooks.log"
  post_deploy:
    - name: post deploy marker
      run: "echo post >> .govard-deploy-hooks.log"
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: %s/public_html
    deploy_path: %s/.deployer
    branch: main
    repository: %s
    local: true
`, deployRoot, deployRoot, origin)
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(localOverride), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	result := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revision, "--yes")
	result.AssertSuccess(t)

	content, err := os.ReadFile(filepath.Join(projectDir, ".govard-deploy-hooks.log"))
	if err != nil {
		t.Fatalf("failed to read deploy hook log: %v", err)
	}
	out := strings.TrimSpace(string(content))
	if out != "pre\npost" {
		t.Fatalf("expected deploy hook order pre->post, got:\n%s", out)
	}

	// The deploy actually published: the current symlink points at the release
	// created for this revision.
	current, err := os.Readlink(filepath.Join(deployRoot, "public_html"))
	if err != nil {
		t.Fatalf("current symlink missing: %v", err)
	}
	if !strings.HasPrefix(current, filepath.Join(deployRoot, ".deployer", "releases")) {
		t.Fatalf("current -> %q, want a path under the deploy path", current)
	}
}

// TestDeployWithoutARemoteIsAUsageError pins the contract that replaced the old
// no-argument behaviour: deploying without naming an environment is a usage
// error, never a silent guess.
func TestDeployWithoutARemoteIsAUsageError(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "deploy-no-remote")

	result := env.RunGovard(t, projectDir, "deploy")
	if result.ExitCode != 2 {
		t.Fatalf("govard deploy without a remote exited %d, want 2\nstdout: %s\nstderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr+result.Stdout, "a remote is required") {
		t.Fatalf("the usage error must say a remote is required, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

// seedDeployOrigin creates a repository the deploy can fetch from and returns
// its path plus the commit to deploy.
func seedDeployOrigin(t *testing.T) (string, string) {
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
	if err := os.WriteFile(filepath.Join(work, "app.txt"), []byte("deployed\n"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	runGit(work, "add", ".")
	runGit(work, "commit", "-q", "-m", "initial")
	revision := runGit(work, "rev-parse", "HEAD")
	runGit(root, "clone", "-q", "--bare", work, origin)
	return origin, revision
}
