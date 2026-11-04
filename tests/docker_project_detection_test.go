package tests

import (
	"slices"
	"testing"

	"github.com/moby/moby/api/types/container"

	"govard/internal/engine"
)

func TestGetRunningProjectNamesFromContainersForTestIgnoresNonGovardComposeServices(t *testing.T) {
	containers := []container.Summary{
		{
			Names: []string{"/govard-proxy-caddy"},
			Labels: map[string]string{
				"com.docker.compose.project": "proxy",
				"com.docker.compose.service": "caddy",
			},
		},
		{
			Names: []string{"/sample-project-web-1"},
			Labels: map[string]string{
				"com.docker.compose.project": "sample-project",
				"com.docker.compose.service": "web",
			},
		},
		{
			Names: []string{"/sample-project-redis-1"},
			Labels: map[string]string{
				"com.docker.compose.project": "sample-project",
				"com.docker.compose.service": "redis",
			},
		},
	}

	got := engine.GetRunningProjectNamesFromContainersForTest(containers)
	want := []string{"sample-project"}
	if !slices.Equal(got, want) {
		t.Fatalf("running projects = %v, want %v", got, want)
	}
}

func TestGetRunningProjectNamesFromContainersForTestFallsBackToContainerNamesWhenLabelsMissing(t *testing.T) {
	containers := []container.Summary{
		{Names: []string{"/demo-shop-web-1"}},
		{Names: []string{"/demo-shop-php-1"}},
		{Names: []string{"/demo-shop-redis-1"}},
	}

	got := engine.GetRunningProjectNamesFromContainersForTest(containers)
	want := []string{"demo-shop"}
	if !slices.Equal(got, want) {
		t.Fatalf("running projects = %v, want %v", got, want)
	}
}
