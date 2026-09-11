package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// The design claims the two build modes can never drift because they run the
// same task list. That is a claim until the same revision is deployed both ways
// and the two release trees are compared file by file.
//
// This is the test that turns the claim into a fact. The recipe is synthetic so
// the difference it makes is deterministic: one build task writes two files,
// one of them into a directory that does not exist in the checkout.
func TestBuildModesProduceTheSameRelease(t *testing.T) {
	work, revision := seedBuildRepo(t)
	origin := seedOriginFromCheckout(t, work)

	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskVendors, deploy.Task{
		ID:      deploy.TaskVendors,
		Command: "cd {{release_path}} && mkdir -p generated && echo built > generated/out.txt && cp app.php generated/copy.php",
	})

	artifactDir := filepath.Join(t.TempDir(), "artifact")
	if _, err := deploy.BuildArtifactDir(context.Background(), deploy.BuildRequest{
		Recipe:    recipe,
		Options:   deploy.Options{Revision: revision},
		Vars:      deploy.NewVars().Set("revision", revision),
		WorkDir:   work,
		OutputDir: artifactDir,
	}); err != nil {
		t.Fatalf("build artifact: %v", err)
	}

	artifactRoot := runParityDeploy(t, recipe, origin, revision, deploy.BuildArtifact, artifactDir)
	serverRoot := runParityDeploy(t, recipe, origin, revision, deploy.BuildServer, "")

	artifacts := treeSnapshot(t, artifactRoot)
	server := treeSnapshot(t, serverRoot)

	if len(artifacts) == 0 {
		t.Fatal("the artifact deploy published nothing")
	}
	// Without this the test would also pass if neither mode ran the build.
	for _, want := range []string{"app.php", "generated/out.txt", "generated/copy.php"} {
		if _, ok := artifacts[want]; !ok {
			t.Fatalf("the artifact release is missing %s: %v", want, sortedKeys(artifacts))
		}
		if _, ok := server[want]; !ok {
			t.Fatalf("the server release is missing %s: %v", want, sortedKeys(server))
		}
	}
	if len(artifacts) != len(server) {
		t.Fatalf("the two modes published %d and %d files:\nartifact: %v\nserver:   %v",
			len(artifacts), len(server), sortedKeys(artifacts), sortedKeys(server))
	}
	for path, digest := range server {
		other, ok := artifacts[path]
		if !ok {
			t.Errorf("the artifact release is missing %s", path)
			continue
		}
		if other != digest {
			t.Errorf("%s differs between the two modes: %s vs %s", path, digest, other)
		}
	}
}

// runParityDeploy deploys one revision into a fresh deploy path and returns the
// published release directory.
func runParityDeploy(t *testing.T, recipe deploy.Recipe, origin, revision, mode, artifactDir string) string {
	t.Helper()
	deployRoot := t.TempDir()
	host := deploy.HostForTest(deployRoot, deploy.LocalRunner{})

	options := deploy.Options{
		Remote:         "local",
		Branch:         "main",
		Revision:       revision,
		Repository:     origin,
		Publish:        deploy.PublishSymlink,
		KeepReleases:   5,
		Build:          mode,
		ArtifactDir:    artifactDir,
		CommandTimeout: deploy.DefaultCommandTimeout,
		Settings:       map[string]any{},
	}

	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	plan = plan.ForBuildMode(mode)

	vars := deploy.NewVars().
		Set("revision", revision).
		Set("branch", "main").
		SetPath("release_path", "").
		SetPath("deploy_path", deployRoot)

	release := deploy.NewRelease("", revision, "main")
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(
		context.Background(), plan, vars, release); err != nil {
		t.Fatalf("%s deploy: %v", mode, err)
	}

	target, err := os.Readlink(filepath.Join(deployRoot, "current"))
	if err != nil {
		t.Fatalf("%s deploy did not publish a current symlink: %v", mode, err)
	}
	return target
}

// treeSnapshot hashes every published file. `.dep/` is the release's own
// bookkeeping — records, history and step timings — so it is compared by the
// tests that care about it, not here.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if info.IsDir() {
			if relative == ".dep" {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relative)] = hex.EncodeToString(hasher.Sum(nil))
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snapshot
}

func sortedKeys(snapshot map[string]string) []string {
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// seedOriginFromCheckout publishes a checkout as a bare origin, which is what
// the deploy pipeline fetches into its mirror.
func seedOriginFromCheckout(t *testing.T, work string) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin.git")
	run(t, work, "clone", "-q", "--bare", work, origin)
	return origin
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
