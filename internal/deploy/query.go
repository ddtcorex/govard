package deploy

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// ReleaseEntry is one release as it exists on the target. Foreign marks a
// directory another tool created: govard lists it and never touches it.
type ReleaseEntry struct {
	Release   string
	Revision  string
	Branch    string
	Status    string
	CreatedAt string
	CreatedBy string
	Live      bool
	Foreign   bool
	Number    int
}

// ListReleases reads the target's release directory.
//
// A directory with no govard record is reported as foreign rather than as an
// error: during a migration from another deploy tool both kinds coexist, and
// hiding the other tool's releases would make `deploy releases` lie about what
// is on the server.
func ListReleases(ctx context.Context, host Host) ([]ReleaseEntry, error) {
	entries, err := releaseEntries(ctx, host)
	if err != nil {
		return nil, err
	}

	live := liveReleaseName(ctx, host)
	for idx := range entries {
		entries[idx].Live = entries[idx].Release == live
	}
	return entries, nil
}

// releaseEntries reads the release directory and every record in it, without
// deciding which release is live.
//
// The split matters: naming the live release needs this list (an in-place docroot
// names a revision, and the release that recorded it is what is live), so a
// `ListReleases` that asked `liveReleaseName` and a `liveReleaseName` that asked
// `ListReleases` called each other for as long as the process lived. It fired only
// on the target the in-place strategy exists for — a docroot that is a git
// checkout with a commit — and every level cost three commands, so the deploy hung
// before its first step.
func releaseEntries(ctx context.Context, host Host) ([]ReleaseEntry, error) {
	result, err := host.Runner().Run(ctx, "ls -1 "+Shell(host.ReleasesPath())+" 2>/dev/null || true", RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	entries := make([]ReleaseEntry, 0, 8)
	for _, line := range strings.Split(result.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		entry := ReleaseEntry{Release: name}
		if number, convErr := strconv.Atoi(name); convErr == nil {
			entry.Number = number
		}
		record, readErr := ReadRelease(ctx, host, name)
		if readErr != nil || record.Tool != ReleaseTool {
			entry.Foreign = true
			entries = append(entries, entry)
			continue
		}
		entry.Revision = record.Revision
		entry.Branch = record.Branch
		entry.Status = record.Status
		entry.CreatedAt = record.CreatedAt
		entry.CreatedBy = record.CreatedBy
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Number > entries[j].Number })
	return entries, nil
}

// LiveRelease returns the release record the target is currently serving, or nil
// when nothing deterministic is live (a fresh server, or a directory another tool
// owns). nil is a legitimate answer; an error means the target could not be read.
func LiveRelease(ctx context.Context, host Host) (*Release, error) {
	name := liveReleaseName(ctx, host)
	if name == "" {
		return nil, nil
	}
	record, err := ReadRelease(ctx, host, name)
	if err != nil {
		// The live path may point at a directory no deploy tool recorded.
		return nil, nil
	}
	return record, nil
}

// LiveReleaseForTest exposes LiveRelease to the tests/ package.
func LiveReleaseForTest(ctx context.Context, host Host) (*Release, error) {
	return LiveRelease(ctx, host)
}

// ListReleasesForTest exposes ListReleases to the tests/ package.
func ListReleasesForTest(ctx context.Context, host Host) ([]ReleaseEntry, error) {
	return ListReleases(ctx, host)
}

// liveReleaseName resolves which release directory the target is serving, for
// either publish strategy. An empty answer means "not determinable", which every
// caller must treat as a refusal rather than as "nothing is live".
func liveReleaseName(ctx context.Context, host Host) string {
	runner := host.Runner()
	if result, err := runner.Run(ctx, "readlink -f "+Shell(host.CurrentPath), RunOptions{Timeout: shortCommandTimeout}); err == nil {
		resolved := strings.TrimSpace(result.Stdout)
		if resolved != "" && resolved != host.CurrentPath {
			return path.Base(resolved)
		}
	}

	// An in-place docroot is a git checkout, so its HEAD names the release.
	head, err := runner.Run(ctx, "git -C "+Shell(host.CurrentPath)+" rev-parse HEAD", RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return ""
	}
	revision := strings.TrimSpace(head.Stdout)
	if revision == "" {
		return ""
	}
	entries, err := releaseEntries(ctx, host)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.Foreign && entry.Revision == revision {
			return entry.Release
		}
	}
	return ""
}

