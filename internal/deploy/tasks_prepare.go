package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Prepare-stage failures. They are all detectable before anything live changes,
// which is what lets the executor release the lock when one of them fires.
var (
	// ErrLockHeld means another govard deploy holds the lock.
	ErrLockHeld = errors.New("deploy lock is held")
	// ErrDeployerLockHeld means the other deploy tool is mid-deploy. Govard
	// refuses to run rather than race it on the same release directories.
	ErrDeployerLockHeld = errors.New("another deploy tool holds its lock")
	// ErrReleaseExists means the target release directory is already there.
	// Overwriting it could destroy a release the other tool created.
	ErrReleaseExists = errors.New("release directory already exists")
	// ErrSubmodulesUnsupported means the checkout has submodules, which the
	// archive-based materialisation cannot express. Failing loudly is the
	// point: silently dropping submodule content would ship a broken release.
	ErrSubmodulesUnsupported = errors.New("submodules are not supported by the archive checkout")
	// ErrRevisionMissing means the mirror does not contain the revision.
	ErrRevisionMissing = errors.New("revision is not present in the deploy mirror")
	// ErrDeployPathMissing means the deploy path is neither configured nor
	// discoverable, so govard cannot tell where releases live.
	ErrDeployPathMissing = errors.New("deploy_path is not configured and no existing layout was found")
)

// StepContextForTest builds a step context bound to a host. Tests need it
// because StepContext is otherwise assembled by the executor.
func StepContextForTest(host Host, opts Options) *StepContext {
	return &StepContext{
		Host:   host,
		Runner: host.Runner(),
		Vars:   NewVars(),
		Opts:   opts,
		Out:    os.Stderr,
	}
}

// CoreCheck is the prepare-stage preflight. It answers "can this deploy even
// start" before anything is created, so a bad target fails in seconds instead of
// halfway through a build.
//
// It is deliberately made of cheap, independent probes: a check that needs the
// target to be in a particular state would defeat the point.
func CoreCheck(ctx context.Context, sc *StepContext) error {
	host := sc.Host
	if strings.TrimSpace(host.DeployPath) == "" {
		return ErrDeployPathMissing
	}
	opts := RunOptions{Timeout: shortCommandTimeout}

	if _, err := sc.Runner.Run(ctx, "true", opts); err != nil {
		return fmt.Errorf("target %s is not reachable: %w", host.Name, err)
	}

	if _, err := sc.Runner.Run(ctx, "mkdir -p "+Shell(host.DeployPath)+" && test -w "+Shell(host.DeployPath), opts); err != nil {
		return fmt.Errorf("deploy path %s is not writable: %w", host.DeployPath, err)
	}

	if err := checkRepositoryReachable(ctx, sc); err != nil {
		return err
	}

	// The atomic symlink swap needs GNU `mv -T`. Probing now turns a publish
	// failure into a preflight failure, which is the whole point of this step.
	strategy, err := ResolvePublishStrategy(host, sc.Opts)
	if err == nil {
		sc.Notes = append(sc.Notes, "publish strategy: "+strategy)
		if strategy == PublishSymlink {
			if err := probeAtomicRename(ctx, sc); err != nil {
				return err
			}
		}
	}

	if err := checkDiskSpace(ctx, sc); err != nil {
		return err
	}
	if err := checkPHPVersion(ctx, sc); err != nil {
		return err
	}

	// Fail before building when the checkout uses submodules: git archive does
	// not include their content, and a release with empty submodule
	// directories would look successful.
	if sc.WorkDir != "" {
		if _, err := os.Stat(filepath.Join(sc.WorkDir, ".gitmodules")); err == nil {
			return fmt.Errorf("%w: %s/.gitmodules is present", ErrSubmodulesUnsupported, sc.WorkDir)
		}
	}

	return nil
}

// checkRepositoryReachable proves the *target* can reach the repository. The
// most common first-deploy failure is a server without the deploy key, and it is
// worth catching before a release directory exists.
func checkRepositoryReachable(ctx context.Context, sc *StepContext) error {
	repository := strings.TrimSpace(sc.Opts.Repository)
	if repository == "" {
		repository = strings.TrimSpace(sc.Host.Repository)
	}
	branch := strings.TrimSpace(sc.Opts.Branch)
	if repository == "" || branch == "" {
		return nil
	}
	command := "git ls-remote --exit-code " + Shell(repository) + " " + Shell(branch)
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
		return fmt.Errorf("target %s cannot reach %s branch %s (deploy key or network): %w", sc.Host.Name, repository, branch, err)
	}
	sc.Notes = append(sc.Notes, "repository reachable from the target: "+branch)
	return nil
}

