package tests

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// assetsRecipe produces its output with a command whose result differs on every
// run, so a test can tell "ran again" from "reused" by the stamp alone.
func assetsRecipe(command string) deploy.Recipe {
	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskAssets, deploy.Task{
		ID:               deploy.TaskAssets,
		Command:          command,
		NeedsApplication: true,
		ReusableOutputs:  []string{"pub/static/frontend"},
	})
	return recipe
}

const stampCommand = "cd {{release_path}} && mkdir -p pub/static/frontend && date +%s%N > pub/static/frontend/stamp.txt"

// enterSandboxCheckout makes the work directory the checkout a sandbox deploy
// refreshes its mirror from: the preflight needs the bare mirror to exist and the
// executor reads the checkout from the working directory.
func enterSandboxCheckout(t *testing.T, work string) {
	t.Helper()
	if err := os.MkdirAll(deploy.SandboxMirrorPath(work), 0o755); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

type assetsDeploy struct {
	releasePath string
	output      string
	assets      deploy.StepResult
}

// deployArtifact runs one artifact deploy into deployRoot. Force is on so the
// second deploy of the same revision is not short-circuited as "already live".
func deployArtifact(t *testing.T, deployRoot string, recipe deploy.Recipe, origin, revision, artifactDir string, sandbox bool, settings map[string]any) assetsDeploy {
	t.Helper()
	host := deploy.HostForTest(deployRoot, deploy.LocalRunner{})
	host.Remote.Sandbox = sandbox
	if settings == nil {
		settings = map[string]any{}
	}
	options := deploy.Options{
		Branch:         "main",
		Revision:       revision,
		Repository:     origin,
		Publish:        deploy.PublishSymlink,
		KeepReleases:   5,
		Build:          deploy.BuildArtifact,
		ArtifactDir:    artifactDir,
		CommandTimeout: deploy.DefaultCommandTimeout,
		Force:          true,
		Settings:       settings,
	}
	plan, err := deploy.BuildPlanForTest(recipe, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan = plan.ForBuildMode(deploy.BuildArtifact)
	vars := deploy.NewVars().Set("revision", revision).Set("branch", "main").SetPath("release_path", "").SetPath("deploy_path", deployRoot)
	release := deploy.NewRelease("", revision, "main")
	var out bytes.Buffer
	outcome, err := deploy.NewExecutor(host, options, &out).Run(context.Background(), plan, vars, release)
	if err != nil {
		t.Fatalf("deploy: %v\n%s", err, out.String())
	}
	result := assetsDeploy{releasePath: host.ReleasePath(release.Release), output: out.String()}
	for _, step := range outcome.Steps {
		if step.ID == deploy.TaskAssets {
			result.assets = step
		}
	}
	return result
}

func buildAssetsArtifact(t *testing.T, recipe deploy.Recipe, work, revision string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "artifact")
	if _, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe: recipe, Options: deploy.Options{Revision: revision},
		Vars: deploy.NewVars().Set("revision", revision), WorkDir: work, OutputDir: dir,
	}); err != nil {
		t.Fatalf("build artifact: %v", err)
	}
	return dir
}

func stampOf(t *testing.T, releasePath string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(releasePath, "pub/static/frontend/stamp.txt"))
	if err != nil {
		t.Fatalf("the release has no static content: %v", err)
	}
	return strings.TrimSpace(string(body))
}

func TestSandboxDeployReusesStaticContentForTheSameArtifact(t *testing.T) {
	hermeticPHP(t)
	work, revision := seedBuildRepo(t)
	enterSandboxCheckout(t, work)
	origin := seedOriginFromCheckout(t, work)
	recipe := assetsRecipe(stampCommand)
	artifact := buildAssetsArtifact(t, recipe, work, revision)
	root := t.TempDir()

	first := deployArtifact(t, root, recipe, origin, revision, artifact, true, nil)
	if first.assets.Status != deploy.StepOK {
		t.Fatalf("the first deploy must run the assets task, got %s", first.assets.Status)
	}
	second := deployArtifact(t, root, recipe, origin, revision, artifact, true, nil)
	if second.assets.Status != deploy.StepSkipped {
		t.Fatalf("the same artifact must reuse the static content, got %s:\n%s", second.assets.Status, second.output)
	}
	if !strings.Contains(second.output, "static content reused from release") {
		t.Fatalf("the skip must say why:\n%s", second.output)
	}
	if stampOf(t, first.releasePath) != stampOf(t, second.releasePath) {
		t.Fatal("the reused release must carry the first release's static content")
	}
}

