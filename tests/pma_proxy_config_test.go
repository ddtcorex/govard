package tests

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"

	"github.com/moby/moby/api/types/container"
)

func TestActiveProjectNamesFromContainersForTest(t *testing.T) {
	containers := []container.Summary{
		{
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project": "alpha",
				"com.docker.compose.service": "db",
			},
		},
		{
			State: "exited",
			Labels: map[string]string{
				"com.docker.compose.project": "beta",
				"com.docker.compose.service": "db",
			},
		},
		{
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project": "gamma",
				"com.docker.compose.service": "web",
			},
		},
		{
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project": "proxy",
				"com.docker.compose.service": "db",
			},
		},
		{
			State: "running",
			Names: []string{"/delta-db-1"},
		},
		{
			State: "running",
			Names: []string{"/epsilon-web-1"},
		},
		{
			State: "running",
			Names: []string{"/delta-db-1"},
		},
	}

	got := engine.ActiveProjectNamesFromContainersForTest(containers)
	want := []string{"alpha", "delta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected active project names %v, got %v", want, got)
	}
}

func TestBuildPMAConfigContentForTest_ContainsActiveFilteringAndProjectSelection(t *testing.T) {
	content := engine.BuildPMAConfigContentForTest()

	requiredSnippets := []string{
		"/govard-registry/active-projects.json",
		"$projectToServer",
		"$_GET['project']",
		"legacy links",
		"$_GET['server']",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected PMA config to contain %q", snippet)
		}
	}
}