// probeAtomicRename verifies the target's mv supports the atomic rename the
// symlink swap depends on.
func probeAtomicRename(ctx context.Context, sc *StepContext) error {
	probeDir := path.Join(sc.Host.DeployPath, ".dep")
	link := path.Join(probeDir, ".govard-mvprobe.link")
	target := path.Join(probeDir, ".govard-mvprobe.target")
	command := fmt.Sprintf(
		"mkdir -p %s && ln -sfn %s %s && mv -T %s %s && rm -f %s %s",
		Shell(probeDir), Shell(probeDir), Shell(link), Shell(link), Shell(target), Shell(target), Shell(link),
	)
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return fmt.Errorf("%w: the symlink swap needs GNU mv (mv -T)", ErrMoveAtomicUnsupported)
	}
	sc.Notes = append(sc.Notes, "atomic symlink rename: supported")
	return nil
}

// checkDiskSpace reports free space and refuses an actually full filesystem. A
// release plus its build output is large, and a deploy that dies at 90% is worse
// than one that never starts.
func checkDiskSpace(ctx context.Context, sc *StepContext) error {
	result, err := sc.Runner.Run(ctx, "df -Pk "+Shell(sc.Host.DeployPath), RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		// `df` is not universal; an unavailable probe must not block a deploy.
		return nil
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) < 2 {
		return nil
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return nil
	}
	availableKB, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return nil
	}
	if availableKB <= 0 {
		return fmt.Errorf("deploy path %s is on a full filesystem", sc.Host.DeployPath)
	}
	sc.Notes = append(sc.Notes, fmt.Sprintf("free space at the deploy path: %.1f GiB", float64(availableKB)/1024/1024))
	return nil
}

// checkPHPVersion compares the target's PHP with the version the project
// declares. Both are opt-in: a project that never set them is not a PHP project
// as far as deployment is concerned, and the core must not assume one.
func checkPHPVersion(ctx context.Context, sc *StepContext) error {
	phpBin := settingsString(sc.Opts.Settings, "php_bin")
	if phpBin == "" {
		return nil
	}
	result, err := sc.Runner.Run(ctx, phpBin+" -r 'echo PHP_VERSION;'", RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return fmt.Errorf("php is not available as %q on the target: %w", phpBin, err)
	}
	actual := strings.TrimSpace(result.Stdout)
	sc.Notes = append(sc.Notes, "php on the target: "+actual)

	want := settingsString(sc.Opts.Settings, "php_version")
	if want == "" {
		return nil
	}
	if !strings.HasPrefix(actual, want) {
		return fmt.Errorf("target runs php %s but the project declares %s", actual, want)
	}
	return nil
}

// CoreLock acquires the deploy lock with a single atomic mkdir. Read-then-write
// over SSH is a race, and a second concurrent deploy against one target is the
// most damaging failure this pipeline can have.
func CoreLock(ctx context.Context, sc *StepContext) error {
	host := sc.Host

	if !sc.Opts.IgnoreDeployerLock {
		if _, err := sc.Runner.Run(ctx, "test -e "+Shell(host.DeployerLockPath()), RunOptions{Timeout: shortCommandTimeout}); err == nil {
			return fmt.Errorf("%w: %s exists (pass --ignore-deployer-lock to override)", ErrDeployerLockHeld, host.DeployerLockPath())
		}
	}

	// The parent must exist before mkdir can be the atomic acquisition, and the
	// distinct exit code keeps "already held" separate from a real failure
	// (permissions, missing path) instead of reporting both as a held lock.
	command := fmt.Sprintf(
		"mkdir -p %s && (mkdir %s 2>/dev/null || exit %d)",
		Shell(host.DepPath()), Shell(host.LockPath()), lockHeldExitCode,
	)
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout}); err != nil {
		var commandErr *CommandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == lockHeldExitCode {
			return fmt.Errorf("%w: %s", ErrLockHeld, host.LockPath())
		}
		return fmt.Errorf("acquire deploy lock: %w", err)
	}

	owner := fmt.Sprintf(
		`{"pid":%d,"actor":%q,"revision":%q,"branch":%q,"host":%q,"started_at":%q}`,
		os.Getpid(), currentActor(), sc.Opts.Revision, sc.Opts.Branch, host.Name, time.Now().UTC().Format(time.RFC3339),
	)
	ownerCommand := fmt.Sprintf("cat > %s <<'%s'\n%s\n%s", Shell(host.LockOwnerPath()), releaseHeredocDelimiter, owner, releaseHeredocDelimiter)
	if _, err := sc.Runner.Run(ctx, ownerCommand, RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return fmt.Errorf("record lock owner: %w", err)
	}
	return nil
}

