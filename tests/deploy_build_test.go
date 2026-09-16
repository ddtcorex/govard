package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// seedBuildRepo creates a local git checkout with two revisions and returns the
// checkout and its HEAD. `deploy build` materialises from a checkout, not from
// a bare mirror, so the tests need one.
func seedBuildRepo(t *testing.T) (string, string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	runGit("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(work, "app.php"), "<?php echo 'v1';\n")
	writeFile(t, filepath.Join(work, "composer.lock"), `{"packages":[]}`)
	runGit("add", ".")
	runGit("commit", "-q", "-m", "v1")
	return work, runGit("rev-parse", "HEAD")
}

// buildRecipe returns a recipe whose build stage produces exactly one file, so a
// parity comparison has something deterministic to compare.
func buildRecipe(t *testing.T) deploy.Recipe {
	t.Helper()
	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskVendors,
		deploy.Task{ID: deploy.TaskVendors, Command: "mkdir -p generated && echo built-{{revision}} > generated/build.txt"})
	return recipe
}

func TestBuildArtifactDirMaterialisesTheRevisionAndRecordsIt(t *testing.T) {
	work, revision := seedBuildRepo(t)
	output := filepath.Join(t.TempDir(), "artifact")

	manifest, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    buildRecipe(t),
		Options:   deploy.Options{Revision: revision, CommandTimeout: deploy.DefaultCommandTimeout},
		Vars:      deploy.NewVars().Set("revision", revision),
		WorkDir:   work,
		OutputDir: output,
	})
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}

	// The tracked files come from `git archive`, the built file from the recipe.
	for _, expected := range []string{"app.php", "composer.lock", filepath.Join("generated", "build.txt"), deploy.ArtifactManifestName} {
		if _, err := os.Stat(filepath.Join(output, expected)); err != nil {
			t.Errorf("the artifact is missing %s: %v", expected, err)
		}
	}
	built, err := os.ReadFile(filepath.Join(output, "generated", "build.txt"))
	if err != nil {
		t.Fatalf("read the built file: %v", err)
	}
	if strings.TrimSpace(string(built)) != "built-"+revision {
		t.Fatalf("the build task expanded {{revision}} to %q, want %q", strings.TrimSpace(string(built)), revision)
	}

	if manifest.Revision != revision {
		t.Fatalf("manifest revision = %q, want %q", manifest.Revision, revision)
	}
	if manifest.ComposerLockSHA256 == "" {
		t.Fatal("composer.lock is in the artifact, so its hash must be recorded")
	}

	loaded, err := deploy.ReadManifest(output)
	if err != nil {
		t.Fatalf("read the written manifest: %v", err)
	}
	if err := loaded.Verify(output); err != nil {
		t.Fatalf("the artifact does not verify against its own manifest: %v", err)
	}
}

func TestBuildArtifactDirRunsTheBuildTaskInTheArtifactDirectory(t *testing.T) {
	work, revision := seedBuildRepo(t)
	output := filepath.Join(t.TempDir(), "artifact")

	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskVendors,
		deploy.Task{ID: deploy.TaskVendors, Command: "pwd > where.txt"})

	if _, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    recipe,
		Options:   deploy.Options{Revision: revision},
		Vars:      deploy.NewVars(),
		WorkDir:   work,
		OutputDir: output,
	}); err != nil {
		t.Fatalf("build artifact: %v", err)
	}

	recorded, err := os.ReadFile(filepath.Join(output, "where.txt"))
	if err != nil {
		t.Fatalf("read the working directory record: %v", err)
	}
	// The output directory is a temp path; on macOS /tmp is a symlink, so
	// compare the resolved form.
	resolvedOutput, err := filepath.EvalSymlinks(output)
	if err != nil {
		resolvedOutput = output
	}
	resolvedRecorded, err := filepath.EvalSymlinks(strings.TrimSpace(string(recorded)))
	if err != nil {
		resolvedRecorded = strings.TrimSpace(string(recorded))
	}
	if resolvedRecorded != resolvedOutput {
		t.Fatalf("the build task ran in %q, want the artifact directory %q", resolvedRecorded, resolvedOutput)
	}
}

func TestBuildArtifactDirRefusesANonEmptyOutputDirectory(t *testing.T) {
	work, revision := seedBuildRepo(t)
	output := filepath.Join(t.TempDir(), "artifact")
	writeFile(t, filepath.Join(output, "stale.txt"), "from an earlier build\n")

	_, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    buildRecipe(t),
		Options:   deploy.Options{Revision: revision},
		WorkDir:   work,
		OutputDir: output,
	})
	if err == nil {
		t.Fatal("want an error when the output directory is not empty")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("the error must name the way out, got %q", err.Error())
	}

	if _, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    buildRecipe(t),
		Options:   deploy.Options{Revision: revision},
		Vars:      deploy.NewVars().Set("revision", revision),
		WorkDir:   work,
		OutputDir: output,
		Force:     true,
	}); err != nil {
		t.Fatalf("build artifact with --force: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("--force must clear the directory, but stale.txt survives (%v)", err)
	}
}

