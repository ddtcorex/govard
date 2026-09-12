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

// runInDir runs git in a directory and returns its stdout, failing the test on a
// non-zero exit.
func runInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// seedRelease writes a release record the way a real deploy does, so the query
// helpers are exercised against the same on-disk shape the pipeline produces.
func seedRelease(t *testing.T, host deploy.Host, name, revision string, status string) *deploy.Release {
	t.Helper()
	release := deploy.NewReleaseForTest(name, revision, "main")
	release.Path = host.ReleasePath(name)
	release.Status = status
	release.Publish.Strategy = deploy.PublishSymlink
	if err := deploy.WriteRelease(context.Background(), host, release); err != nil {
		t.Fatalf("write release %s: %v", name, err)
	}
	return release
}

func seedDeployPath(t *testing.T) deploy.Host {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	for _, name := range []string{"releases", "shared", ".dep"} {
		if err := os.MkdirAll(filepath.Join(host.DeployPath, name), 0o755); err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
	}
	return host
}

func pointCurrentAt(t *testing.T, host deploy.Host, release string) {
	t.Helper()
	if err := os.Symlink(host.ReleasePath(release), host.CurrentPath); err != nil {
		t.Fatalf("point current at %s: %v", release, err)
	}
}

func TestListReleasesMarksTheLiveReleaseAndForeignDirectories(t *testing.T) {
	host := seedDeployPath(t)
	seedRelease(t, host, "1", "aaaa1111", deploy.StatusOK)
	seedRelease(t, host, "2", "bbbb2222", deploy.StatusOK)
	seedRelease(t, host, "3", "cccc3333", deploy.StatusRunning)
	pointCurrentAt(t, host, "2")
	// A directory another deploy tool created: govard must report it, never
	// adopt it.
	if err := os.MkdirAll(host.ReleasePath("4"), 0o755); err != nil {
		t.Fatalf("prepare foreign release: %v", err)
	}

	entries, err := deploy.ListReleasesForTest(context.Background(), host)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("listed %d releases, want 4: %+v", len(entries), entries)
	}

	byName := map[string]deploy.ReleaseEntry{}
	for _, entry := range entries {
		byName[entry.Release] = entry
	}
	if !byName["2"].Live {
		t.Error("release 2 is the current target of the symlink but is not marked live")
	}
	if byName["1"].Live {
		t.Error("release 1 is marked live")
	}
	if !byName["4"].Foreign {
		t.Error("a directory with no govard record must be reported as foreign")
	}
	if byName["4"].Revision != "" {
		t.Errorf("foreign release reported a revision %q", byName["4"].Revision)
	}
	if entries[0].Release != "4" {
		t.Errorf("releases are listed %s first, want the newest number", entries[0].Release)
	}
}

func TestLiveReleaseReadsTheRecordTheSymlinkPointsAt(t *testing.T) {
	host := seedDeployPath(t)
	seedRelease(t, host, "1", "aaaa1111", deploy.StatusOK)
	seedRelease(t, host, "2", "bbbb2222", deploy.StatusOK)

	live, err := deploy.LiveReleaseForTest(context.Background(), host)
	if err != nil {
		t.Fatalf("live release: %v", err)
	}
	if live != nil {
		t.Fatalf("a target with no current path reported %q as live", live.Release)
	}

	pointCurrentAt(t, host, "2")
	live, err = deploy.LiveReleaseForTest(context.Background(), host)
	if err != nil {
		t.Fatalf("live release: %v", err)
	}
	if live == nil || live.Release != "2" {
		t.Fatalf("live release = %+v, want release 2", live)
	}
}