// lockHeldExitCode is the shell exit code CoreLock uses to signal "another
// deploy holds the lock", as opposed to any other mkdir failure.
const lockHeldExitCode = 9

// CoreUnlock releases the lock.
func CoreUnlock(ctx context.Context, sc *StepContext) error {
	if _, err := sc.Runner.Run(ctx, "rm -rf "+Shell(sc.Host.LockPath()), RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return fmt.Errorf("release deploy lock: %w", err)
	}
	return nil
}

// NextReleaseNumber returns one past the highest existing release number. It
// never reuses a directory, so a release another tool created cannot be
// overwritten.
func NextReleaseNumber(ctx context.Context, host Host) (string, error) {
	result, err := host.Runner().Run(ctx, "ls -1 "+Shell(host.ReleasesPath())+" 2>/dev/null || true", RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return "", fmt.Errorf("list releases: %w", err)
	}

	highest := 0
	for _, line := range strings.Split(result.Stdout, "\n") {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		number, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		if number > highest {
			highest = number
		}
	}
	return strconv.Itoa(highest + 1), nil
}

// NextReleaseNumberForTest exposes NextReleaseNumber to the tests/ package.
func NextReleaseNumberForTest(ctx context.Context, host Host) (string, error) {
	return NextReleaseNumber(ctx, host)
}

// CoreRelease creates the release directory. The number is taken from the
// release record when the caller already fixed it, so a resumed deploy keeps
// the same directory instead of allocating a new one.
func CoreRelease(ctx context.Context, sc *StepContext) error {
	host := sc.Host

	number := ""
	if sc.Release != nil {
		number = strings.TrimSpace(sc.Release.Release)
	}
	if number == "" {
		computed, err := NextReleaseNumber(ctx, host)
		if err != nil {
			return err
		}
		number = computed
	}

	if _, err := sc.Runner.Run(ctx, "test -e "+Shell(host.ReleasePath(number)), RunOptions{Timeout: shortCommandTimeout}); err == nil {
		return fmt.Errorf("%w: %s", ErrReleaseExists, host.ReleasePath(number))
	}

	if _, err := sc.Runner.Run(ctx, "mkdir -p "+Shell(host.ReleasePath(number)), RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return fmt.Errorf("create release %s: %w", number, err)
	}

	if sc.Release != nil {
		sc.Release.Release = number
		sc.Release.Path = host.ReleasePath(number)
	}
	return nil
}

