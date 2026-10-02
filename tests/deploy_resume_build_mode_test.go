package tests

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/deploy"
)

// buildMarkerRecipe is the default recipe with a build task that leaves a marker
// and an activation that fails while the marker file named `failAt` is absent.
func buildMarkerRecipe(marker string, failTask string) deploy.Recipe {
	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskVendors, deploy.Task{
		ID: deploy.TaskVendors, Stage: deploy.StageBuild, Command: "echo ran >> " + marker,
	})
	if failTask != "" {
		deploy.OverrideTaskForTest(&recipe, failTask, deploy.Task{
			ID: failTask, Stage: stageOfTask(failTask), Command: "exit 9",
		})
	}
	return recipe
}

// matchingArtifact is an artifact whose manifest names the PHP the hermetic
// stand-in reports, so the parity gate lets the run reach the steps under test.
func matchingArtifact(t *testing.T, revision string) string {
	t.Helper()
	dir, _ := artifactFixture(t, revision)
	manifest, err := deploy.BuildManifest(dir, revision, "8.3.6")
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if err := deploy.WriteManifest(dir, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

func stageOfTask(id string) deploy.Stage {
	if id == deploy.TaskCode {
		return deploy.StagePrepare
	}
	return deploy.StagePublish
}

// A release that began as an artifact release is finished as one. The tester ran
// `deploy --build artifact --artifact-dir ...`, the run failed at verify, and a
// plain `--resume` ran build:vendors and build:compile on the target because the
// mode was re-resolved from flags the retry did not repeat.
func TestResumeContinuesAnArtifactReleaseWithoutBuildingOnTheTarget(t *testing.T) {
	hermeticPHP(t)
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	cfg := deployRemoteConfig(t, root, origin)
	options := deployOptionsForTest(t, cfg, revision)
	artifactDir := matchingArtifact(t, revision)
	marker := filepath.Join(root, "build-ran")

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	ctx := context.Background()
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")

	firstOptions := options
	firstOptions.Build = deploy.BuildArtifact
	firstOptions.ArtifactDir = artifactDir
	brokenPlan, err := deploy.BuildPlanForTest(buildMarkerRecipe(marker, deploy.TaskActivate), nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	first := deploy.NewReleaseForTest("", revision, "main")
	if _, err := deploy.NewExecutor(host, firstOptions, io.Discard).Run(ctx, brokenPlan.ForBuildMode(deploy.BuildArtifact), deploy.NewVars(), first); err == nil {
		t.Fatal("want the first attempt to fail")
	} else {
		t.Logf("first attempt: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the artifact run built on the target")
	}

	// The retry repeats no flags: no --build, no --artifact-dir, so options
	// resolve to a server build.
	incomplete, err := deploy.IncompleteRelease(ctx, host)
	if err != nil || incomplete == nil {
		t.Fatalf("incomplete = %+v (err %v)", incomplete, err)
	}
	if err := deploy.CoreUnlock(ctx, deploy.StepContextForTest(host, options)); err != nil {
		t.Fatalf("clear the stale lock: %v", err)
	}
	resumeOptions := options
	resumeOptions.Resume = true
	resumeOptions.Build = deploy.BuildServer
	resumeOptions, err = cmd.ReconcileResumeBuildModeForTest(incomplete, resumeOptions, "")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if resumeOptions.Build != deploy.BuildArtifact {
		t.Fatalf("resumed build mode = %q, want artifact", resumeOptions.Build)
	}

	goodPlan, err := deploy.BuildPlanForTest(buildMarkerRecipe(marker, ""), nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	outcome, err := deploy.NewExecutor(host, resumeOptions, io.Discard).Run(ctx, goodPlan.ForBuildMode(resumeOptions.Build), deploy.NewVars(), incomplete)
	if err != nil {
		t.Fatalf("resume: %v\nsteps: %+v", err, outcome.Steps)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the resumed run executed a build task on the target")
	}
	skipped := map[string]bool{}
	for _, step := range outcome.Steps {
		if step.Status == deploy.StepSkipped {
			skipped[step.ID] = true
		}
	}
	if !skipped[deploy.TaskArtifact] {
		t.Error("deploy:artifact already succeeded and must not upload the artifact again")
	}
}

// The mode is stamped before the first step can fail, so a release interrupted
// before it ever reached deploy:artifact is still known to be an artifact release.
func TestInterruptedReleaseRecordsItsBuildModeBeforeTheArtifactStep(t *testing.T) {
	hermeticPHP(t)
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	cfg := deployRemoteConfig(t, root, origin)
	options := deployOptionsForTest(t, cfg, revision)
	artifactDir := matchingArtifact(t, revision)
	options.Build = deploy.BuildArtifact
	options.ArtifactDir = artifactDir

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	ctx := context.Background()
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")
	plan, err := deploy.BuildPlanForTest(buildMarkerRecipe(filepath.Join(root, "m"), deploy.TaskCode), nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(ctx, plan.ForBuildMode(deploy.BuildArtifact), deploy.NewVars(), deploy.NewReleaseForTest("", revision, "main")); err == nil {
		t.Fatal("want the attempt to fail at deploy:code")
	}
	incomplete, err := deploy.IncompleteRelease(ctx, host)
	if err != nil || incomplete == nil {
		t.Fatalf("incomplete = %+v (err %v)", incomplete, err)
	}
	if got := deploy.RecordedBuildMode(incomplete); got != deploy.BuildArtifact {
		t.Fatalf("recorded build mode = %q, want artifact", got)
	}
}

func TestResumeRefusesAnExplicitBuildThatContradictsTheRelease(t *testing.T) {
	release := deploy.NewReleaseForTest("3", "abc", "main")
	release.Build.Mode = deploy.BuildArtifact
	_, err := cmd.ReconcileResumeBuildModeForTest(release, deploy.Options{Build: deploy.BuildServer, Resume: true}, deploy.BuildServer)
	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v, want a usage error", err)
	}
	if !strings.Contains(err.Error(), "--build=artifact") {
		t.Fatalf("the message must name the recorded mode: %v", err)
	}
	// Restating the recorded mode is not a contradiction.
	if _, err := cmd.ReconcileResumeBuildModeForTest(release, deploy.Options{Build: deploy.BuildArtifact}, deploy.BuildArtifact); err != nil {
		t.Fatalf("restating the recorded mode must be accepted: %v", err)
	}
}

// A record from before the mode was stamped still tells its mode through the
// steps that succeeded.
func TestRecordedBuildModeReadsLegacyRecordEvidence(t *testing.T) {
	artifact := deploy.NewReleaseForTest("1", "abc", "main")
	artifact.RecordTask(deploy.StepRecord{ID: deploy.TaskArtifact, Status: deploy.StepOK})
	if got := deploy.RecordedBuildMode(artifact); got != deploy.BuildArtifact {
		t.Errorf("artifact evidence = %q", got)
	}
	server := deploy.NewReleaseForTest("2", "abc", "main")
	server.RecordTask(deploy.StepRecord{ID: deploy.TaskVendors, Status: deploy.StepOK})
	if got := deploy.RecordedBuildMode(server); got != deploy.BuildServer {
		t.Errorf("server evidence = %q", got)
	}
	if got := deploy.RecordedBuildMode(deploy.NewReleaseForTest("3", "abc", "main")); got != "" {
		t.Errorf("no evidence = %q, want empty", got)
	}
	// Nothing to contradict: the resolved mode stands.
	options, err := cmd.ReconcileResumeBuildModeForTest(deploy.NewReleaseForTest("3", "abc", "main"), deploy.Options{Build: deploy.BuildServer}, "")
	if err != nil || options.Build != deploy.BuildServer {
		t.Errorf("options = %+v err %v", options, err)
	}
}
