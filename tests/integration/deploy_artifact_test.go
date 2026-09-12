//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeployArtifactBuildAndDeploy is the two-job CI shape, end to end: one
// command builds an artifact on a machine with the project's toolchain, the
// other deploys it with nothing but the artifact directory.
//
// The build half deliberately runs inside the project checkout, because that is
// what a CI runner has: the revision is already in the local object store. The
// deploy half never looks at the repository's working tree.
func TestDeployArtifactBuildAndDeploy(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-artifact")

	origin, revisions := seedDeployRevisions(t, 1)
	revision := revisions[0]
	deployRoot := t.TempDir()
	writeLocalRemote(t, projectDir, deployRoot, origin)

	// Make the project directory a checkout that has the revision, exactly as
	// a CI job's workspace does.
	runGitIn(t, projectDir, "init", "-q", "-b", "main")
	runGitIn(t, projectDir, "fetch", "-q", origin, "main")

	artifactDir := filepath.Join(t.TempDir(), "artifacts")

	built := env.RunGovard(t, projectDir, "deploy", "build", "local",
		"--output", artifactDir, "--revision", revision)
	built.AssertSuccess(t)
	built.AssertOutputContains(t, "Built artifact")

	manifestPath := filepath.Join(artifactDir, "manifest.json")
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("the build did not write a manifest: %v", err)
	}
	var manifest struct {
		SchemaVersion int    `json:"schema_version"`
		BuildMode     string `json:"build_mode"`
		Revision      string `json:"revision"`
		FileCount     int    `json:"file_count"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("the manifest is not valid JSON: %v", err)
	}
	if manifest.Revision != revision {
		t.Fatalf("manifest revision = %q, want %q", manifest.Revision, revision)
	}
	if manifest.BuildMode != "artifact" {
		t.Fatalf("manifest build mode = %q, want artifact", manifest.BuildMode)
	}
	if manifest.FileCount == 0 {
		t.Fatal("the manifest lists no files")
	}
	if _, err := os.Stat(filepath.Join(artifactDir, "app.txt")); err != nil {
		t.Fatalf("the artifact does not hold the revision's files: %v", err)
	}

	// The deploy job: no repository working tree needed, only the artifact.
	deployed := env.RunGovard(t, projectDir, "deploy", "local",
		"--artifact-dir", artifactDir, "--revision", revision, "--yes")
	deployed.AssertSuccess(t)

	releaseDir := readlink(t, filepath.Join(deployRoot, "public_html"))
	if !strings.HasSuffix(releaseDir, filepath.Join("releases", "1")) {
		t.Fatalf("current -> %q, want the first release", releaseDir)
	}
	if _, err := os.Stat(filepath.Join(releaseDir, "app.txt")); err != nil {
		t.Fatalf("the published release does not hold the artifact's files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(releaseDir, ".dep", "artifact-manifest.json")); err != nil {
		t.Fatalf("the release does not record the artifact manifest it was built from: %v", err)
	}

	record, err := os.ReadFile(filepath.Join(releaseDir, ".dep", "release.json"))
	if err != nil {
		t.Fatalf("read the release record: %v", err)
	}
	var release struct {
		Build struct {
			Mode string `json:"mode"`
		} `json:"build"`
	}
	if err := json.Unmarshal(record, &release); err != nil {
		t.Fatalf("the release record is not valid JSON: %v", err)
	}
	if release.Build.Mode != "artifact" {
		t.Fatalf("release record build mode = %q, want artifact", release.Build.Mode)
	}
}

// TestDeployArtifactModeSkipsTheServerBuild pins what `deploy plan` reports for
// this mode: the build tasks are marked skipped, and the reason says why.
//
// It cannot observe whether the run executes them — the fixture's recipe
// declares no build tasks, so there is nothing to execute either way. That
// contract is pinned by TestExecutorDoesNotRunAStepThePlanMarkedSkipped in the
// unit suite, which puts a marked-skipped build task in front of a real
// executor. Keep the two apart: a plan assertion is not a run assertion, and
// reading this one as proof of the other is how the executor silently ignored
// the skip in the first place.
func TestDeployArtifactModeSkipsTheServerBuild(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-artifact-plan")
	writeLocalRemote(t, projectDir, t.TempDir(), "git@example.invalid:project.git")

	plan := env.RunGovard(t, projectDir, "deploy", "plan", "local", "--revision", "abc1234", "--artifact-dir", "artifacts")
	plan.AssertSuccess(t)
	plan.AssertOutputContains(t, "Build mode: artifact")
	plan.AssertOutputContains(t, "artifact mode: the artifact already provides this")
}

// TestDeployBuildArtifactWithoutAnArtifactDirIsAUsageError pins the refusal: the
// operator asked for a mode the run cannot deliver, and falling back to a server
// build is exactly what the mode exists to avoid.
func TestDeployBuildArtifactWithoutAnArtifactDirIsAUsageError(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-artifact-nodir")

	origin, revisions := seedDeployRevisions(t, 1)
	writeLocalRemote(t, projectDir, t.TempDir(), origin)

	result := env.RunGovard(t, projectDir, "deploy", "local", "--revision", revisions[0], "--build", "artifact", "--yes")
	if result.ExitCode != 2 {
		t.Fatalf("--build=artifact without an artifact exited %d, want 2\nstdout: %s\nstderr: %s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr+result.Stdout, "--artifact-dir") {
		t.Fatalf("the usage error must name the way out, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

// TestDeployBuildRefusesANonEmptyOutput is the guard against shipping a stale
// file: a second build into a used directory must be refused, not merged.
func TestDeployBuildRefusesANonEmptyOutput(t *testing.T) {
	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "deploy-build-stale")

	origin, revisions := seedDeployRevisions(t, 1)
	writeLocalRemote(t, projectDir, t.TempDir(), origin)
	runGitIn(t, projectDir, "init", "-q", "-b", "main")
	runGitIn(t, projectDir, "fetch", "-q", origin, "main")

	artifactDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(artifactDir, "stale.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write the stale file: %v", err)
	}

	result := env.RunGovard(t, projectDir, "deploy", "build", "local",
		"--output", artifactDir, "--revision", revisions[0])
	if result.ExitCode == 0 {
		t.Fatalf("a build into a non-empty directory must fail\nstdout: %s", result.Stdout)
	}
	if !strings.Contains(result.Stderr+result.Stdout, "--force") {
		t.Fatalf("the error must name --force, got:\n%s%s", result.Stdout, result.Stderr)
	}
}

func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Integration", "GIT_AUTHOR_EMAIL=integration@example.com",
		"GIT_COMMITTER_NAME=Integration", "GIT_COMMITTER_EMAIL=integration@example.com",
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
