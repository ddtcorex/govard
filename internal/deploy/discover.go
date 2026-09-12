package deploy

import (
	"context"
	"fmt"
	"path"
	"strings"
)

// DeployPathCandidates are the layouts a target may already have when its remote
// does not configure `deploy_path`. They are the two shapes the reference
// projects use: the home directory itself, and a hidden subdirectory.
var DeployPathCandidates = []string{"~", "~/.deployer"}

// DiscoverDeployPath returns the deploy path a target already has.
//
// `deploy_path` has no safe default (spec 5.1): guessing one points the whole
// pipeline at a directory nobody chose, which is exactly what the on-disk
// compatibility rule forbids. But refusing to look is also wrong for a project
// migrating an environment whose layout already exists, so the layout is read
// from the server and adopted only when the answer is unambiguous.
//
// A Deployer layout is recognisable from outside: it holds `releases/`,
// `shared/`, `.dep/` or a `current` symlink. Zero matches and several matches
// are both refusals that name what was probed — adopting one of several would be
// the guess this function exists to avoid.
func DiscoverDeployPath(ctx context.Context, host Host) (string, error) {
	return discoverDeployPath(ctx, host, DeployPathCandidates)
}

// DiscoverDeployPathForTest exposes the candidate list so a test can point the
// probe at a temporary directory instead of the real home directory.
func DiscoverDeployPathForTest(ctx context.Context, host Host, candidates []string) (string, error) {
	return discoverDeployPath(ctx, host, candidates)
}

func discoverDeployPath(ctx context.Context, host Host, candidates []string) (string, error) {
	if strings.TrimSpace(host.DeployPath) != "" {
		return host.DeployPath, nil
	}
	if len(candidates) == 0 {
		return "", ErrDeployPathMissing
	}

	matches := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, err := host.Runner().Run(ctx, layoutProbe(candidate), RunOptions{Timeout: shortCommandTimeout}); err == nil {
			matches = append(matches, candidate)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("%w: none of %s holds a deploy layout; set remotes.<name>.deploy_path",
			ErrDeployPathMissing, strings.Join(candidates, ", "))
	default:
		return "", fmt.Errorf("%w: %s all hold a deploy layout; set remotes.<name>.deploy_path to choose one",
			ErrDeployPathMissing, strings.Join(matches, ", "))
	}
}

// layoutProbe is the shell test for "this directory is a deploy root".
func layoutProbe(candidate string) string {
	markers := []string{"releases", "shared", ".dep"}
	tests := make([]string, 0, len(markers)+1)
	for _, marker := range markers {
		tests = append(tests, "[ -d "+Shell(path.Join(candidate, marker))+" ]")
	}
	tests = append(tests, "[ -L "+Shell(path.Join(candidate, "current"))+" ]")
	return strings.Join(tests, " || ")
}
