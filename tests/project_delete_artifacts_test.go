package tests

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func fakeDockerForProjectDelete(t *testing.T, labelsWithResources ...string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "docker.log")
	script := `#!/bin/sh
echo "$@" >> "` + logPath + `"
for arg in "$@"; do
  case "$arg" in
    label=com.docker.compose.project=*)
      value="${arg#label=com.docker.compose.project=}"
      for want in ` + strings.Join(labelsWithResources, " ") + `; do
        if [ "$value" = "$want" ]; then echo "id-$value"; fi
      done
      ;;
  esac
done
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func writeArtifactFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func artifactExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestProjectArtifactsWithKnownRootRemoveOnlyThisProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	root := t.TempDir()

	mine := engine.ComposeFilePathWithProfile(root, "myapp", "")
	front := engine.FrontendComposeFilePath(root, "myapp", "")
	writeArtifactFile(t, mine, "x")
	writeArtifactFile(t, mine+".hash", "h")
	writeArtifactFile(t, front, "x")
	decoyCompose := filepath.Join(home, "compose", "myapp-extra-0123456789ab.yml")
	otherRoot := engine.ComposeFilePathWithProfile(t.TempDir(), "myapp", "")
	writeArtifactFile(t, decoyCompose, "x")
	writeArtifactFile(t, otherRoot, "x")
	writeArtifactFile(t, filepath.Join(home, "varnish", "myapp", "default.vcl"), "v")
	writeArtifactFile(t, filepath.Join(home, "varnish", "myapp2", "default.vcl"), "v")
	writeArtifactFile(t, filepath.Join(home, "rabbitmq", "myapp", "rabbitmq.conf"), "r")
	writeArtifactFile(t, filepath.Join(home, "active-projects.json"), `{"projects":["myapp","myapp2"]}`)

	art := engine.CollectProjectArtifacts("myapp", root, nil)
	if len(art.Files) != 3 || len(art.Dirs) != 2 || !art.ActiveEntry {
		t.Fatalf("unexpected artifacts: %+v", art)
	}
	engine.RemoveProjectArtifacts(art, io.Discard)

	for _, gone := range []string{mine, mine + ".hash", front, filepath.Join(home, "varnish", "myapp"), filepath.Join(home, "rabbitmq", "myapp")} {
		if artifactExists(gone) {
			t.Fatalf("%s should be removed", gone)
		}
	}
	for _, kept := range []string{decoyCompose, otherRoot, filepath.Join(home, "varnish", "myapp2")} {
		if !artifactExists(kept) {
			t.Fatalf("%s belongs to another project and must stay", kept)
		}
	}
	data, _ := os.ReadFile(filepath.Join(home, "active-projects.json"))
	if strings.Contains(string(data), `"myapp"`) || !strings.Contains(string(data), "myapp2") {
		t.Fatalf("active-projects.json not pruned exactly: %s", data)
	}
}

func TestProjectArtifactsWithoutRootMatchExactComposeShapeOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	exact := filepath.Join(home, "compose", "myapp-0123456789ab.yml")
	frontend := filepath.Join(home, "compose", "frontend", "myapp-frontend-0123456789ab.yml")
	longer := filepath.Join(home, "compose", "myapp-extra-0123456789ab.yml")
	sibling := filepath.Join(home, "compose", "myapp2-0123456789ab.yml")
	for _, p := range []string{exact, exact + ".hash", frontend, longer, sibling} {
		writeArtifactFile(t, p, "x")
	}
	art := engine.CollectProjectArtifacts("myapp", "", nil)
	engine.RemoveProjectArtifacts(art, io.Discard)
	for _, gone := range []string{exact, exact + ".hash", frontend} {
		if artifactExists(gone) {
			t.Fatalf("%s should be removed", gone)
		}
	}
	for _, kept := range []string{longer, sibling} {
		if !artifactExists(kept) {
			t.Fatalf("%s must stay: it is not this project's file", kept)
		}
	}
}

func TestDiscoverProjectByQueryFindsDeregisteredProjectByName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("GOVARD_PROJECT_REGISTRY_PATH", filepath.Join(home, "projects.json"))
	fakeDockerForProjectDelete(t)
	writeArtifactFile(t, filepath.Join(home, "varnish", "gone", "default.vcl"), "v")

	art, ok := engine.DiscoverProjectByQuery(context.Background(), "gone")
	if !ok || art.Name != "gone" || len(art.Dirs) != 1 {
		t.Fatalf("deregistered project with artifacts not discovered: ok=%v %+v", ok, art)
	}
	if _, ok := engine.DiscoverProjectByQuery(context.Background(), "gon"); ok {
		t.Fatal("a prefix of the name must not match")
	}
}

func TestDiscoverProjectByQueryRefusesReservedAndUnsafeNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	fakeDockerForProjectDelete(t, "proxy", "warden")
	writeArtifactFile(t, filepath.Join(home, "varnish", "proxy", "default.vcl"), "v")
	for _, q := range []string{"proxy", "warden", "", "  ", "..", "../compose"} {
		if _, ok := engine.DiscoverProjectByQuery(context.Background(), q); ok {
			t.Fatalf("query %q must not resolve to a deletable project", q)
		}
	}
	if err := engine.RemoveComposeProjectResources(context.Background(), "proxy", io.Discard, io.Discard); err == nil {
		t.Fatal("removing the proxy compose project must be refused")
	}
}

func TestRemoveComposeProjectResourcesMatchesExactLabelsOnly(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	logPath := fakeDockerForProjectDelete(t, "myapp-frontend", "myapp")
	if err := engine.RemoveComposeProjectResources(context.Background(), "myapp", io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(logPath)
	log := string(data)
	for _, want := range []string{
		"label=com.docker.compose.project=myapp\n",
		"label=com.docker.compose.project=myapp-frontend\n",
		"rm -f id-myapp",
		"network rm id-myapp",
		"volume rm id-myapp-frontend",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker log lacks %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "name=") || strings.Contains(log, "label=com.docker.compose.project=myapp*") {
		t.Fatalf("docker was asked for a non-exact match:\n%s", log)
	}
}

func TestDeleteProjectByNameBringsBothComposeProjectsDownAndCleansHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	logPath := fakeDockerForProjectDelete(t)
	writeArtifactFile(t, filepath.Join(home, "varnish", "gone", "default.vcl"), "v")
	writeArtifactFile(t, filepath.Join(home, "active-projects.json"), `{"projects":["gone"]}`)

	art := engine.CollectProjectArtifacts("gone", "", nil)
	if err := engine.DeleteProjectByName(context.Background(), art, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(logPath)
	for _, want := range []string{"compose -p gone down -v --remove-orphans", "compose -p gone-frontend down -v --remove-orphans"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("docker log lacks %q:\n%s", want, data)
		}
	}
	if artifactExists(filepath.Join(home, "varnish", "gone")) {
		t.Fatal("varnish dir should be removed")
	}
	if b, _ := os.ReadFile(filepath.Join(home, "active-projects.json")); strings.Contains(string(b), "gone") {
		t.Fatalf("active entry should be removed: %s", b)
	}
}

func TestProjectDeleteCommandResolvesDeregisteredProjectByName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("GOVARD_PROJECT_REGISTRY_PATH", filepath.Join(home, "projects.json"))
	logPath := fakeDockerForProjectDelete(t)
	writeArtifactFile(t, filepath.Join(home, "varnish", "gone", "default.vcl"), "v")
	writeArtifactFile(t, filepath.Join(home, "compose", "gone-0123456789ab.yml"), "x")
	writeArtifactFile(t, filepath.Join(home, "active-projects.json"), `{"projects":["gone"]}`)

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"project", "delete", "gone", "-f"})
	if err := root.Execute(); err != nil {
		t.Fatalf("project delete of a deregistered project failed: %v", err)
	}
	for _, gone := range []string{
		filepath.Join(home, "varnish", "gone"),
		filepath.Join(home, "compose", "gone-0123456789ab.yml"),
	} {
		if artifactExists(gone) {
			t.Fatalf("%s should be removed", gone)
		}
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "compose -p gone-frontend down") {
		t.Fatalf("frontend compose project was not brought down:\n%s", data)
	}
}

func TestProjectDeleteCommandStillRejectsUnknownProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("GOVARD_PROJECT_REGISTRY_PATH", filepath.Join(home, "projects.json"))
	fakeDockerForProjectDelete(t)
	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"project", "delete", "never-existed", "-f"})
	if err := root.Execute(); err == nil {
		t.Fatal("deleting a project with no registry entry and no artifacts must fail")
	}
}

func TestProjectDeleteCommandRemovesRegisteredProjectArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)
	t.Setenv("GOVARD_PROJECT_REGISTRY_PATH", filepath.Join(home, "projects.json"))
	logPath := fakeDockerForProjectDelete(t)
	root := t.TempDir()
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{Path: root, ProjectName: "reg"}); err != nil {
		t.Fatal(err)
	}
	compose := engine.ComposeFilePathWithProfile(root, "reg", "")
	writeArtifactFile(t, compose, "x")
	writeArtifactFile(t, compose+".hash", "h")
	writeArtifactFile(t, filepath.Join(home, "varnish", "reg", "default.vcl"), "v")
	writeArtifactFile(t, filepath.Join(home, "varnish", "reg2", "default.vcl"), "v")

	c := cmd.RootCommandForTest()
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	c.SetArgs([]string{"project", "delete", "reg", "-f"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{compose, compose + ".hash", filepath.Join(home, "varnish", "reg")} {
		if artifactExists(gone) {
			t.Fatalf("%s should be removed", gone)
		}
	}
	if !artifactExists(filepath.Join(home, "varnish", "reg2")) {
		t.Fatal("another project's varnish dir must stay")
	}
	if data, _ := os.ReadFile(logPath); !strings.Contains(string(data), "compose -p reg-frontend down") {
		t.Fatalf("frontend project not brought down:\n%s", data)
	}
}
