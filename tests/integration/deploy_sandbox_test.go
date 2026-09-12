//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
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

// seedOriginFromProject publishes the project directory itself as the origin, so
// the release the target materialises carries the fixture's files — the stub
// `bin/magento` included.
func seedOriginFromProject(t *testing.T, projectDir string) (string, string) {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	origin := filepath.Join(root, "origin.git")

	copyDir(t, projectDir, work)
	runGitIn(t, work, "init", "-q", "-b", "main")
	runGitIn(t, work, "add", "-A")
	runGitIn(t, work, "commit", "-q", "-m", "initial")
	revision := runGitIn(t, work, "rev-parse", "HEAD")
	runGitIn(t, root, "clone", "-q", "--bare", work, origin)
	return origin, revision
}

// TestDeploySandboxRunsTheMagentoRecipeOverRealSSH deploys a Magento 2 project to
// a real container over real SSH and real rsync, with a `bin/magento` stub that
// *asserts* the arguments the recipe builds: which static-content passes ran, in
// which order, with which `--area`, and that worker control actually moved cron
// and the consumers.
//
// The stub is the point. The static-content split, the worker-control guard and
// the Hyva frontend build all live inside one shell command each, and the recipe's
// guards and fragments were inert until they were executed: `[ "{{settings.mage_mode}}"
// != "developer" ]` expands to `[ "'developer'" != "developer" ]` (always true),
// and `{{settings.frontend_command}}` expands to one quoted word, so the default
// `npm ci && npm run build` was a command the shell could not find. A test that
// reads the template cannot see either; one that runs it against a target can.
//
// A real Magento cannot be installed here — every Magento package is behind
// repo.magento.com credentials, verified with Composer — so the application
// contract that remains unexecuted is Magento's own acceptance of `--area`, which
// is taken from Magento's option definition (Magento_Deploy\Console\
// DeployStaticOptions) and the reference deploy tool's recipe.
func TestDeploySandboxRunsTheMagentoRecipeOverRealSSH(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		static   string
		frontend string
		settings string
		// docroot is the layout `sandbox up` should create: empty means the
		// default dangling symlink, "real" means a docroot that is a git checkout
		// already holding the application (an in-place target).
		docroot string
		// sync names what the stub should verify the activation published into the
		// served docroot. Empty means the case does not deploy in place.
		sync string
	}{
		{
			name:   "single",
			static: "single",
		},
		{
			name:     "split",
			static:   "split",
			settings: "    split_static_deployment: true\n    worker_control: true\n",
		},
		{
			// A Hyva theme: frontend_dir points at the theme's Tailwind
			// directory and the default `frontend_command` builds it there, with
			// the Node toolchain on the build machine rather than in govard.
			name:     "hyva",
			static:   "single",
			frontend: "hyva",
			settings: "    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind\n",
		},
		{
			// Two Node-built themes — two storefronts, or a Hyva theme plus a
			// custom one. Every configured directory must be built, each from its
			// own working directory, which is what the list form and the loop are
			// for.
			name:     "hyva-multi",
			static:   "single",
			frontend: "hyva-multi",
			settings: "    frontend_dir:\n" +
				"      - app/design/frontend/Acme/hyva/web/tailwind\n" +
				"      - app/design/frontend/Acme/other/web/tailwind\n",
		},
		{
			// The in-place layout: the docroot is a previous deployment that the
			// activation rewrites while it is being served, which is the case the
			// maintenance window exists for. It is also the case where the window
			// used to be opened on the release being built — where the flag
			// protected nothing — so the stub asserts that both maintenance steps
			// ran in the served directory.
			name:    "in-place",
			static:  "single",
			docroot: "real",
			sync:    "generated",
		},
	} {
		expectation := testCase.name
		t.Run(expectation, func(t *testing.T) {
			env := NewTestEnvironment(t)
			projectDir := env.CreateProjectFromFixture(t, "deploy/magento-stub", "deploy-magento-stub-"+expectation)

			// The stub reads these from the release, so they have to be committed.
			if err := os.WriteFile(filepath.Join(projectDir, "deploy-expectation.txt"), []byte(testCase.static+"\n"), 0o644); err != nil {
				t.Fatalf("write the expectation: %v", err)
			}
			if testCase.frontend != "" {
				if err := os.WriteFile(filepath.Join(projectDir, "frontend-expectation.txt"), []byte(testCase.frontend+"\n"), 0o644); err != nil {
					t.Fatalf("write the frontend expectation: %v", err)
				}
			}
			// Where the site is served from, so the stub can assert that the
			// maintenance window was opened there. It is the sandbox's own
			// configured current path, and it has to be committed before `up`
			// because that is what seeds the mirror the target deploys from.
			if err := os.WriteFile(filepath.Join(projectDir, "served-path.txt"), []byte(deploy.SandboxDefaultPaths().Current+"\n"), 0o644); err != nil {
				t.Fatalf("write the served path: %v", err)
			}
			if testCase.sync != "" {
				// The stub checks, at verify time, that the served docroot holds what
				// the build produced: an in-place activation that copies nothing
				// leaves the previous deployment's directories in place.
				if err := os.WriteFile(filepath.Join(projectDir, "sync-expectation.txt"), []byte(testCase.sync+"\n"), 0o644); err != nil {
					t.Fatalf("write the sync expectation: %v", err)
				}
			}
			origin, revision := seedOriginFromProject(t, projectDir)
			seedSandboxCheckout(t, projectDir, origin)

			// The `php` profile: the recipe runs `{{php_bin}} bin/magento`, and
			// the stub is a PHP script so the target can execute it the way a
			// real Magento would be executed.
			upArgs := []string{"deploy", "sandbox", "up", "--profile", "php"}
			if testCase.docroot != "" {
				upArgs = append(upArgs, "--docroot", testCase.docroot)
			}
			up := env.RunGovard(t, projectDir, upArgs...)
			if up.ExitCode != 0 {
				t.Fatalf("sandbox up failed (%d)\nstdout: %s\nstderr: %s", up.ExitCode, up.Stdout, up.Stderr)
			}
			t.Cleanup(func() { env.RunGovard(t, projectDir, "deploy", "sandbox", "down", "--purge") })

			// The `php` profile ships the web tier, so `up` advertises the HTTP
			// half of `deploy:verify`. The verdict below is an exit code, so
			// without this the run would prove nothing about the check happening at
			// all — and the fixture serves `pub/index.php` precisely so that it can.
			// The file is read as text on purpose (see sandboxRemote), so the
			// expected URL is the one `up` prints rather than a decoded struct.
			localLayer, readErr := os.ReadFile(filepath.Join(projectDir, ".govard.local.yml"))
			if readErr != nil {
				t.Fatalf("read the local layer: %v", readErr)
			}
			if !strings.Contains(string(localLayer), "verify:") || !strings.Contains(string(localLayer), "url: http://127.0.0.1:") {
				t.Fatalf("the sandbox advertises no verify URL, so the HTTP check never runs:\n%s", localLayer)
			}

			if testCase.settings != "" {
				// Written after `up`, because `up` rewrites .govard.local.yml to
				// add the sandbox remote.
				content, err := os.ReadFile(filepath.Join(projectDir, ".govard.local.yml"))
				if err != nil {
					t.Fatalf("read the local layer: %v", err)
				}
				withSettings := string(content) + "\ndeploy:\n  settings:\n" + testCase.settings
				if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(withSettings), 0o644); err != nil {
					t.Fatalf("write the local layer: %v", err)
				}
			}

			deploy := env.RunGovard(t, projectDir, "deploy", "--remote", "sandbox", "--revision", revision, "--yes")
			if deploy.ExitCode != 0 {
				t.Fatalf("the sandbox deploy failed (%d) — the stub's assertion is the verdict\nstdout: %s\nstderr: %s",
					deploy.ExitCode, deploy.Stdout, deploy.Stderr)
			}
			// The stub fails the deploy if a pass is missing or out of order, so a
			// green deploy is the assertion. Assert the deploy really published.
			status := env.RunGovard(t, projectDir, "deploy", "status", "sandbox")
			status.AssertSuccess(t)
			status.AssertOutputContains(t, revision[:8])
		})
	}
}
