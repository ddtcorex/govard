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

// noteStep writes a line about the activation to the deploy's output when there is
// somewhere to write it. A step that quietly leaves the previous deployment's
// directory in place is the failure this whole list exists to prevent, so it says
// so.
func noteStep(sc *StepContext, line string) {
	if sc.Out == nil {
		return
	}
	fmt.Fprint(sc.Out, line)
}

// sharedCovering reports the shared entry that covers a sync path — the path is
// that entry or lives inside it. Syncing it would replace the docroot's own
// content with a symlink that only resolves at the release's depth.
func sharedCovering(entry string, shared []string) (string, bool) {
	cleaned := cleanRelPath(entry)
	if cleaned == "" {
		return "", false
	}
	for _, candidate := range shared {
		sharedClean := cleanRelPath(candidate)
		if sharedClean == "" {
			continue
		}
		if cleaned == sharedClean || strings.HasPrefix(cleaned, sharedClean+"/") {
			return candidate, true
		}
	}
	return "", false
}

// sharedInside returns the shared entries inside a sync path, relative to it, so
// the copy can exclude them instead of deleting the docroot's own copy.
func sharedInside(entry string, shared []string) []string {
	cleaned := cleanRelPath(entry)
	if cleaned == "" {
		return nil
	}
	inside := make([]string, 0, 2)
	for _, candidate := range shared {
		sharedClean := cleanRelPath(candidate)
		if sharedClean == "" {
			continue
		}
		if strings.HasPrefix(sharedClean, cleaned+"/") {
			inside = append(inside, strings.TrimPrefix(sharedClean, cleaned+"/"))
		}
	}
	return inside
}

// cleanRelPath normalises a configured path for comparison: a leading `./` or `/`
// and a trailing slash are noise, and `..` inside a path is not a path this
// pipeline will follow.
func cleanRelPath(entry string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(entry))
	if cleaned == "/" || cleaned == "" {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

// activateInPlace publishes into a real directory.
//
// The order is the whole point: the docroot is reset to the *exact* revision
// (never the branch tip, which may have moved since the release was built), then
// the configured paths are copied with no error swallowing, and the static
// content version file goes last so asset URLs only ever point at files that are
// already present.
//
// There is deliberately no `git fetch` here even though the reset needs the
// revision present: fetching is the pipeline's one network call for an in-place
// target, and it belongs in prepare (prepareInPlaceDocroot) so that a stall
// cannot happen while the site is in maintenance mode. A rollback re-activates
// a revision this docroot already served, so it needs no fetch either.
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

	if _, err := sc.Runner.Run(ctx, "git -C "+Shell(host.CurrentPath)+" reset --hard "+Shell(revision), RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
		return fmt.Errorf("reset the docroot to %s: %w", revision, err)
	}

	syncPaths := settingsStringList(sc.Opts.Settings, "sync_paths")
	shared := settingsStringList(sc.Opts.Settings, "shared_files", "shared_dirs")
	for _, entry := range syncPaths {
		source := path.Join(releasePath, entry)

		// A build step is allowed to produce nothing — `generated/` only exists
		// after `setup:di:compile`, `pub/static/adminhtml` only when the admin area
		// was deployed — and a path with nothing to copy must not fail the
		// activation.
		if _, err := sc.Runner.Run(ctx, "test -e "+Shell(source), RunOptions{Timeout: shortCommandTimeout}); err != nil {
			noteStep(sc, "  - "+entry+": the release did not build it; the docroot keeps its own\n")
			continue
		}

		// A path the release links from `shared/` is a *relative* symlink: copied
		// into a docroot at a different depth it resolves somewhere else, so the
		// docroot keeps its own copy instead.
		if covering, isShared := sharedCovering(entry, shared); isShared {
			noteStep(sc, "  ! "+entry+" is linked from shared/ ("+covering+"); not copied into the docroot\n")
			continue
		}

		command := "mkdir -p " + Shell(path.Join(host.CurrentPath, entry)) + " && rsync -a --delete"
		// A shared path *inside* the entry is excluded rather than deleted: the
		// docroot's own copy of it has to survive the sync.
		for _, inside := range sharedInside(entry, shared) {
			command += " --exclude=" + Shell("/"+inside)
			noteStep(sc, "  ! "+entry+"/"+inside+" is linked from shared/; excluded from the copy\n")
		}
		command += " " + Shell(source+"/") + " " + Shell(path.Join(host.CurrentPath, entry)+"/")
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
