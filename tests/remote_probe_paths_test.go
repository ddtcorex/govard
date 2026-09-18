package tests

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine/remote"
)

func TestProbeCandidatePathsCoversDeployLayouts(t *testing.T) {
	got := remote.ProbeCandidatePaths("/home/deployer")
	want := []string{"/home/deployer", "/home/deployer/public_html", "/home/deployer/current"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProbeCandidatePaths() = %v, want %v", got, want)
	}
}

func TestProbeCandidatePathsSkipsEmptiesAndDuplicates(t *testing.T) {
	if got := remote.ProbeCandidatePaths(""); len(got) != 0 {
		t.Fatalf("expected no candidates for an empty path, got %v", got)
	}
	got := remote.ProbeCandidatePaths("/srv/app/current")
	for _, path := range got {
		if strings.Count(path, "current") > 1 {
			t.Fatalf("expected no doubled served directory, got %v", got)
		}
	}
	if len(got) == 0 || got[0] != "/srv/app/current" {
		t.Fatalf("expected the configured path first, got %v", got)
	}
}

func TestTryProbeCandidatePathsAdvancesOnFilesNotFound(t *testing.T) {
	calls := 0
	got, err := remote.TryProbeCandidatePaths("/srv/app", func(path string) (string, error) {
		calls++
		if !strings.HasSuffix(path, "current") {
			return "", remote.ErrProbeFilesNotFound
		}
		return "creds@" + path, nil
	})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got != "creds@/srv/app/current" {
		t.Fatalf("probe returned %q", got)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestTryProbeCandidatePathsAbortsOnTransportError(t *testing.T) {
	boom := errors.New("ssh: exit status 255")
	calls := 0
	_, err := remote.TryProbeCandidatePaths("/srv/app", func(string) (string, error) {
		calls++
		return "", boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected the transport error back, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 attempt, got %d", calls)
	}
}

func TestTryProbeCandidatePathsListsTriedPaths(t *testing.T) {
	_, err := remote.TryProbeCandidatePaths("/srv/app", func(string) (string, error) {
		return "", remote.ErrProbeFilesNotFound
	})
	if err == nil {
		t.Fatal("expected an error when every candidate misses")
	}
	for _, path := range []string{"/srv/app", "/srv/app/public_html", "/srv/app/current"} {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("expected error to name %q, got: %v", path, err)
		}
	}
}
