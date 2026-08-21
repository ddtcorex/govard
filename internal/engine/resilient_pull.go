package engine

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// ResilientPullFailure records why one image could not be prepared.
type ResilientPullFailure struct {
	Image string
	Err   error
}

// ResilientPullResult summarizes per-image pull outcomes. Unlike a plain
// "docker compose pull", one broken image never prevents the remaining
// images from being pulled.
type ResilientPullResult struct {
	Pulled []string
	// ReusedLocal lists images whose pull failed but for which a
	// compatible image already exists on this host, so nothing had to be built.
	ReusedLocal  []string
	BuiltLocally []string
	Failed       []ResilientPullFailure
}

// ResilientPullOptions tunes the resilient pull behavior.
type ResilientPullOptions struct {
	// FallbackLocalBuild enables building Govard-managed images locally when
	// their pull fails. When false, every pull failure is fatal.
	FallbackLocalBuild bool
}

// RunResilientImagePullsFromCompose pulls every image referenced by the
// rendered compose file one by one. A failing image falls back to a local
// Govard build when allowed, and never blocks the remaining images.
//
// Missing third-party images (no Govard local build available) always make
// the call fail, because there is no way to materialize them locally.
func RunResilientImagePullsFromCompose(ctx context.Context, composePath string, options ResilientPullOptions, out io.Writer, errOut io.Writer) (ResilientPullResult, error) {
	serviceImages, err := ReadServiceImagesFromCompose(composePath)
	if err != nil {
		return ResilientPullResult{}, fmt.Errorf("read compose service images: %w", err)
	}

	seen := make(map[string]struct{}, len(serviceImages))
	images := make([]string, 0, len(serviceImages))
	for _, image := range serviceImages {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if _, exists := seen[image]; exists {
			continue
		}
		seen[image] = struct{}{}
		images = append(images, image)
	}
	sort.Strings(images)

	var build func(string) error
	if options.FallbackLocalBuild {
		build = func(image string) error {
			dockerRoot, rootErr := ensureDockerAssetsRoot(".")
			if rootErr != nil {
				return fmt.Errorf("resolve docker build contexts: %w", rootErr)
			}
			return buildGovardImageLocally(image, dockerRoot, out, errOut)
		}
	}

	result := runResilientImagePulls(ctx, images, pullSingleImage, build, func(image string) (string, error) {
		inspection, inspectErr := inspectLocalImage(image)
		return inspection.Architecture, inspectErr
	}, out, errOut)

	var unavailable []string
	for _, failure := range result.Failed {
		if _, _, _, ok := parseGovardImageReference(failure.Image); !ok {
			unavailable = append(unavailable, failure.Image)
		}
	}
	if len(unavailable) > 0 {
		return result, fmt.Errorf("failed to pull images without a Govard local build fallback: %s", strings.Join(unavailable, ", "))
	}
	if !options.FallbackLocalBuild && len(result.Failed) > 0 {
		names := make([]string, 0, len(result.Failed))
		for _, failure := range result.Failed {
			names = append(names, failure.Image)
		}
		return result, fmt.Errorf("failed to pull images (local fallback disabled): %s", strings.Join(names, ", "))
	}
	return result, nil
}

func pullSingleImage(ctx context.Context, image string) error {
	command := exec.CommandContext(ctx, "docker", "pull", image)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

// runResilientImagePulls is the injectable core used by production and tests.
func runResilientImagePulls(ctx context.Context, images []string, pull func(context.Context, string) error, build func(string) error, inspect func(string) (string, error), out io.Writer, errOut io.Writer) ResilientPullResult {
	out = normalizeWriter(out)
	errOut = normalizeWriter(errOut)
	result := ResilientPullResult{
		Pulled:       []string{},
		ReusedLocal:  []string{},
		BuiltLocally: []string{},
		Failed:       []ResilientPullFailure{},
	}

	for _, image := range images {
		if ctx != nil && ctx.Err() != nil {
			break
		}

		pullErr := pull(ctx, image)
		if pullErr == nil {
			result.Pulled = append(result.Pulled, image)
			continue
		}
		fmt.Fprintf(errOut, "WARNING: pull %s failed: %v\n", image, pullErr)

		// A failed pull is harmless when a compatible image is already on
		// this host: keep it instead of rebuilding from scratch.
		if arch, inspectErr := inspect(image); inspectErr == nil && canUseExistingLocalImageOnPullFailureWithRuntime(true, arch, runtime.GOOS, runtime.GOARCH) {
			fmt.Fprintf(out, "Pull %s failed but a compatible local image is already present; keeping it.\n", image)
			result.ReusedLocal = append(result.ReusedLocal, image)
			continue
		}

		if build == nil {
			result.Failed = append(result.Failed, ResilientPullFailure{Image: image, Err: pullErr})
			continue
		}
		if _, _, _, ok := parseGovardImageReference(image); !ok {
			fmt.Fprintf(errOut, "WARNING: %s is not a Govard-managed image; no local build fallback available\n", image)
			result.Failed = append(result.Failed, ResilientPullFailure{Image: image, Err: pullErr})
			continue
		}

		fmt.Fprintf(out, "Attempting local Govard build fallback for %s...\n", image)
		buildErr := build(image)
		if buildErr == nil {
			result.BuiltLocally = append(result.BuiltLocally, image)
			continue
		}
		fmt.Fprintf(errOut, "WARNING: local build fallback for %s failed: %v\n", image, buildErr)
		result.Failed = append(result.Failed, ResilientPullFailure{
			Image: image,
			Err:   fmt.Errorf("pull: %v; local build: %w", pullErr, buildErr),
		})
	}
	return result
}

// RunResilientImagePullsForTest exposes the resilient pull core for tests.
func RunResilientImagePullsForTest(
	images []string,
	pull func(context.Context, string) error,
	build func(string) error,
	inspect func(string) (string, error),
	out io.Writer,
	errOut io.Writer,
) ResilientPullResult {
	return runResilientImagePulls(context.Background(), images, pull, build, inspect, out, errOut)
}

// canUseExistingLocalImageOnPullFailureWithRuntime reports whether an image
// that failed to pull can be satisfied by the image already present locally.
func canUseExistingLocalImageOnPullFailureWithRuntime(exists bool, imageArchitecture string, goos string, goarch string) bool {
	return exists && !shouldRebuildGovardImageForHostWithRuntime(imageArchitecture, goos, goarch)
}

// CanUseExistingLocalImageOnPullFailureForTest exposes the local reuse decision for tests.
func CanUseExistingLocalImageOnPullFailureForTest(exists bool, imageArchitecture string, goos string, goarch string) bool {
	return canUseExistingLocalImageOnPullFailureWithRuntime(exists, imageArchitecture, goos, goarch)
}