// CoreCode materialises the exact revision into the release from a bare mirror.
// A mirror plus `git archive` replaces a full clone per release; the release
// directory stays self-contained, with no worktree bookkeeping to prune.
func CoreCode(ctx context.Context, sc *StepContext) error {
	host := sc.Host
	releasePath := releasePathOf(sc)
	if releasePath == "" {
		return fmt.Errorf("release path is unknown; deploy:release must run first")
	}

	repository := strings.TrimSpace(sc.Opts.Repository)
	if repository == "" {
		repository = strings.TrimSpace(host.Repository)
	}
	if repository == "" {
		return fmt.Errorf("no repository configured for remote %q", host.Name)
	}
	revision := strings.TrimSpace(sc.Opts.Revision)
	if revision == "" {
		revision = strings.TrimSpace(sc.Opts.Tag)
	}
	if revision == "" {
		return fmt.Errorf("no revision resolved for remote %q", host.Name)
	}

	opts := RunOptions{Timeout: shortCommandTimeout}
	if _, err := sc.Runner.Run(ctx, "test -d "+Shell(host.RepoPath())+" || git init --bare -q "+Shell(host.RepoPath()), opts); err != nil {
		return fmt.Errorf("prepare mirror: %w", err)
	}
	branch := strings.TrimSpace(sc.Opts.Branch)
	fetch := "git --git-dir=" + Shell(host.RepoPath()) + " fetch -q " + Shell(repository)
	if branch != "" {
		fetch += " " + Shell(branch)
	}
	if _, err := sc.Runner.Run(ctx, fetch, RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
		return fmt.Errorf("fetch %s into the mirror: %w", repository, err)
	}

	if _, err := sc.Runner.Run(ctx, "git --git-dir="+Shell(host.RepoPath())+" cat-file -e "+Shell(revision)+"^{commit}", opts); err != nil {
		return fmt.Errorf("%w: %s (branch %q)", ErrRevisionMissing, revision, branch)
	}

	extract := "git --git-dir=" + Shell(host.RepoPath()) + " archive " + Shell(revision) + " | tar -x -C " + Shell(releasePath)
	if _, err := sc.Runner.Run(ctx, extract, RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
		return fmt.Errorf("extract %s: %w", revision, err)
	}
	return nil
}

// CoreShared links configured shared files and directories from shared/ into
// the release, so a release never owns state that must survive it.
func CoreShared(ctx context.Context, sc *StepContext) error {
	releasePath := releasePathOf(sc)
	if releasePath == "" {
		return fmt.Errorf("release path is unknown; deploy:release must run first")
	}
	host := sc.Host

	for _, entry := range settingsStringList(sc.Opts.Settings, "shared_files", "shared_dirs") {
		source := path.Join(host.SharedPath(), entry)
		target := path.Join(releasePath, entry)
		// The link is created only when the shared entry exists: the first
		// deploy of a project legitimately has no shared state yet. There is no
		// `|| true` here — a genuine link failure must fail the deploy.
		command := fmt.Sprintf(
			"mkdir -p %s && if [ -e %s ]; then mkdir -p %s && ln -sfn %s %s; fi",
			Shell(host.SharedPath()),
			Shell(source),
			Shell(path.Dir(target)),
			Shell(source),
			Shell(target),
		)
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fmt.Errorf("link shared entry %s: %w", entry, err)
		}
	}
	return nil
}

// CoreWritable applies write permissions, and ownership when configured. The
// deploying user is frequently not the web server's runtime user, so guessing
// is worse than reporting.
func CoreWritable(ctx context.Context, sc *StepContext) error {
	releasePath := releasePathOf(sc)
	if releasePath == "" {
		return fmt.Errorf("release path is unknown; deploy:release must run first")
	}

	paths := settingsStringList(sc.Opts.Settings, "writable_dirs")
	if len(paths) == 0 {
		paths = []string{"."}
	}
	for _, entry := range paths {
		target := releasePath + "/" + entry
		// A fresh checkout legitimately lacks generated/, pub/static and var/,
		// so the writable step creates them before applying the mode.
		command := "mkdir -p " + Shell(target) + " && chmod -R 0775 " + Shell(target)
		if owner := settingsString(sc.Opts.Settings, "owner"); owner != "" {
			command += " && chown -R " + owner + " " + Shell(target)
		}
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
			return fmt.Errorf("apply writable mode to %s: %w", entry, err)
		}
	}
	return nil
}

func releasePathOf(sc *StepContext) string {
	if sc.Release != nil && sc.Release.Path != "" {
		return sc.Release.Path
	}
	return ""
}

// settingsString reads a string setting.
func settingsString(settings map[string]any, key string) string {
	if settings == nil {
		return ""
	}
	value, ok := settings[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

// settingsStringList reads a list setting, accepting the shapes a YAML author
// naturally writes: a list of strings, or a single string.
func settingsStringList(settings map[string]any, keys ...string) []string {
	if settings == nil {
		return nil
	}
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		raw, ok := settings[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case []string:
			values = append(values, typed...)
		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok {
					values = append(values, text)
				}
			}
		case string:
			values = append(values, typed)
		}
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
