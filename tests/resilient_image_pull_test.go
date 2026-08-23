package tests

import (
	"context"
	"errors"
	"io"
	"testing"

	"govard/internal/engine"
)

func TestResolveElasticsearchBuildVersionKnownMinors(t *testing.T) {
	cases := map[string]string{
		// Minors published on Docker Hub as Govard images must resolve to a
		// concrete upstream patch tag, otherwise the local build FROM line
		// references a non-existent upstream minor-only tag.
		"5.6":  "5.6.16",
		"7.7":  "7.7.1",
		"7.10": "7.10.1",
		"7.16": "7.16.3",
		"8.11": "8.11.4",
		// Pre-existing mappings stay stable.
		"7.9":  "7.9.3",
		"7.17": "7.17.10",
		"6.8":  "6.8.23",
	}
	for tag, expected := range cases {
		if got := engine.ResolveElasticsearchBuildVersionForTest(tag); got != expected {
			t.Errorf("elasticsearch tag %q: expected %q, got %q", tag, expected, got)
		}
	}
}

func TestResolveElasticsearchBuildVersionGenericMinorFallback(t *testing.T) {
	// Unknown major.minor tags fall back to X.Y.0 so the local build targets
	// a real upstream release instead of a non-existent floating minor tag.
	if got := engine.ResolveElasticsearchBuildVersionForTest("8.19"); got != "8.19.0" {
		t.Errorf("expected generic minor fallback 8.19.0, got %q", got)
	}
	// Full patch tags pass through untouched so projects pinned to a specific
	// upstream release (e.g. search_version: "7.17.28") keep building exactly
	// that version.
	if got := engine.ResolveElasticsearchBuildVersionForTest("7.17.28"); got != "7.17.28" {
		t.Errorf("expected patch tag passthrough 7.17.28, got %q", got)
	}
}

func TestRunResilientImagePullsContinuesAfterFailures(t *testing.T) {
	images := []string{
		"ddtcorex/govard-elasticsearch:7.17.28", // pull fails, build fails -> recorded
		"ddtcorex/govard-mariadb:10.6",          // pull succeeds
		"ddtcorex/govard-redis:7.0",             // pull fails, local build succeeds
		"node:24-alpine",                        // third-party: pull fails, no fallback possible
	}

	pullCalls := make([]string, 0)
	pullErrs := map[string]error{
		"ddtcorex/govard-elasticsearch:7.17.28": errors.New("manifest unknown"),
		"ddtcorex/govard-redis:7.0":             errors.New("not found"),
		"node:24-alpine":                        errors.New("not found"),
	}
	buildCalls := make([]string, 0)
	buildErrs := map[string]error{
		"ddtcorex/govard-elasticsearch:7.17.28": errors.New("plugin install failed"),
	}

	result := engine.RunResilientImagePullsForTest(
		images,
		func(_ context.Context, image string) error {
			pullCalls = append(pullCalls, image)
			return pullErrs[image]
		},
		func(image string) error {
			buildCalls = append(buildCalls, image)
			return buildErrs[image]
		},
		func(string) (string, error) {
			return "", errors.New("no local image")
		},
		io.Discard,
		io.Discard,
	)

	// Every image is attempted even though the first one fails completely.
	if len(pullCalls) != len(images) {
		t.Fatalf("expected all %d images to be attempted, pull was called for %v", len(images), pullCalls)
	}

	if len(result.Pulled) != 1 || result.Pulled[0] != "ddtcorex/govard-mariadb:10.6" {
		t.Errorf("unexpected pulled set: %v", result.Pulled)
	}
	if len(result.BuiltLocally) != 1 || result.BuiltLocally[0] != "ddtcorex/govard-redis:7.0" {
		t.Errorf("unexpected built-locally set: %v", result.BuiltLocally)
	}
	if len(result.Failed) != 2 {
		t.Fatalf("expected 2 failures (elasticsearch + node), got %v", result.Failed)
	}
	failedImages := map[string]bool{}
	for _, failure := range result.Failed {
		failedImages[failure.Image] = true
		if failure.Err == nil {
			t.Errorf("failure for %s must carry the underlying error", failure.Image)
		}
	}
	if !failedImages["ddtcorex/govard-elasticsearch:7.17.28"] || !failedImages["node:24-alpine"] {
		t.Errorf("expected elasticsearch and node failures, got %v", failedImages)
	}

	// Third-party images never reach the local build fallback.
	for _, image := range buildCalls {
		if image == "node:24-alpine" {
			t.Error("third-party image node:24-alpine must not be sent to Govard local build")
		}
	}
}

func TestRunResilientImagePullsNilBuildSkipsFallback(t *testing.T) {
	images := []string{"ddtcorex/govard-redis:7.4"}

	result := engine.RunResilientImagePullsForTest(
		images,
		func(_ context.Context, _ string) error { return errors.New("not found") },
		nil,
		func(string) (string, error) {
			return "", errors.New("no local image")
		},
		io.Discard,
		io.Discard,
	)

	if len(result.Failed) != 1 {
		t.Fatalf("expected the image to be recorded as failed without a build fallback, got %+v", result)
	}
	if len(result.BuiltLocally) != 0 {
		t.Errorf("nothing should be built when fallback is disabled: %v", result.BuiltLocally)
	}
}

func TestCanUseExistingLocalImageOnPullFailure(t *testing.T) {
	cases := []struct {
		name     string
		exists   bool
		arch     string
		goos     string
		goarch   string
		expected bool
	}{
		{"existing compatible image is used", true, "amd64", "linux", "amd64", true},
		{"missing image requires build", false, "", "linux", "amd64", false},
		{"incompatible darwin/arm64 image requires rebuild", true, "amd64", "darwin", "arm64", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.CanUseExistingLocalImageOnPullFailureForTest(tc.exists, tc.arch, tc.goos, tc.goarch); got != tc.expected {
				t.Errorf("exists=%v arch=%q os=%s arch=%s: expected %v, got %v", tc.exists, tc.arch, tc.goos, tc.goarch, tc.expected, got)
			}
		})
	}
}

func TestRunResilientImagePullsReusesCompatibleLocalImage(t *testing.T) {
	images := []string{"ddtcorex/govard-elasticsearch:7.17.28"}
	buildCalled := false

	result := engine.RunResilientImagePullsForTest(
		images,
		func(_ context.Context, _ string) error { return errors.New("manifest unknown") },
		func(string) error { buildCalled = true; return nil },
		func(string) (string, error) { return "amd64", nil }, // compatible local image exists
		io.Discard,
		io.Discard,
	)

	if len(result.ReusedLocal) != 1 || result.ReusedLocal[0] != "ddtcorex/govard-elasticsearch:7.17.28" {
		t.Errorf("expected image to be reused from local cache, got %+v", result)
	}
	if len(result.Failed) != 0 {
		t.Errorf("reused image must not be reported as failed: %+v", result.Failed)
	}
	if buildCalled {
		t.Error("local build must not run when a compatible image already exists")
	}
}
