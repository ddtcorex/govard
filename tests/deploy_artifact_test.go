package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/engine"
)

// writeArtifactTree creates a small tree with a nested directory, so a manifest
// has to walk rather than list one level.
func writeArtifactTree(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "index.php"), "<?php echo 1;\n")
	writeFile(t, filepath.Join(root, "composer.lock"), `{"packages":[]}`)
	writeFile(t, filepath.Join(root, "vendor", "autoload.php"), "<?php\n")
	writeFile(t, filepath.Join(root, "generated", "code", "A.php"), "<?php\n")
}

func TestArtifactManifestRoundTrips(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root)

	manifest, err := deploy.BuildManifest(root, "abc123", "8.2.11")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if manifest.Revision != "abc123" || manifest.PHPVersion != "8.2.11" {
		t.Fatalf("manifest = %+v, want revision abc123 and php 8.2.11", manifest)
	}
	if manifest.SchemaVersion != deploy.ArtifactManifestSchema {
		t.Fatalf("schema version = %d, want %d", manifest.SchemaVersion, deploy.ArtifactManifestSchema)
	}
	if manifest.FileCount != len(manifest.Files) || manifest.FileCount != 4 {
		t.Fatalf("file count = %d / %d, want 4 files", manifest.FileCount, len(manifest.Files))
	}
	if manifest.ComposerLockSHA256 == "" {
		t.Fatal("composer.lock is present, so its hash must be recorded")
	}

	if err := deploy.WriteManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	loaded, err := deploy.ReadManifest(root)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if loaded.Revision != manifest.Revision || loaded.PHPVersion != manifest.PHPVersion {
		t.Fatalf("loaded = %+v, want %+v", loaded, manifest)
	}
	if loaded.TotalBytes != manifest.TotalBytes {
		t.Fatalf("total bytes = %d, want %d", loaded.TotalBytes, manifest.TotalBytes)
	}
}

func TestArtifactManifestIsDeterministicAndSorted(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root)

	first, err := deploy.BuildManifest(root, "abc123", "8.2.11")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	second, err := deploy.BuildManifest(root, "abc123", "8.2.11")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if len(first.Files) != len(second.Files) {
		t.Fatalf("two runs disagree on the file count: %d vs %d", len(first.Files), len(second.Files))
	}
	for idx := range first.Files {
		if first.Files[idx] != second.Files[idx] {
			t.Fatalf("file %d differs between runs: %+v vs %+v", idx, first.Files[idx], second.Files[idx])
		}
	}
	for idx := 1; idx < len(first.Files); idx++ {
		if first.Files[idx-1].Path >= first.Files[idx].Path {
			t.Fatalf("files are not sorted: %q before %q", first.Files[idx-1].Path, first.Files[idx].Path)
		}
	}
}

func TestArtifactManifestVerifyAcceptsAnUntouchedArtifact(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root)
	manifest, err := deploy.BuildManifest(root, "abc123", "")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := manifest.Verify(root); err != nil {
		t.Fatalf("verify a clean artifact: %v", err)
	}
}

func TestArtifactManifestVerifyCatchesCorruption(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, root string)
	}{
		{
			name: "a modified file",
			damage: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "index.php"), "<?php echo 2;\n")
			},
		},
		{
			name: "a deleted file",
			damage: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "vendor", "autoload.php")); err != nil {
					t.Fatalf("remove: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeArtifactTree(t, root)
			manifest, err := deploy.BuildManifest(root, "abc123", "")
			if err != nil {
				t.Fatalf("build manifest: %v", err)
			}
			tc.damage(t, root)
			err = manifest.Verify(root)
			if err == nil {
				t.Fatal("verify accepted a damaged artifact")
			}
			if !strings.Contains(err.Error(), "index.php") && !strings.Contains(err.Error(), "autoload.php") {
				t.Fatalf("the error must name the damaged path, got %q", err.Error())
			}
		})
	}
}