func TestBuildArtifactDirWithoutBuildTasksIsStillAnArtifact(t *testing.T) {
	work, revision := seedBuildRepo(t)
	output := filepath.Join(t.TempDir(), "artifact")

	manifest, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    deploy.DefaultRecipe(),
		Options:   deploy.Options{Revision: revision},
		WorkDir:   work,
		OutputDir: output,
	})
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}
	if manifest.Revision != revision {
		t.Fatalf("manifest revision = %q, want %q", manifest.Revision, revision)
	}
	// A recipe with no build tasks still produces the checkout, which is a
	// legitimate artifact for a project that needs no build step.
	if manifest.FileCount != 2 {
		t.Fatalf("file count = %d, want the two tracked files", manifest.FileCount)
	}
}

func TestBuildArtifactDirRequiresTheRevisionInTheCheckout(t *testing.T) {
	work, _ := seedBuildRepo(t)
	_, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    buildRecipe(t),
		Options:   deploy.Options{Revision: strings.Repeat("0", 40)},
		WorkDir:   work,
		OutputDir: filepath.Join(t.TempDir(), "artifact"),
	})
	if err == nil {
		t.Fatal("want an error when the revision is not in the checkout")
	}
	if !strings.Contains(err.Error(), "materialise") {
		t.Fatalf("the error must say what failed, got %q", err.Error())
	}
}

func TestBuildArtifactDirRequiresAnOutputDirectory(t *testing.T) {
	work, revision := seedBuildRepo(t)
	if _, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:  buildRecipe(t),
		Options: deploy.Options{Revision: revision},
		WorkDir: work,
	}); err == nil {
		t.Fatal("want an error when no output directory is given")
	}
}

// countingRunner records how often the local PHP was probed, so "the gate is
// off for a project that has no PHP" is provable on a host without php.
type countingRunner struct {
	base      deploy.Runner
	phpProbes int
}

func (r *countingRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	if strings.Contains(command, "PHP_VERSION") {
		r.phpProbes++
		return deploy.Result{Stdout: "8.3.6\n"}, nil
	}
	return r.base.Run(ctx, command, opts)
}

// seedPlainBuildRepo is the same fixture without a Composer manifest: the
// project is not a PHP project as far as deployment is concerned.
func seedPlainBuildRepo(t *testing.T) (string, string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(work, "manage.py"), "print('hi')\n")
	runGit("add", ".")
	runGit("commit", "-q", "-m", "v1")
	return work, runGit("rev-parse", "HEAD")
}

func TestBuildArtifactDirRecordsPHPForAComposerProject(t *testing.T) {
	work, revision := seedBuildRepo(t)
	runner := &countingRunner{base: deploy.LocalRunner{}}

	manifest, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    deploy.DefaultRecipe(),
		Options:   deploy.Options{Revision: revision},
		WorkDir:   work,
		OutputDir: filepath.Join(t.TempDir(), "artifact"),
		Runner:    runner,
	})
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}
	if runner.phpProbes != 1 {
		t.Fatalf("php probes = %d, want exactly 1 for a Composer project", runner.phpProbes)
	}
	if manifest.PHPVersion != "8.3.6" {
		t.Fatalf("manifest php version = %q, want the probed 8.3.6", manifest.PHPVersion)
	}
}

func TestBuildArtifactDirDoesNotProbePHPForANonPHPProject(t *testing.T) {
	work, revision := seedPlainBuildRepo(t)
	runner := &countingRunner{base: deploy.LocalRunner{}}

	manifest, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    deploy.DefaultRecipe(),
		Options:   deploy.Options{Revision: revision},
		WorkDir:   work,
		OutputDir: filepath.Join(t.TempDir(), "artifact"),
		Runner:    runner,
	})
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}
	// A CI image that happens to ship PHP must not turn a Django project into
	// one whose artifact claims a PHP version the target then has to match.
	if runner.phpProbes != 0 {
		t.Fatalf("php probes = %d, want 0 for a project with no Composer manifest", runner.phpProbes)
	}
	if manifest.PHPVersion != "" {
		t.Fatalf("manifest php version = %q, want empty", manifest.PHPVersion)
	}
}

