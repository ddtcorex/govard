//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeploySandboxEndToEnd is the deploy feature's regression suite: a real
// container, reached over real SSH and real rsync, running the same pipeline a
// production deploy runs. Nothing in the pipeline knows the target is a
// sandbox, so a green run here is evidence about the pipeline itself rather
// than about a test double.
//
// The container is the `basic` profile: the pipeline, the layout, publish,
// verify, rollback and the lock are all exercised without needing an
// application toolchain, which keeps the test fast enough to run on every push.
func TestDeploySandboxEndToEnd(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-sandbox")

	origin, revisions := seedDeployRevisions(t, 2)
	seedSandboxCheckout(t, projectDir, origin)

	up := env.RunGovard(t, projectDir, "deploy", "sandbox", "up", "--profile", "basic")
	if up.ExitCode != 0 {
		t.Fatalf("sandbox up failed (%d)\nstdout: %s\nstderr: %s", up.ExitCode, up.Stdout, up.Stderr)
	}
	t.Cleanup(func() {
		env.RunGovard(t, projectDir, "deploy", "sandbox", "down", "--purge")
	})

	// The remote govard wrote is what makes the sandbox a normal deploy target.
	remote, configured, err := sandboxRemote(t, projectDir)
	if err != nil {
		t.Fatalf("read the sandbox remote: %v", err)
	}
	if !configured {
		t.Fatal("up did not write a sandbox remote")
	}
	if remote.Port == 0 {
		t.Fatal("the sandbox remote has no published port")
	}

	first := env.RunGovard(t, projectDir, "deploy", "--remote", "sandbox", "--revision", revisions[0], "--yes")
	if first.ExitCode != 0 {
		t.Fatalf("deploy to the sandbox failed (%d)\nstdout: %s\nstderr: %s", first.ExitCode, first.Stdout, first.Stderr)
	}

	// Deploying an unpushed local commit is the whole reason the sandbox mounts
	// a mirror instead of pointing at the real repository.
	second := env.RunGovard(t, projectDir, "deploy", "--remote", "sandbox", "--revision", revisions[1], "--yes")
	if second.ExitCode != 0 {
		t.Fatalf("second deploy failed (%d)\nstdout: %s\nstderr: %s", second.ExitCode, second.Stdout, second.Stderr)
	}

	listed := env.RunGovard(t, projectDir, "deploy", "releases", "sandbox")
	listed.AssertSuccess(t)
	listed.AssertOutputContains(t, "(live)")

	status := env.RunGovard(t, projectDir, "deploy", "status", "sandbox")
	status.AssertSuccess(t)
	status.AssertOutputContains(t, revisions[1][:8])

	// Rollback re-points the symlink against the release that is already on the
	// target: no rebuild, no re-upload.
	rollback := env.RunGovard(t, projectDir, "deploy", "rollback", "sandbox", "--yes")
	if rollback.ExitCode != 0 {
		t.Fatalf("rollback failed (%d)\nstdout: %s\nstderr: %s", rollback.ExitCode, rollback.Stdout, rollback.Stderr)
	}
	rolled := env.RunGovard(t, projectDir, "deploy", "status", "sandbox")
	rolled.AssertSuccess(t)
	rolled.AssertOutputContains(t, revisions[0][:8])

	down := env.RunGovard(t, projectDir, "deploy", "sandbox", "down")
	down.AssertSuccess(t)
	if _, configured, err := sandboxRemote(t, projectDir); err != nil || configured {
		t.Fatalf("the sandbox remote survived down (configured=%v err=%v)", configured, err)
	}
}

// TestDeploySandboxInPlaceAndDeployerLayout covers the two levers the design
// needs from a sandbox: a real docroot selects in-place publishing, and a
// seeded target proves govard refuses to race the other deploy tool.
func TestDeploySandboxInPlaceAndDeployerLayout(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-sandbox-inplace")

	origin, revisions := seedDeployRevisions(t, 1)
	seedSandboxCheckout(t, projectDir, origin)

	up := env.RunGovard(t, projectDir, "deploy", "sandbox", "up", "--profile", "basic", "--docroot", "real")
	if up.ExitCode != 0 {
		t.Fatalf("sandbox up --docroot=real failed (%d)\nstdout: %s\nstderr: %s", up.ExitCode, up.Stdout, up.Stderr)
	}
	t.Cleanup(func() {
		env.RunGovard(t, projectDir, "deploy", "sandbox", "down", "--purge")
	})

	deployed := env.RunGovard(t, projectDir, "deploy", "--remote", "sandbox", "--revision", revisions[0], "--yes")
	if deployed.ExitCode != 0 {
		t.Fatalf("in-place deploy failed (%d)\nstdout: %s\nstderr: %s", deployed.ExitCode, deployed.Stdout, deployed.Stderr)
	}
	deployed.AssertOutputContains(t, "in_place")

	// The seeded target is owned by the other deploy tool, and govard must
	// refuse rather than race it on the same release directories.
	reset := env.RunGovard(t, projectDir, "deploy", "sandbox", "reset", "--layout", "deployer", "--docroot", "absent")
	reset.AssertSuccess(t)

	blocked := env.RunGovard(t, projectDir, "deploy", "--remote", "sandbox", "--revision", revisions[0], "--yes")
	if blocked.ExitCode == 0 {
		t.Fatalf("govard deployed onto a target the other tool holds:\n%s", blocked.Stdout)
	}
	if !strings.Contains(blocked.Stderr+blocked.Stdout, "lock") {
		t.Fatalf("the refusal must name the other tool's lock, got:\n%s%s", blocked.Stdout, blocked.Stderr)
	}
}

// seedSandboxCheckout turns the fixture into a checkout that holds the seeded
// revisions, the way a developer's working directory does.
func seedSandboxCheckout(t *testing.T, projectDir, origin string) {
	t.Helper()
	runGitIn(t, projectDir, "init", "-q", "-b", "main")
	runGitIn(t, projectDir, "fetch", "-q", origin, "main")
	runGitIn(t, projectDir, "update-ref", "refs/heads/main", "FETCH_HEAD")
}

// sandboxRemoteInfo is what the test reads back out of the local layer.
type sandboxRemoteInfo struct {
	Port       int
	Repository string
}

// sandboxRemote reads the remote govard wrote into .govard.local.yml. The file
// is small and flat, and the test deliberately reads it as text: a test that
// decoded it with govard's own config loader would pass even if govard wrote a
// file no other tool could read.
func sandboxRemote(t *testing.T, projectDir string) (sandboxRemoteInfo, bool, error) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, ".govard.local.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return sandboxRemoteInfo{}, false, nil
		}
		return sandboxRemoteInfo{}, false, err
	}
	text := string(content)
	if !strings.Contains(text, "\n  sandbox:\n") && !strings.HasPrefix(text, "sandbox:\n") {
		return sandboxRemoteInfo{}, false, nil
	}

	result := sandboxRemoteInfo{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "port:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "port:"))
			var port int
			if err := json.Unmarshal([]byte(value), &port); err == nil {
				result.Port = port
			}
		}
		if strings.HasPrefix(trimmed, "repository:") {
			result.Repository = strings.TrimSpace(strings.TrimPrefix(trimmed, "repository:"))
		}
	}
	return result, true, nil
}