func TestArtifactManifestExcludesItself(t *testing.T) {
	root := t.TempDir()
	writeArtifactTree(t, root)
	manifest, err := deploy.BuildManifest(root, "abc123", "")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := deploy.WriteManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	// Rebuilding after the manifest was written must produce the same list:
	// manifest.json is evidence, not payload.
	rebuilt, err := deploy.BuildManifest(root, "abc123", "")
	if err != nil {
		t.Fatalf("rebuild manifest: %v", err)
	}
	if len(rebuilt.Files) != len(manifest.Files) {
		t.Fatalf("the manifest leaked into its own file list: %d vs %d", len(rebuilt.Files), len(manifest.Files))
	}
	if err := rebuilt.Verify(root); err != nil {
		t.Fatalf("verify after the manifest was written: %v", err)
	}
}

func TestArtifactManifestWithoutComposerLock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.php"), "<?php echo 1;\n")
	manifest, err := deploy.BuildManifest(root, "abc123", "")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if manifest.ComposerLockSHA256 != "" {
		t.Fatalf("composer.lock hash = %q, want empty on a project without one", manifest.ComposerLockSHA256)
	}
	if manifest.FileCount != 1 {
		t.Fatalf("file count = %d, want 1", manifest.FileCount)
	}
}

func TestReadManifestWithoutAnArtifactIsAnError(t *testing.T) {
	if _, err := deploy.ReadManifest(t.TempDir()); err == nil {
		t.Fatal("want an error when there is no manifest to read")
	}
}

func TestArtifactUploadCommandCopiesLocallyAndRsyncsRemotely(t *testing.T) {
	local := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	localCommand := deploy.ArtifactUploadCommandForTest(local, "/tmp/artifact", "/srv/app/releases/1", false, nil)
	if !strings.Contains(localCommand, "cp -a") {
		t.Fatalf("a local target must be a copy, got %q", localCommand)
	}
	if strings.Contains(localCommand, "rsync") {
		t.Fatalf("a local target must not need rsync, got %q", localCommand)
	}

	remoteHost := deploy.Host{
		Name:       "staging",
		DeployPath: "/srv/app",
		Remote:     engine.RemoteConfig{Host: "staging.example.com", User: "deploy"},
	}
	remoteCommand := deploy.ArtifactUploadCommandForTest(remoteHost, "/tmp/artifact", "/srv/app/releases/1", false, nil)
	for _, want := range []string{"rsync", "--numeric-ids", "manifest.json", "deploy@staging.example.com:/srv/app/releases/1/"} {
		if !strings.Contains(remoteCommand, want) {
			t.Errorf("the remote upload command is missing %q:\n%s", want, remoteCommand)
		}
	}
}

// The upload must not be able to replace the shared paths `deploy:shared` linked
// on the target, even when the artifact it is handed still holds them: the
// artifact is copied over a release whose `app/etc/env.php` is a symlink to the
// target's own configuration, and a regular file would replace it.
func TestArtifactUploadKeepsTheTargetsSharedState(t *testing.T) {
	remoteHost := deploy.Host{
		Name:       "staging",
		DeployPath: "/srv/app",
		Remote:     engine.RemoteConfig{Host: "staging.example.com", User: "deploy"},
	}
	// Driven through the step's own settings, so this pins the wiring as well as
	// the formatting: the exclusion list comes from the recipe's shared paths.
	sc := deploy.StepContextForTest(remoteHost, deploy.Options{
		Settings: map[string]any{
			"shared_files": []string{"app/etc/env.php"},
			"shared_dirs":  []string{"var/log", "pub/media"},
		},
	})
	command := deploy.ArtifactUploadCommandForContextForTest(sc, "/tmp/artifact", "/srv/app/releases/1")

	for _, want := range []string{
		"--exclude='app/etc/env.php'",
		"--exclude='var/log'",
		"--exclude='pub/media'",
	} {
		if !strings.Contains(command, want) {
			t.Errorf("the upload can still replace the target's %s:\n%s", want, command)
		}
	}
	if !strings.Contains(command, "--exclude='manifest.json'") {
		t.Errorf("the manifest exclusion was lost:\n%s", command)
	}
}