func TestSelectRollbackTarget(t *testing.T) {
	newHost := func(t *testing.T) deploy.Host {
		host := seedDeployPath(t)
		seedRelease(t, host, "1", "aaaa1111", deploy.StatusOK)
		seedRelease(t, host, "2", "bbbb2222", deploy.StatusOK)
		seedRelease(t, host, "3", "cccc3333", deploy.StatusOK)
		seedRelease(t, host, "4", "dddd4444", deploy.StatusFailed)
		if err := os.MkdirAll(host.ReleasePath("5"), 0o755); err != nil {
			t.Fatalf("prepare foreign release: %v", err)
		}
		pointCurrentAt(t, host, "3")
		return host
	}

	t.Run("default is the release before the live one", func(t *testing.T) {
		target, err := deploy.SelectRollbackTargetForTest(context.Background(), newHost(t), "")
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		if target.Release != "2" {
			t.Fatalf("target = %s, want 2", target.Release)
		}
	})

	t.Run("by release number", func(t *testing.T) {
		target, err := deploy.SelectRollbackTargetForTest(context.Background(), newHost(t), "1")
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		if target.Release != "1" {
			t.Fatalf("target = %s, want 1", target.Release)
		}
	})

	t.Run("by revision prefix", func(t *testing.T) {
		target, err := deploy.SelectRollbackTargetForTest(context.Background(), newHost(t), "aaaa")
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		if target.Release != "1" {
			t.Fatalf("target = %s, want 1", target.Release)
		}
	})

	t.Run("a failed release is never a rollback target", func(t *testing.T) {
		if _, err := deploy.SelectRollbackTargetForTest(context.Background(), newHost(t), "dddd"); err == nil {
			t.Fatal("selected a release whose own deploy failed")
		}
	})

	t.Run("a directory govard does not own is never a target", func(t *testing.T) {
		if _, err := deploy.SelectRollbackTargetForTest(context.Background(), newHost(t), "5"); err == nil {
			t.Fatal("selected a foreign release directory")
		}
	})

	t.Run("an unknown target is an error", func(t *testing.T) {
		if _, err := deploy.SelectRollbackTargetForTest(context.Background(), newHost(t), "99"); err == nil {
			t.Fatal("want an error for an unknown release number")
		}
	})
}

func TestRunStepExpandsTheReleasePathAndSkipsUnimplementedSteps(t *testing.T) {
	host := seedDeployPath(t)
	release := deploy.NewReleaseForTest("1", "aaaa1111", "main")
	release.Path = host.ReleasePath("1")
	if err := os.MkdirAll(release.Path, 0o755); err != nil {
		t.Fatalf("prepare release: %v", err)
	}

	plan, err := deploy.BuildPlanForTest(deploy.RecipeForTest("test", []deploy.Task{
		{ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "touch " + filepath.Join(host.DeployPath, "activated") + " && test -d {{release_path}}"},
		{ID: deploy.TaskArtifact, Stage: deploy.StageBuild},
	}), nil, "local")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	options := deploy.Options{Remote: "local", CommandTimeout: 30_000_000_000}
	if err := deploy.RunStep(context.Background(), host, options, deploy.NewVars(), plan.Only(deploy.TaskActivate).Steps[0], release, os.Stderr); err != nil {
		t.Fatalf("run activate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(host.DeployPath, "activated")); err != nil {
		t.Fatalf("the step did not run: %v", err)
	}

	// An unimplemented step is a no-op, not an expansion failure.
	if err := deploy.RunStep(context.Background(), host, options, deploy.NewVars(), plan.Only(deploy.TaskArtifact).Steps[0], release, os.Stderr); err != nil {
		t.Fatalf("an unimplemented step must be a no-op, got %v", err)
	}
}

func TestPlanFromAndOnlySelectSubtrees(t *testing.T) {
	plan, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), nil, "local")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	tail, ok := plan.From(deploy.TaskActivate)
	if !ok {
		t.Fatal("From(publish:activate) reported the step missing from the default recipe")
	}
	if tail.Steps[0].ID != deploy.TaskActivate {
		t.Fatalf("tail starts at %s, want %s", tail.Steps[0].ID, deploy.TaskActivate)
	}
	if strings.Contains(strings.Join(tail.StepIDs(), " "), deploy.TaskVendors) {
		t.Fatalf("the tail still contains build steps: %v", tail.StepIDs())
	}

	only := plan.Only(deploy.TaskMaintenanceDisable, deploy.TaskMaintenanceEnable).StepIDs()
	if len(only) != 2 || only[0] != deploy.TaskMaintenanceEnable || only[1] != deploy.TaskMaintenanceDisable {
		t.Fatalf("Only() = %v, want the two maintenance tasks in pipeline order", only)
	}

	if _, ok := plan.From("nope"); ok {
		t.Fatal("From() accepted an id that is not in the plan")
	}
}

