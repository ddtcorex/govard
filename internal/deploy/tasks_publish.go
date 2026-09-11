package deploy

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

// Publish-stage failures.
var (
	// ErrDocrootNotAGitCheckout means the in-place target is a real directory
	// that is not a git checkout, so govard cannot reset it to a revision. It
	// refuses instead of copying over an unknown tree.
	ErrDocrootNotAGitCheckout = errors.New("in-place docroot is not a git checkout")
	// ErrMoveAtomicUnsupported means the target's mv cannot do the atomic
	// rename the symlink swap depends on.
	ErrMoveAtomicUnsupported = errors.New("mv -T is not supported on the target")
	// ErrNoRevisionForActivation means the deploy has nothing to activate.
	ErrNoRevisionForActivation = errors.New("no revision to activate")
)

// ResolvePublishStrategy decides how a release becomes live. `auto` reads the
// server rather than guessing: a missing or symlinked current path means the
// symlink strategy, a real directory means in-place.
func ResolvePublishStrategy(host Host, opts Options) (string, error) {
	switch opts.Publish {
	case PublishSymlink, PublishInPlace:
		return opts.Publish, nil
	case "", PublishAuto:
	default:
		return "", fmt.Errorf("unsupported publish strategy %q", opts.Publish)
	}

	ctx := context.Background()
	runner := host.Runner()
	if _, err := runner.Run(ctx, "test -L "+Shell(host.CurrentPath), RunOptions{Timeout: shortCommandTimeout}); err == nil {
		return PublishSymlink, nil
	}
	if _, err := runner.Run(ctx, "test -e "+Shell(host.CurrentPath), RunOptions{Timeout: shortCommandTimeout}); err == nil {
		return PublishInPlace, nil
	}
	return PublishSymlink, nil
}

// CoreActivate makes the release live.
func CoreActivate(ctx context.Context, sc *StepContext) error {
	strategy, err := ResolvePublishStrategy(sc.Host, sc.Opts)
	if err != nil {
		return err
	}
	releasePath := releasePathOf(sc)
	if releasePath == "" {
		return fmt.Errorf("release path is unknown; deploy:release must run first")
	}

	if sc.Release != nil {
		sc.Release.Publish.Strategy = strategy
		sc.Release.Publish.Docroot = sc.Host.CurrentPath
	}

	switch strategy {
	case PublishSymlink:
		return activateSymlink(ctx, sc, releasePath)
	case PublishInPlace:
		return activateInPlace(ctx, sc, releasePath)
	default:
		return fmt.Errorf("unsupported publish strategy %q", strategy)
	}
}

// activateSymlink swaps the current symlink atomically.
//
// `ln -sfn` alone is not atomic: it unlinks and recreates, leaving a window with
// no symlink. Creating a temporary link and renaming it into place is atomic for
// every observer, at the cost of requiring GNU `mv -T`, which deploy:check
// probes for.
func activateSymlink(ctx context.Context, sc *StepContext, releasePath string) error {
	host := sc.Host
	temporary := host.CurrentPath + ".govard-tmp"
	command := fmt.Sprintf(
		"mkdir -p %s && ln -sfn %s %s && mv -T %s %s",
		Shell(path.Dir(host.CurrentPath)),
		Shell(releasePath),
		Shell(temporary),
		Shell(temporary),
		Shell(host.CurrentPath),
	)
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return fmt.Errorf("swap %s -> %s: %w", host.CurrentPath, releasePath, err)
	}
	return nil
}

// activateInPlace publishes into a real directory.
//
// The order is the whole point: the docroot is reset to the *exact* revision
// (never the branch tip, which may have moved since the release was built), then
// the configured paths are copied with no error swallowing, and the static
// content version file goes last so asset URLs only ever point at files that are
// already present.
func activateInPlace(ctx context.Context, sc *StepContext, releasePath string) error {
	host := sc.Host
	revision := strings.TrimSpace(sc.Opts.Revision)
	if revision == "" {
		revision = strings.TrimSpace(sc.Opts.Tag)
	}
	if revision == "" {
		return ErrNoRevisionForActivation
	}
	if sc.Release != nil {
		sc.Release.Revision = revision
	}

	if _, err := sc.Runner.Run(ctx, "git -C "+Shell(host.CurrentPath)+" rev-parse --git-dir", RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return fmt.Errorf("%w: %s", ErrDocrootNotAGitCheckout, host.CurrentPath)
	}

	repository := strings.TrimSpace(sc.Opts.Repository)
	if repository == "" {
		repository = strings.TrimSpace(host.Repository)
	}
	if repository != "" {
		fetch := "git -C " + Shell(host.CurrentPath) + " fetch -q " + Shell(repository)
		if branch := strings.TrimSpace(sc.Opts.Branch); branch != "" {
			fetch += " " + Shell(branch)
		}
		if _, err := sc.Runner.Run(ctx, fetch, RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
			return fmt.Errorf("fetch into the docroot: %w", err)
		}
	}

	if _, err := sc.Runner.Run(ctx, "git -C "+Shell(host.CurrentPath)+" reset --hard "+Shell(revision), RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
		return fmt.Errorf("reset the docroot to %s: %w", revision, err)
	}

	syncPaths := settingsStringList(sc.Opts.Settings, "sync_paths")
	for _, entry := range syncPaths {
		source := path.Join(releasePath, entry) + "/"
		target := path.Join(host.CurrentPath, entry) + "/"
		command := "mkdir -p " + Shell(path.Join(host.CurrentPath, entry)) + " && rsync -a --delete " + Shell(source) + " " + Shell(target)
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
			return fmt.Errorf("sync %s into the docroot: %w", entry, err)
		}
	}

	// Last, and only when the release actually produced one: this is what makes
	// the asset switch happen after every asset file is in place.
	versionFile := "deployed_version.txt"
	versionSource := path.Join(releasePath, "pub/static", versionFile)
	command := fmt.Sprintf(
		"if [ -e %s ]; then mkdir -p %s && rsync -a %s %s; fi",
		Shell(versionSource),
		Shell(path.Join(host.CurrentPath, "pub/static")),
		Shell(versionSource),
		Shell(path.Join(host.CurrentPath, "pub/static", versionFile)),
	)
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
		return fmt.Errorf("publish the static content version: %w", err)
	}
	return nil
}

// CoreRecord writes the release record and appends the deploy history. It runs
// at the end of publish, before verification, so a failed verification still
// leaves a record of what was deployed.
func CoreRecord(ctx context.Context, sc *StepContext) error {
	if sc.Release == nil {
		return fmt.Errorf("no release record to write")
	}
	if err := WriteRelease(ctx, sc.Host, sc.Release); err != nil {
		return err
	}
	return AppendHistory(ctx, sc.Host, sc.Release)
}
