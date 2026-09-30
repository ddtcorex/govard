//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"govard/internal/deploy"
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
    local: true
    deploy:
      deploy_path: %s/.deployer
      branch: main
      repository: %s
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

// A value in `.govard.yml` that the configuration layer cannot parse is a
// configuration error, not a usage error. The distinction is what lets a
// pipeline tell "the operator typed the command wrong" from "the project is
// misconfigured", and the exit-code table has reserved 4 for it since the
// capability contract landed.
func TestDeployConfigurationErrorsExitFour(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-config-error")
	origin, revision := seedDeployOrigin(t)
	deployRoot := t.TempDir()

	override := fmt.Sprintf(`deploy:
  verify:
    timeout: not-a-duration
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: %s/public_html
    local: true
    deploy:
      deploy_path: %s/.deployer
      branch: main
      repository: %s
`, deployRoot, deployRoot, origin)
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(override), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	result := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revision, "--yes", "--error-json")
	result.AssertExitCode(t, 4)
	if !strings.Contains(result.Stdout, `"code": "CONFIG"`) {
		t.Fatalf("a configuration error must carry the CONFIG envelope, got:\n%s%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout+result.Stderr, "verify.timeout") {
		t.Fatalf("the error must name the setting, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

// The other half of the split: a bad flag value stays a usage error (exit 2),
// so the two classes never collapse into one.
func TestDeployBadFlagValuesStayUsageErrors(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-usage-error")
	origin, revision := seedDeployOrigin(t)
	writeLocalRemote(t, projectDir, t.TempDir(), origin)

	result := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revision, "--command-timeout", "-5s", "--error-json")
	result.AssertExitCode(t, 2)
	if !strings.Contains(result.Stdout, `"code": "USAGE"`) {
		t.Fatalf("a bad flag value must carry the USAGE envelope, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

// Spec 5.1: `deploy_path` has no default, but the layout a server already has is
// discoverable — that is the migration aid for an environment another tool set
// up. The discovered path has to be the one the rest of the command then uses,
// not just something printed.
func TestDeployDiscoversAnExistingLayoutWhenNoDeployPathIsConfigured(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-discovery")
	origin, _ := seedDeployOrigin(t)

	home := t.TempDir()
	deployRoot := filepath.Join(home, ".deployer") // reached as ~/.deployer
	releaseDir := filepath.Join(deployRoot, "releases", "3")
	for _, dir := range []string{filepath.Join(releaseDir, ".dep"), filepath.Join(deployRoot, "shared")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	record := `{"schema_version":1,"tool":"govard","release":"3","revision":"abc1234","branch":"main","status":"ok"}`
	if err := os.WriteFile(filepath.Join(releaseDir, ".dep", "release.json"), []byte(record), 0o644); err != nil {
		t.Fatalf("write release record: %v", err)
	}

	// The remote deliberately has no deploy_path: the layout is the only source.
	override := fmt.Sprintf(`remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: %s/public_html
    local: true
    deploy:
      branch: main
      repository: %s
`, deployRoot, origin)
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(override), 0o644); err != nil {
		t.Fatalf("write .govard.local.yml: %v", err)
	}

	result := env.RunGovardWithEnv(t, projectDir, []string{"HOME=" + home}, "deploy", "releases", "local")
	result.AssertSuccess(t)
	// The note reports the candidate as written, because that is the value the
	// operator can paste into `deploy_path`.
	if !strings.Contains(result.Stdout, "~/.deployer") {
		t.Fatalf("the discovered layout must be reported, got:\n%s%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "3") {
		t.Fatalf("the release inside the discovered layout must be listed, got:\n%s%s", result.Stdout, result.Stderr)
	}

	// With no layout anywhere, the refusal must name deploy_path: the operator
	// has a file to edit, so it is a configuration error.
	empty := t.TempDir()
	refused := env.RunGovardWithEnv(t, projectDir, []string{"HOME=" + empty}, "deploy", "releases", "local", "--error-json")
	refused.AssertExitCode(t, 4)
	if !strings.Contains(refused.Stdout, "deploy_path") {
		t.Fatalf("the refusal must name the setting, got:\n%s%s", refused.Stdout, refused.Stderr)
	}
}

// Spec 13: `--json` "emits a machine-readable equivalent for CI:
// schema_version, remote, branch, revision, release, build.mode,
// publish.strategy, tasks[…], verify, result. Failures name the task id, host,
// command and the resume command; exit 1."
//
// The fields were flat (`build_mode`, `publish` as a string), `result` was
// hardcoded "ok", and JSON was emitted only on the success path — while the
// human timeline was written to the same stream, so stdout was not parseable at
// all.
func TestDeployJSONIsAMachineReadableContract(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-json")
	origin, revision := seedDeployOrigin(t)
	writeLocalRemote(t, projectDir, t.TempDir(), origin)

	parse := func(t *testing.T, result *CommandResult) map[string]any {
		t.Helper()
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &payload); err != nil {
			t.Fatalf("stdout is not one JSON document (%v)\nstdout: %s\nstderr: %s", err, result.Stdout, result.Stderr)
		}
		return payload
	}

	result := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revision, "--yes", "--json")
	result.AssertSuccess(t)
	payload := parse(t, result)

	if payload["result"] != "ok" {
		t.Errorf("result = %v, want ok", payload["result"])
	}
	if payload["schema_version"] != float64(1) {
		t.Errorf("schema_version = %v, want 1", payload["schema_version"])
	}
	build, _ := payload["build"].(map[string]any)
	if build == nil || build["mode"] != deploy.BuildServer {
		t.Errorf("build = %v, want a nested {mode:%s}", payload["build"], deploy.BuildServer)
	}
	publish, _ := payload["publish"].(map[string]any)
	if publish == nil || publish["strategy"] == "" {
		t.Errorf("publish = %v, want a nested {strategy:…}", payload["publish"])
	}
	tasks, ok := payload["tasks"].([]any)
	if !ok || len(tasks) == 0 {
		t.Fatalf("tasks = %v, want a list of steps", payload["tasks"])
	}
	if payload["remote"] != "local" || payload["revision"] != revision {
		t.Errorf("remote/revision = %v/%v, want local/%s", payload["remote"], payload["revision"], revision)
	}

	// The failure path must produce the same document on the same stream: CI
	// reads one shape, not two.
	failed := env.RunGovard(t, projectDir, "deploy", "local", "--revision", strings.Repeat("0", 40), "--yes", "--json")
	if failed.ExitCode != 1 {
		t.Fatalf("a failing deploy exited %d, want 1\nstdout: %s\nstderr: %s", failed.ExitCode, failed.Stdout, failed.Stderr)
	}
	failure := parse(t, failed)
	if failure["result"] != "failed" {
		t.Errorf("result = %v, want failed", failure["result"])
	}
	if message, _ := failure["error"].(string); message == "" {
		t.Errorf("a failed deploy must say what failed: %v", failure)
	}
}

// Spec 5.2 makes an unknown `deploy.settings` key a configuration error (exit 4),
// which is what `/reference/cli-commands` already documents. Without it a typo was
// a silent no-op.
func TestDeployRefusesAnUnknownSetting(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-unknown-setting")
	origin, revision := seedDeployOrigin(t)
	deployRoot := t.TempDir()

	override := fmt.Sprintf(`deploy:
  settings:
    shared_file: app/etc/env.php
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: %s/public_html
    local: true
    deploy:
      deploy_path: %s/.deployer
      branch: main
      repository: %s
`, deployRoot, deployRoot, origin)
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(override), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	result := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revision, "--error-json")
	result.AssertExitCode(t, 4)
	if !strings.Contains(result.Stdout, `"code": "CONFIG"`) {
		t.Fatalf("an unknown setting must carry the CONFIG envelope, got:\n%s%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout+result.Stderr, "shared_file") {
		t.Fatalf("the refusal must name the key, got:\n%s%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout+result.Stderr, "shared_files") {
		t.Fatalf("the refusal must suggest the near miss, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

// One condition — the named remote is not in the project configuration — used to
// report itself as three different exit codes: `deploy plan` called it a usage
// mistake (2) while `deploy status` and `sync` let it fall out as a plain
// execution failure (1). A script branching on the exit code was therefore told
// to fix its command line for a name that belongs in `.govard.yml`.
//
// Exit 4 is the class for "the operator edits the file", and every deploy
// command has to answer with it. The capability gate is forced rather than left
// to the host: the six sub-cases fail before any connection is attempted, so
// nothing here needs a real ssh or rsync, but the gate itself runs first and
// would otherwise decide the exit code before the command is even reached.
func TestDeployUnknownRemoteIsAConfigurationError(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-unknown-remote")
	forced := []string{"GOVARD_TEST_SATISFIED_CAPABILITIES=ssh,rsync"}
	outputDir := t.TempDir()

	cases := []struct {
		name string
		args []string
	}{
		{name: "plan", args: []string{"deploy", "plan", "nope"}},
		{name: "check", args: []string{"deploy", "check", "nope"}},
		{name: "releases", args: []string{"deploy", "releases", "nope"}},
		{name: "build", args: []string{"deploy", "build", "nope", "--output", outputDir}},
		{name: "deploy", args: []string{"deploy", "nope", "--yes"}},
		{name: "status", args: []string{"deploy", "status", "nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "--error-json")
			result := env.RunGovardWithEnv(t, projectDir, forced, args...)
			result.AssertExitCode(t, 4)
			if !strings.Contains(result.Stdout, `"code": "CONFIG"`) {
				t.Fatalf("an unknown remote must carry the CONFIG envelope, got:\n%s%s", result.Stdout, result.Stderr)
			}
		})
	}

	// The same class, from the other direction: a project that cannot be read at
	// all. `deploy plan` reached it as a `ConfigError` and then re-wrapped it as a
	// usage error on the way out, so the downcast half of the defect was the
	// missing `.govard.yml` rather than the missing remote.
	missing := env.RunGovardWithEnv(t, t.TempDir(), forced, "deploy", "plan", "local", "--error-json")
	missing.AssertExitCode(t, 4)
	if !strings.Contains(missing.Stdout, `"code": "CONFIG"`) {
		t.Fatalf("a missing .govard.yml must carry the CONFIG envelope, got:\n%s%s", missing.Stdout, missing.Stderr)
	}
}

// The other half of the boundary, and the reason the fix cannot be "make every
// failure a configuration error". A remote that *is* configured but whose target
// cannot be read is an execution failure: the operator edits nothing. So
// `deploy status` must keep answering 1 for a reachable-looking name it could
// not read, and 4 only for a name the project never had.
func TestDeployStatusKeepsAConfiguredButUnreachableRemoteAtExitOne(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-unreachable-remote")

	// A local remote with no deploy_path and nothing to discover: the name
	// resolves, and reading the target fails. That is a row, not the command.
	deployRoot := t.TempDir()
	home := t.TempDir()
	override := fmt.Sprintf(`remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: %s/public_html
    local: true
    deploy:
      branch: main
`, deployRoot)
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(override), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}

	result := env.RunGovardWithEnv(t, projectDir, []string{"GOVARD_TEST_SATISFIED_CAPABILITIES=ssh", "HOME=" + home}, "deploy", "status", "local")
	result.AssertExitCode(t, 1)
	if !strings.Contains(result.Stdout+result.Stderr, "no configured remote could be reached") {
		t.Fatalf("an unreadable target must still be aggregated as a row, got:\n%s%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "deploy_path") {
		t.Fatalf("the row must carry the reason the target could not be read, got:\n%s", result.Stdout)
	}
}

// writeDeployLocalOverride writes a .govard.local.yml that configures one local
// remote named "local". When deploySettings is non-empty it is emitted verbatim
// as the top-level `deploy:` block.
func writeDeployLocalOverride(t *testing.T, projectDir, deploySettings string) {
	t.Helper()
	deployRoot := t.TempDir()
	override := deploySettings + fmt.Sprintf(`remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: %s/public_html
    local: true
    deploy:
      branch: main
`, deployRoot)
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.local.yml"), []byte(override), 0o644); err != nil {
		t.Fatalf("failed to write .govard.local.yml: %v", err)
	}
}

// `deploy check` returned resolveDeployRecipeOptions errors raw, so the typo that
// makes `deploy`, `plan`, `build` and `rollback` exit 4 made `check` exit 1.
func TestDeployCheckTypoedSettingExits4(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-check-typo")
	writeDeployLocalOverride(t, projectDir, "deploy:\n  settings:\n    shared_file: app/etc/env.php\n")

	result := env.RunGovardWithEnv(t, projectDir, []string{"GOVARD_TEST_SATISFIED_CAPABILITIES=ssh,rsync"}, "deploy", "check", "local", "--error-json")
	result.AssertExitCode(t, 4)
	if !strings.Contains(result.Stdout, `"code": "CONFIG"`) {
		t.Fatalf("a typoed setting must carry the CONFIG envelope from check, got:\n%s%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout+result.Stderr, "shared_file") {
		t.Fatalf("the refusal must name the key, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

// A project with no remotes, and a project that cannot be loaded at all, are
// both "the operator edits the file" (4), not execution failures (1).
func TestDeployStatusNoRemotesExits4(t *testing.T) {
	env := NewTestEnvironment(t)
	forced := []string{"GOVARD_TEST_SATISFIED_CAPABILITIES=ssh"}
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-status-no-remotes")

	noRemotes := env.RunGovardWithEnv(t, projectDir, forced, "deploy", "status", "--error-json")
	noRemotes.AssertExitCode(t, 4)
	if !strings.Contains(noRemotes.Stdout, `"code": "CONFIG"`) || !strings.Contains(noRemotes.Stdout+noRemotes.Stderr, "configures no remotes") {
		t.Fatalf("no remotes must be a CONFIG error naming the cause, got:\n%s%s", noRemotes.Stdout, noRemotes.Stderr)
	}

	missing := env.RunGovardWithEnv(t, t.TempDir(), forced, "deploy", "status", "--error-json")
	missing.AssertExitCode(t, 4)
	if !strings.Contains(missing.Stdout, `"code": "CONFIG"`) {
		t.Fatalf("an unloadable project must be a CONFIG error from status, got:\n%s%s", missing.Stdout, missing.Stderr)
	}
}

// A positional remote that contradicts --remote is a command-line mistake (2),
// as it is for the sibling commands.
func TestDeployStatusRemoteMismatchExits2(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-status-mismatch")
	writeDeployLocalOverride(t, projectDir, "")

	result := env.RunGovardWithEnv(t, projectDir, []string{"GOVARD_TEST_SATISFIED_CAPABILITIES=ssh"}, "deploy", "status", "local", "--remote", "other", "--error-json")
	result.AssertExitCode(t, 2)
	if !strings.Contains(result.Stdout, `"code": "USAGE"`) {
		t.Fatalf("a remote mismatch must carry the USAGE envelope, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

// With every remote unreachable `deploy status --json` used to print the rows
// and exit 0 while the table mode exited 1, so a health job reading JSON went
// green with every target down. Both modes now exit 1, and the JSON document is
// still rendered (once, to stdout) so the consumer sees the per-remote rows.
func TestDeployStatusJSONAllUnreachableExits1(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-status-json-down")
	writeDeployLocalOverride(t, projectDir, "")
	envVars := []string{"GOVARD_TEST_SATISFIED_CAPABILITIES=ssh", "HOME=" + t.TempDir()}

	result := env.RunGovardWithEnv(t, projectDir, envVars, "deploy", "status", "--json")
	result.AssertExitCode(t, 1)

	var rows []map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &rows); err != nil {
		t.Fatalf("stdout must be exactly one JSON document, got:\n%s\nstderr:\n%s\nerr: %v", result.Stdout, result.Stderr, err)
	}
	if len(rows) != 1 || rows[0]["remote"] != "local" || rows[0]["status"] != "unknown" || rows[0]["error"] == "" || rows[0]["error"] == nil {
		t.Fatalf("the unknown row must be reported, got: %v", rows)
	}
	if !strings.Contains(result.Stderr, "no configured remote could be reached") {
		t.Fatalf("the failure reason must be on stderr, got:\n%s", result.Stderr)
	}

	table := env.RunGovardWithEnv(t, projectDir, envVars, "deploy", "status")
	table.AssertExitCode(t, 1)
}