// A bound on the commands a query is allowed to run, so an unbounded recursion
// fails as "the query recursed" instead of overflowing the stack in the test
// binary.
type boundedRunner struct {
	base  deploy.Runner
	calls *int
	limit int
}

func (r boundedRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	*r.calls++
	if *r.calls > r.limit {
		return deploy.Result{}, errQueryRecursed
	}
	return r.base.Run(ctx, command, opts)
}

var errQueryRecursed = errRecursed{}

type errRecursed struct{}

func (errRecursed) Error() string { return "the query recursed" }

// `liveReleaseName` resolved an in-place docroot's HEAD by listing the releases,
// and `ListReleases` asked `liveReleaseName` which release is live: the two called
// each other for as long as the process lived. It only fired on the target that
// matters — an in-place docroot that is a git checkout *with a commit*, which is
// what a previously deployed docroot is — and every level cost three ssh commands,
// so the deploy appeared to hang before its first step.
func TestReleaseQueriesDoNotRecurseOnAnInPlaceDocroot(t *testing.T) {
	host := seedDeployPath(t)
	// An in-place docroot: a real directory (not a symlink) that is a git
	// checkout with a commit, so its HEAD names a revision nobody recorded.
	docroot := filepath.Join(host.DeployPath, "public_html")
	if err := os.MkdirAll(docroot, 0o755); err != nil {
		t.Fatalf("mkdir docroot: %v", err)
	}
	runInDir(t, docroot, "git", "init", "-q")
	if err := os.WriteFile(filepath.Join(docroot, "index.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write a file: %v", err)
	}
	runInDir(t, docroot, "git", "add", "-A")
	runInDir(t, docroot, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")

	calls := new(int)
	bounded := deploy.HostForTest(host.DeployPath, boundedRunner{base: deploy.LocalRunner{}, calls: calls, limit: 40})
	bounded.CurrentPath = docroot

	// Both entry points must terminate well inside the bound.
	if name := deploy.LiveReleaseNameForTest(context.Background(), bounded); name != "" {
		t.Fatalf("live release = %q, want empty: no release recorded this revision", name)
	}
	afterName := *calls
	if afterName > 12 {
		t.Fatalf("resolving the live release ran %d commands: the queries are recursing", afterName)
	}
	if _, err := deploy.ListReleasesForTest(context.Background(), bounded); err != nil {
		t.Fatalf("list releases: %v", err)
	}
	if total := *calls; total > 20 {
		t.Fatalf("listing releases ran %d commands: the queries are recursing", total)
	}
}

// The in-place branch still has to answer when a recorded release matches the
// docroot's HEAD: that is which release a rollback and the record's
// `previous_release` mean.
func TestLiveReleaseNameFindsTheRecordOfAnInPlaceDocroot(t *testing.T) {
	host := seedDeployPath(t)
	docroot := filepath.Join(host.DeployPath, "public_html")
	if err := os.MkdirAll(docroot, 0o755); err != nil {
		t.Fatalf("mkdir docroot: %v", err)
	}
	runInDir(t, docroot, "git", "init", "-q")
	if err := os.WriteFile(filepath.Join(docroot, "index.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write a file: %v", err)
	}
	runInDir(t, docroot, "git", "add", "-A")
	runInDir(t, docroot, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")
	head := strings.TrimSpace(runInDir(t, docroot, "git", "rev-parse", "HEAD"))

	seedRelease(t, host, "7", head, deploy.StatusOK)
	host.CurrentPath = docroot

	if name := deploy.LiveReleaseNameForTest(context.Background(), host); name != "7" {
		t.Fatalf("live release = %q, want 7 (the record whose revision is the docroot's HEAD)", name)
	}
}