// artifactFixture builds a real artifact on disk so CoreArtifact can be driven
// against a local host.
func artifactFixture(t *testing.T, revision string) (string, string) {
	t.Helper()
	root := t.TempDir()
	artifactDir := filepath.Join(root, "artifact")
	releaseDir := filepath.Join(root, "releases", "1")
	writeFile(t, filepath.Join(artifactDir, "app.txt"), "deployed\n")
	writeFile(t, filepath.Join(artifactDir, "vendor", "autoload.php"), "<?php\n")

	manifest, err := deploy.BuildManifest(artifactDir, revision, "8.2.11")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := deploy.WriteManifest(artifactDir, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	return artifactDir, releaseDir
}

func TestCoreArtifactUploadsAndRecordsTheManifest(t *testing.T) {
	artifactDir, releaseDir := artifactFixture(t, "abc123")
	host := deploy.HostForTest(filepath.Dir(filepath.Dir(releaseDir)), deploy.LocalRunner{})

	release := deploy.NewRelease("1", "abc123", "main")
	release.Path = releaseDir
	sc := deploy.StepContextForTest(host, deploy.Options{Build: deploy.BuildArtifact, ArtifactDir: artifactDir})
	sc.Release = release

	if err := deploy.CoreArtifact(context.Background(), sc); err != nil {
		t.Fatalf("core artifact: %v", err)
	}

	if _, err := os.Stat(filepath.Join(releaseDir, "vendor", "autoload.php")); err != nil {
		t.Fatalf("the artifact's files did not reach the release: %v", err)
	}
	// The manifest is evidence, not payload: it lands in .dep next to the
	// release record, not at the release root.
	if _, err := os.Stat(filepath.Join(releaseDir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("the manifest leaked into the release root (%v)", err)
	}
	recorded, err := os.ReadFile(filepath.Join(releaseDir, ".dep", "artifact-manifest.json"))
	if err != nil {
		t.Fatalf("the release does not carry the manifest it was built from: %v", err)
	}
	var loaded deploy.ArtifactManifest
	if err := json.Unmarshal(recorded, &loaded); err != nil {
		t.Fatalf("the recorded manifest is not valid JSON: %v", err)
	}
	if loaded.Revision != "abc123" {
		t.Fatalf("recorded manifest revision = %q, want abc123", loaded.Revision)
	}
	if release.Build.Mode != deploy.BuildArtifact || release.Build.ArtifactRevision != "abc123" {
		t.Fatalf("release build record = %+v, want artifact mode at abc123", release.Build)
	}
	if release.Build.ArtifactFiles == 0 {
		t.Fatal("the build record must carry the manifest's file count")
	}
}

func TestCoreArtifactIsANoOpInServerMode(t *testing.T) {
	artifactDir, releaseDir := artifactFixture(t, "abc123")
	host := deploy.HostForTest(filepath.Dir(filepath.Dir(releaseDir)), deploy.LocalRunner{})

	release := deploy.NewRelease("1", "abc123", "main")
	release.Path = releaseDir
	sc := deploy.StepContextForTest(host, deploy.Options{Build: deploy.BuildServer, ArtifactDir: artifactDir})
	sc.Release = release

	if err := deploy.CoreArtifact(context.Background(), sc); err != nil {
		t.Fatalf("core artifact in server mode: %v", err)
	}
	if _, err := os.Stat(filepath.Join(releaseDir, "app.txt")); !os.IsNotExist(err) {
		t.Fatalf("server mode must not upload an artifact (%v)", err)
	}
}

func TestCoreArtifactRefusesARevisionMismatch(t *testing.T) {
	artifactDir, releaseDir := artifactFixture(t, "abc123")
	host := deploy.HostForTest(filepath.Dir(filepath.Dir(releaseDir)), deploy.LocalRunner{})

	release := deploy.NewRelease("1", "def456", "main")
	release.Path = releaseDir
	sc := deploy.StepContextForTest(host, deploy.Options{Build: deploy.BuildArtifact, ArtifactDir: artifactDir, Revision: "def456"})
	sc.Release = release

	err := deploy.CoreArtifact(context.Background(), sc)
	if err == nil {
		t.Fatal("want a refusal when the artifact was built for another revision")
	}
	for _, want := range []string{"abc123", "def456"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must name both revisions, got %q", err.Error())
		}
	}
	if _, err := os.Stat(filepath.Join(releaseDir, "app.txt")); !os.IsNotExist(err) {
		t.Fatalf("a refused artifact must not be transferred (%v)", err)
	}
}

func TestCoreArtifactRefusesAnEmptyArtifact(t *testing.T) {
	root := t.TempDir()
	artifactDir := filepath.Join(root, "artifact")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	empty, err := deploy.BuildManifest(artifactDir, "abc123", "")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := deploy.WriteManifest(artifactDir, empty); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	releaseDir := filepath.Join(root, "releases", "1")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	release := deploy.NewRelease("1", "abc123", "main")
	release.Path = releaseDir
	sc := deploy.StepContextForTest(host, deploy.Options{Build: deploy.BuildArtifact, ArtifactDir: artifactDir})
	sc.Release = release

	err = deploy.CoreArtifact(context.Background(), sc)
	if err == nil {
		t.Fatal("want a refusal when the artifact holds no files")
	}
	if !strings.Contains(err.Error(), "govard deploy build") {
		t.Fatalf("the refusal must name the command that produces one, got %q", err.Error())
	}
}

// The manifest's hashes are the only thing that separates an artifact CI built
// from one a truncated cache download left behind, so deploy:artifact has to
// check them before the release is populated rather than after a visitor finds
// out. The check itself already existed and was unit-tested; nothing called it.
func TestCoreArtifactRefusesAnArtifactThatNoLongerMatchesItsManifest(t *testing.T) {
	artifactDir, releaseDir := artifactFixture(t, "abc123")
	writeFile(t, filepath.Join(artifactDir, "vendor", "autoload.php"), "<?php // tampered\n")

	host := deploy.HostForTest(filepath.Dir(filepath.Dir(releaseDir)), deploy.LocalRunner{})
	release := deploy.NewRelease("1", "abc123", "main")
	release.Path = releaseDir
	sc := deploy.StepContextForTest(host, deploy.Options{Build: deploy.BuildArtifact, ArtifactDir: artifactDir})
	sc.Release = release

	err := deploy.CoreArtifact(context.Background(), sc)
	if err == nil {
		t.Fatal("deploy:artifact accepted an artifact modified after the build")
	}
	if !strings.Contains(err.Error(), "autoload.php") {
		t.Fatalf("the error must name the damaged file, got %q", err.Error())
	}
	// The check gates the upload: a corrupt artifact must not reach the release.
	if _, statErr := os.Stat(filepath.Join(releaseDir, "vendor", "autoload.php")); statErr == nil {
		t.Fatal("the corrupt artifact was uploaded into the release anyway")
	}
}

// Artifact mode leaves the tasks that need the deployed application to the
// target, and the transfer has to come first: a task that reads the
// application's own configuration cannot run before the application is there.
//
// Measured on a real project: `setup:static-content:deploy` compiles every theme
// and then fails with "The default website isn't defined" on a build machine,
// because the store it asks about lives in the database.
func TestArtifactModeRunsTheApplicationTasksOnTheTargetAfterTheUpload(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	task := recipe.Task(deploy.TaskAssets)
	task.Command = "php bin/console assets:install"
	task.NeedsApplication = true
	recipe.ReplaceTask(task)

	plan, err := deploy.BuildPlanForTest(recipe, nil, "sandbox")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	plan = plan.ForBuildMode(deploy.BuildArtifact)

	artifactAt, assetsAt := -1, -1
	for idx, step := range plan.Steps {
		switch step.ID {
		case deploy.TaskArtifact:
			artifactAt = idx
		case deploy.TaskAssets:
			assetsAt = idx
			if step.Skipped {
				t.Fatalf("a task that needs the application must run on the target, not be skipped: %s", step.SkipReason)
			}
		}
	}
	if artifactAt < 0 || assetsAt < 0 {
		t.Fatalf("the plan is missing deploy:artifact (%d) or the assets task (%d)", artifactAt, assetsAt)
	}
	if artifactAt > assetsAt {
		t.Fatalf("deploy:artifact (step %d) must come before the tasks that need the code it brings (step %d)", artifactAt, assetsAt)
	}
}