func TestChangedArtifactRunsTheAssetsTask(t *testing.T) {
	hermeticPHP(t)
	work, revision := seedBuildRepo(t)
	enterSandboxCheckout(t, work)
	origin := seedOriginFromCheckout(t, work)
	recipe := assetsRecipe(stampCommand)
	root := t.TempDir()
	first := deployArtifact(t, root, recipe, origin, revision, buildAssetsArtifact(t, recipe, work, revision), true, nil)

	git := func(args ...string) string {
		c := exec.Command("git", args...)
		c.Dir = work
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.com")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	writeFile(t, filepath.Join(work, "app.php"), "<?php echo 'v2';\n")
	git("add", ".")
	git("commit", "-q", "-m", "v2")
	next := git("rev-parse", "HEAD")
	origin2 := seedOriginFromCheckout(t, work)

	second := deployArtifact(t, root, recipe, origin2, next, buildAssetsArtifact(t, recipe, work, next), true, nil)
	if second.assets.Status != deploy.StepOK {
		t.Fatalf("a changed artifact must rebuild its static content, got %s", second.assets.Status)
	}
	if stampOf(t, first.releasePath) == stampOf(t, second.releasePath) {
		t.Fatal("the new release must have been built, not copied")
	}
}

func TestChangedAssetsCommandRunsTheAssetsTask(t *testing.T) {
	hermeticPHP(t)
	work, revision := seedBuildRepo(t)
	enterSandboxCheckout(t, work)
	origin := seedOriginFromCheckout(t, work)
	recipe := assetsRecipe(stampCommand)
	artifact := buildAssetsArtifact(t, recipe, work, revision)
	root := t.TempDir()
	deployArtifact(t, root, recipe, origin, revision, artifact, true, nil)
	// A settings change (themes, locales, jobs) changes the rendered command.
	changed := assetsRecipe(stampCommand + " && echo more >> pub/static/frontend/stamp.txt")
	second := deployArtifact(t, root, changed, origin, revision, artifact, true, nil)
	if second.assets.Status != deploy.StepOK {
		t.Fatalf("a different assets command must run, got %s", second.assets.Status)
	}
}

func TestMissingPreviousStaticContentRunsTheAssetsTask(t *testing.T) {
	hermeticPHP(t)
	work, revision := seedBuildRepo(t)
	enterSandboxCheckout(t, work)
	origin := seedOriginFromCheckout(t, work)
	recipe := assetsRecipe(stampCommand)
	artifact := buildAssetsArtifact(t, recipe, work, revision)
	root := t.TempDir()
	first := deployArtifact(t, root, recipe, origin, revision, artifact, true, nil)
	// Cleanup removed (or someone deleted) the static content the record points at.
	if err := os.RemoveAll(filepath.Join(first.releasePath, "pub/static/frontend")); err != nil {
		t.Fatal(err)
	}
	second := deployArtifact(t, root, recipe, origin, revision, artifact, true, nil)
	if second.assets.Status != deploy.StepOK {
		t.Fatalf("with nothing to reuse the task must run, got %s", second.assets.Status)
	}
	if _, err := os.Stat(filepath.Join(second.releasePath, "pub/static/frontend/stamp.txt")); err != nil {
		t.Fatalf("the release must still end up with static content: %v", err)
	}
}

func TestRealRemoteAlwaysRunsTheAssetsTask(t *testing.T) {
	hermeticPHP(t)
	work, revision := seedBuildRepo(t)
	origin := seedOriginFromCheckout(t, work)
	recipe := assetsRecipe(stampCommand)
	artifact := buildAssetsArtifact(t, recipe, work, revision)
	root := t.TempDir()
	deployArtifact(t, root, recipe, origin, revision, artifact, false, nil)
	second := deployArtifact(t, root, recipe, origin, revision, artifact, false, nil)
	if second.assets.Status != deploy.StepOK {
		t.Fatalf("a real remote must always run the assets task, got %s", second.assets.Status)
	}
	if _, err := os.Stat(filepath.Join(root, ".dep", "sandbox-assets.json")); !os.IsNotExist(err) {
		t.Fatalf("a real remote must not write the sandbox record (%v)", err)
	}
}

func TestTheSandboxReuseCanBeSwitchedOff(t *testing.T) {
	hermeticPHP(t)
	work, revision := seedBuildRepo(t)
	enterSandboxCheckout(t, work)
	origin := seedOriginFromCheckout(t, work)
	recipe := assetsRecipe(stampCommand)
	artifact := buildAssetsArtifact(t, recipe, work, revision)
	root := t.TempDir()
	off := map[string]any{"sandbox_reuse_assets": false}
	deployArtifact(t, root, recipe, origin, revision, artifact, true, off)
	second := deployArtifact(t, root, recipe, origin, revision, artifact, true, off)
	if second.assets.Status != deploy.StepOK {
		t.Fatalf("sandbox_reuse_assets: false must keep the task running, got %s", second.assets.Status)
	}
}

func TestAssetsFingerprintIgnoresCreatedAtButNotFiles(t *testing.T) {
	base := &deploy.ArtifactManifest{
		Revision: "r1", PHPVersion: "8.2", ComposerLockSHA256: "aa", CreatedAt: "2026-01-01T00:00:00Z",
		Files: []deploy.ArtifactFile{{Path: "a.php", SHA256: "11"}, {Path: "b.php", SHA256: "22"}},
	}
	same := *base
	same.CreatedAt = "2027-02-02T00:00:00Z"
	if deploy.AssetsFingerprintForTest(base, "cmd") != deploy.AssetsFingerprintForTest(&same, "cmd") {
		t.Error("the build time must not change the fingerprint")
	}
	for name, mutate := range map[string]func(*deploy.ArtifactManifest){
		"file content": func(m *deploy.ArtifactManifest) {
			m.Files = []deploy.ArtifactFile{{Path: "a.php", SHA256: "11"}, {Path: "b.php", SHA256: "99"}}
		},
		"file added": func(m *deploy.ArtifactManifest) {
			m.Files = append(m.Files, deploy.ArtifactFile{Path: "c.php", SHA256: "33"})
		},
		"revision": func(m *deploy.ArtifactManifest) { m.Revision = "r2" },
		"php":      func(m *deploy.ArtifactManifest) { m.PHPVersion = "8.3" },
		"lock":     func(m *deploy.ArtifactManifest) { m.ComposerLockSHA256 = "bb" },
	} {
		other := *base
		other.Files = append([]deploy.ArtifactFile(nil), base.Files...)
		mutate(&other)
		if deploy.AssetsFingerprintForTest(base, "cmd") == deploy.AssetsFingerprintForTest(&other, "cmd") {
			t.Errorf("a change of %s must change the fingerprint", name)
		}
	}
	if deploy.AssetsFingerprintForTest(base, "cmd") == deploy.AssetsFingerprintForTest(base, "other") {
		t.Error("the rendered command must change the fingerprint")
	}
}
