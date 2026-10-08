package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/verify"
)

func canonicalForTest(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestProjectDeleteRemovesVerifyAndAuditStoresOfThatProjectOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("GOVARD_PROJECT_REGISTRY_PATH", filepath.Join(home, "projects.json"))
	fakeDockerForProjectDelete(t)
	root := t.TempDir()
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{Path: root, ProjectName: "stores"}); err != nil {
		t.Fatal(err)
	}

	verifyDir := filepath.Join(home, "verify-runs", verify.ProjectID(root))
	auditDir := filepath.Join(home, "audit", audit.ProjectID(canonicalForTest(t, root), ""))
	otherVerify := filepath.Join(home, "verify-runs", "project-0123456789abcdef")
	otherAudit := filepath.Join(home, "audit", "project-fedcba9876543210")
	for _, dir := range []string{verifyDir, auditDir, otherVerify, otherAudit} {
		writeArtifactFile(t, filepath.Join(dir, "run.json"), "{}")
	}

	c := cmd.RootCommandForTest()
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	c.SetArgs([]string{"project", "delete", "stores", "-f"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{verifyDir, auditDir} {
		if artifactExists(gone) {
			t.Fatalf("%s should be removed", gone)
		}
	}
	for _, kept := range []string{otherVerify, otherAudit} {
		if !artifactExists(kept) {
			t.Fatalf("%s belongs to another project and must stay", kept)
		}
	}
}

func TestProjectStoreDirsAreListedAndSymlinksNeverFollowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	_ = cmd.RootCommandForTest() // registers the store resolvers
	root := t.TempDir()

	verifyLink := filepath.Join(home, "verify-runs", verify.ProjectID(root))
	outside := t.TempDir()
	writeArtifactFile(t, filepath.Join(outside, "precious"), "x")
	if err := os.MkdirAll(filepath.Dir(verifyLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, verifyLink); err != nil {
		t.Fatal(err)
	}
	auditDir := filepath.Join(home, "audit", audit.ProjectID(canonicalForTest(t, root), ""))
	writeArtifactFile(t, filepath.Join(auditDir, "x"), "x")

	art := engine.CollectProjectArtifacts("symlinks", root, nil)
	listed := strings.Join(art.Lines(), "\n")
	if strings.Contains(listed, verifyLink) {
		t.Fatalf("a symlinked store must not be collected:\n%s", listed)
	}
	if !strings.Contains(listed, auditDir) {
		t.Fatalf("audit store must be listed in the confirmation:\n%s", listed)
	}
	engine.RemoveProjectArtifacts(art, io.Discard)
	if !artifactExists(filepath.Join(outside, "precious")) {
		t.Fatal("symlink target was followed")
	}
}

func TestProjectDeleteStaysSilentAboutResourcesThatDoNotExist(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'No resource found to remove for project x' >&2\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stderr bytes.Buffer
	art := engine.ProjectArtifacts{Name: "ghost"}
	if err := engine.DeleteProjectByName(context.Background(), art, io.Discard, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(stderr.String()), "no resource found") {
		t.Fatalf("a missing resource must be silent, got %q", stderr.String())
	}
}

func TestProjectDeleteRemovesTheProjectLintCacheNamespaceOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("GOVARD_PROJECT_REGISTRY_PATH", filepath.Join(home, "projects.json"))
	fakeDockerForProjectDelete(t)
	root := t.TempDir()
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{Path: root, ProjectName: "lintcache"}); err != nil {
		t.Fatal(err)
	}

	canonical := canonicalForTest(t, root)
	projectID := audit.ProjectID(canonical, "")
	lintRoot := audit.DefaultLintCacheRoot(home)
	mine := filepath.Join(lintRoot, audit.LintTargetID(projectID, "project", canonical))
	otherProject := filepath.Join(lintRoot, audit.LintTargetID("project-fedcba9876543210", "project", "/elsewhere"))
	// A module namespace of this project is keyed by an arbitrary module path,
	// so it cannot be attributed from the project root and must stay.
	module := filepath.Join(lintRoot, audit.LintTargetID(projectID, "module_in_project", filepath.Join(canonical, "app/code/Acme/Mod")))
	for _, dir := range []string{mine, otherProject, module} {
		writeArtifactFile(t, filepath.Join(dir, "gen", "state.json"), "{}")
	}

	art := engine.CollectProjectArtifacts("lintcache", root, nil)
	listed := false
	for _, line := range art.Lines() {
		if strings.HasPrefix(line, mine) {
			listed = true
		}
	}
	if !listed {
		t.Errorf("the confirmation must list %s, got %v", mine, art.Lines())
	}

	c := cmd.RootCommandForTest()
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	c.SetArgs([]string{"project", "delete", "lintcache", "-f"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if artifactExists(mine) {
		t.Errorf("%s should be removed", mine)
	}
	for _, kept := range []string{otherProject, module} {
		if !artifactExists(kept) {
			t.Errorf("%s cannot be attributed to this project and must stay", kept)
		}
	}
}