// An artifact a build machine produces is not allowed to carry the state the
// recipe declares shared: that state belongs to the target, `deploy:shared` links
// it there after the artifact lands, and an artifact that also holds it replaces
// those links with whatever the build machine had.
//
// Measured on a real Magento project: `setup:di:compile` wrote a 78-byte
// `app/etc/env.php` of its own, the artifact carried it, the upload replaced the
// target's 4791-byte shared configuration with it, and the release failed with
// `Connection "default" is not defined` — after a successful upload, with the
// maintenance window already open.
func TestBuildArtifactDirDoesNotCarryTheTargetsSharedState(t *testing.T) {
	work, revision := seedBuildRepo(t)
	output := filepath.Join(t.TempDir(), "artifact")

	recipe := deploy.DefaultRecipe()
	recipe.Defaults = map[string]any{
		"shared_files": []string{"app/etc/env.php"},
		"shared_dirs":  []string{"var/log"},
	}
	// The build writes both paths, exactly as Magento's own build does.
	deploy.OverrideTaskForTest(&recipe, deploy.TaskVendors,
		deploy.Task{ID: deploy.TaskVendors, Command: "mkdir -p app/etc var/log && echo '<?php return [];' > app/etc/env.php && echo log > var/log/system.log"})

	manifest, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    recipe,
		Options:   deploy.Options{Revision: revision},
		Vars:      deploy.NewVars(),
		WorkDir:   work,
		OutputDir: output,
	})
	if err != nil {
		t.Fatalf("build artifact: %v", err)
	}

	for _, shared := range []string{"app/etc/env.php", "var/log"} {
		if _, err := os.Lstat(filepath.Join(output, shared)); !os.IsNotExist(err) {
			t.Errorf("the artifact still carries the target's %s (lstat err = %v)", shared, err)
		}
		for _, file := range manifest.Files {
			if file.Path == shared || strings.HasPrefix(file.Path, shared+"/") {
				t.Errorf("the manifest still lists the target's %s: %s", shared, file.Path)
			}
		}
	}
}

// A task the recipe marks as needing the application must not run on the build
// machine. Before this, `govard deploy build` ran it and died: static content
// deployment compiled every theme and then asked for a store that only exists in
// the target's database.
func TestBuildArtifactDirLeavesApplicationTasksToTheTarget(t *testing.T) {
	work, revision := seedBuildRepo(t)
	output := filepath.Join(t.TempDir(), "artifact")

	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskVendors,
		deploy.Task{ID: deploy.TaskVendors, Command: "echo built > from-the-builder.txt"})
	deploy.OverrideTaskForTest(&recipe, deploy.TaskAssets,
		deploy.Task{ID: deploy.TaskAssets, Command: "echo built > needs-the-application.txt", NeedsApplication: true})

	if _, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    recipe,
		Options:   deploy.Options{Revision: revision},
		Vars:      deploy.NewVars(),
		WorkDir:   work,
		OutputDir: output,
	}); err != nil {
		t.Fatalf("build artifact: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, "from-the-builder.txt")); err != nil {
		t.Fatalf("the builder must still run the tasks it can: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "needs-the-application.txt")); !os.IsNotExist(err) {
		t.Fatalf("a task that needs the application ran on the build machine (stat err = %v)", err)
	}
}

// A task gated on the migration probe must not run on the build machine
// either: there is no database there, so the probe cannot answer and the task
// cannot be decided. It travels to the target like a NeedsApplication task.
func TestBuildArtifactDirLeavesMigrationTasksToTheTarget(t *testing.T) {
	work, revision := seedBuildRepo(t)
	output := filepath.Join(t.TempDir(), "artifact")

	recipe := deploy.DefaultRecipe()
	recipe.MigrationProbe = &deploy.MigrationProbe{
		Title:   "database schema is current",
		Command: "echo probe-stub",
	}
	deploy.OverrideTaskForTest(&recipe, deploy.TaskVendors,
		deploy.Task{ID: deploy.TaskVendors, Command: "echo built > from-the-builder.txt"})
	deploy.OverrideTaskForTest(&recipe, deploy.TaskCompile,
		deploy.Task{ID: deploy.TaskCompile, Command: "echo built > needs-the-migration-decision.txt", NeedsMigration: true})

	if _, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    recipe,
		Options:   deploy.Options{Revision: revision},
		Vars:      deploy.NewVars(),
		WorkDir:   work,
		OutputDir: output,
	}); err != nil {
		t.Fatalf("build artifact: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, "from-the-builder.txt")); err != nil {
		t.Fatalf("the builder must still run the tasks it can: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "needs-the-migration-decision.txt")); !os.IsNotExist(err) {
		t.Fatalf("a task gated on the migration probe ran on the build machine (stat err = %v)", err)
	}
}