// LiveReleaseNameForTest exposes liveReleaseName to the tests/ package.
func LiveReleaseNameForTest(ctx context.Context, host Host) string {
	return liveReleaseName(ctx, host)
}

// SelectRollbackTarget resolves which release a rollback goes back to.
//
// `to` is either a release number or a revision prefix; empty means "the release
// before the live one", which is what an operator means by "roll back". Every
// answer is an existing, govard-recorded release: guessing at a directory
// another tool owns is exactly the mistake rollback must not make.
func SelectRollbackTarget(ctx context.Context, host Host, to string) (*Release, error) {
	entries, err := ListReleases(ctx, host)
	if err != nil {
		return nil, err
	}
	usable := make([]ReleaseEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.Foreign && entry.Status == StatusOK {
			usable = append(usable, entry)
		}
	}
	if len(usable) == 0 {
		return nil, fmt.Errorf("no successfully deployed govard release was found under %s", host.ReleasesPath())
	}

	to = strings.TrimSpace(to)
	if to == "" {
		live := liveReleaseName(ctx, host)
		if live == "" {
			return nil, fmt.Errorf("cannot tell which release %s is serving; pass --to <release>", host.Name)
		}
		liveNumber := numberFor(entries, live)
		for _, entry := range usable {
			// usable is already newest first, so the first older entry is the
			// release the current one replaced.
			if entry.Number < liveNumber {
				return ReadRelease(ctx, host, entry.Release)
			}
		}
		return nil, fmt.Errorf("no release older than %s to roll back to", live)
	}

	if number, convErr := strconv.Atoi(to); convErr == nil {
		for _, entry := range usable {
			if entry.Number == number {
				return ReadRelease(ctx, host, entry.Release)
			}
		}
		return nil, fmt.Errorf("no release %d on %s", number, host.Name)
	}

	for _, entry := range usable {
		if strings.HasPrefix(entry.Revision, to) {
			return ReadRelease(ctx, host, entry.Release)
		}
	}
	return nil, fmt.Errorf("no release of %s matches %q", host.Name, to)
}

// SelectRollbackTargetForTest exposes SelectRollbackTarget to the tests/ package.
func SelectRollbackTargetForTest(ctx context.Context, host Host, to string) (*Release, error) {
	return SelectRollbackTarget(ctx, host, to)
}

func numberFor(entries []ReleaseEntry, release string) int {
	for _, entry := range entries {
		if entry.Release == release {
			return entry.Number
		}
	}
	return 1 << 30
}

// RunStep executes one step of a plan against a host without the executor's
// bookkeeping.
//
// Recovery commands use it because they must not rewrite the release record of
// the release they are recovering: `rollback` changes which release is live, it
// does not produce a new deployment.
func RunStep(ctx context.Context, host Host, opts Options, vars Vars, step Step, release *Release, out io.Writer) error {
	stepVars := vars.Set("release", release.Release)
	if release.Path != "" {
		stepVars = stepVars.SetPath("release_path", release.Path)
	}
	sc := StepContext{Host: host, Runner: host.Runner(), Vars: stepVars, Release: release, Opts: opts, Out: out, Checks: step.Checks}

	// A step the plan marked skipped is not run here either: the executor and
	// this entry point have to agree on what "skipped" means.
	if step.Skipped {
		return nil
	}

	// A rollback runs the same steps a deploy does, and its longest ones (a
	// database restore, a re-activation) are exactly the ones an operator wants to
	// watch: the same writer, terminal flag and heartbeat the executor gives them.
	live := newLiveWriter(out, linePrefix)
	if opts.Verbose && !opts.JSON {
		sc.Live = live
	}
	sc.Terminal = isTerminal(out)
	stopHeartbeat := startHeartbeat(step, live)
	defer stopHeartbeat()

	if step.core != nil {
		return step.core(ctx, &sc)
	}
	if step.Command == "" {
		return nil
	}
	expanded, err := stepVars.Expand(step.Command)
	if err != nil {
		return fmt.Errorf("expand %s: %w", step.ID, err)
	}
	timeout := opts.CommandTimeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	if _, err := sc.Runner.Run(ctx, expanded, RunOptions{Timeout: timeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("%s: %w", step.ID, err)
	}
	return nil
}
