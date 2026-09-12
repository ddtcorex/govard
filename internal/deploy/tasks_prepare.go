package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"govard/internal/conventions"
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
	opts := RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}

	if _, err := sc.Runner.Run(ctx, "true", opts); err != nil {
		// A sandbox that is not answering is almost always a container that is
		// not running, and "unreachable" would send the operator to look at a
		// network that was never involved.
		if sc.Host.Remote.Sandbox {
			return fmt.Errorf("the sandbox container behind remote %q is not running; run `govard deploy sandbox up` and retry: %w", host.Name, err)
		}
		return fmt.Errorf("target %s is not reachable: %w", host.Name, err)
	}

	if err := checkSandboxMirror(ctx, sc); err != nil {
		return err
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
	if err := checkArtifactParity(ctx, sc); err != nil {
		return err
	}
	if err := checkWritableMode(ctx, sc); err != nil {
		return err
	}
	if err := noteInPlaceSyncPaths(ctx, sc); err != nil {
		return err
	}

	if err := noteComposerCredentials(ctx, sc); err != nil {
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

// checkSandboxMirror refreshes the local bare mirror a sandbox container mounts.
//
// The sandbox has no credentials for the real repository and must be able to
// deploy a commit that was never pushed, so the mirror is refreshed from the
// local checkout on the way in. Doing it here rather than in `sandbox up` means
// `govard deploy sandbox` sees the commit the operator just made.
func checkSandboxMirror(ctx context.Context, sc *StepContext) error {
	if !sc.Host.Remote.Sandbox {
		return nil
	}
	root := sc.WorkDir
	if strings.TrimSpace(root) == "" {
		if resolved, err := os.Getwd(); err == nil {
			root = resolved
		}
	}
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("cannot locate the project checkout to refresh the sandbox mirror")
	}
	mirror := SandboxMirrorPath(root)
	if _, err := os.Stat(mirror); err != nil {
		return fmt.Errorf("the sandbox mirror %s is missing; run `govard deploy sandbox up` and retry", mirror)
	}
	if err := RefreshSandboxMirror(ctx, LocalRunner{}, root, mirror); err != nil {
		return err
	}
	sc.Notes = append(sc.Notes, "sandbox mirror refreshed from the local checkout")
	return nil
}

// checkRepositoryReachable proves the *target* can reach the repository and that
// the ref this deploy will actually fetch exists there. The most common
// first-deploy failure is a server without the deploy key, and "forgot to push"
// is the second: both are worth catching before a release directory exists.
//
// The ref is checked fully qualified — `refs/heads/<branch>` or
// `refs/tags/<tag>` — because `git ls-remote <repo> main` matches *any* ref named
// `main`, including a tag, and would then bless a branch that does not exist.
//
// `--revision` names a commit rather than a ref, and `ls-remote` lists refs only,
// so a revision-only deploy proves reachability here and the commit itself in
// deploy:code, which runs before anything live changes.
func checkRepositoryReachable(ctx context.Context, sc *StepContext) error {
	repository := strings.TrimSpace(sc.Opts.Repository)
	if repository == "" {
		repository = strings.TrimSpace(sc.Host.Repository)
	}
	if repository == "" {
		return nil
	}

	ref := ""
	switch {
	case strings.TrimSpace(sc.Opts.Tag) != "":
		ref = qualifyRef("refs/tags/", strings.TrimSpace(sc.Opts.Tag))
	case strings.TrimSpace(sc.Opts.Branch) != "":
		ref = qualifyRef("refs/heads/", strings.TrimSpace(sc.Opts.Branch))
	}

	command := "git ls-remote --exit-code " + Shell(repository)
	if ref != "" {
		command += " " + Shell(ref)
	}
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
		if ref == "" {
			return fmt.Errorf("target %s cannot reach %s (deploy key or network): %w", sc.Host.Name, repository, err)
		}
		return fmt.Errorf("target %s cannot reach %s %s (deploy key, network, or the ref was never pushed): %w", sc.Host.Name, repository, ref, err)
	}
	if ref != "" {
		sc.Notes = append(sc.Notes, "repository reachable from the target: "+ref)
	}
	return nil
}

// qualifyRef prefixes a short ref name, leaving one that is already a full ref
// path alone so `branch: refs/heads/main` keeps working.
func qualifyRef(prefix, name string) string {
	if strings.HasPrefix(name, "refs/") {
		return name
	}
	return prefix + name
}

// CheckRepositoryReachableForTest exposes checkRepositoryReachable to the
// tests/ package.
func CheckRepositoryReachableForTest(ctx context.Context, sc *StepContext) error {
	return checkRepositoryReachable(ctx, sc)
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
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("%w: the symlink swap needs GNU mv (mv -T)", ErrMoveAtomicUnsupported)
	}
	sc.Notes = append(sc.Notes, "atomic symlink rename: supported")
	return nil
}

// checkDiskSpace reports free space and refuses an actually full filesystem. A
// release plus its build output is large, and a deploy that dies at 90% is worse
// than one that never starts.
func checkDiskSpace(ctx context.Context, sc *StepContext) error {
	result, err := sc.Runner.Run(ctx, "df -Pk "+Shell(sc.Host.DeployPath), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live})
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
	result, err := sc.Runner.Run(ctx, phpBin+" -r 'echo PHP_VERSION;'", RunOptions{Timeout: shortCommandTimeout, Out: sc.Live})
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

// checkArtifactParity is the artifact mode's preflight. An artifact was built
// somewhere else, so two facts have to be proven before it is published: it was
// built for the revision being deployed, and it was built for the PHP the target
// runs.
//
// The comparison happens here, on the machine running govard, from the manifest's
// recorded version — which is what lets the deploy job run without PHP of its
// own. Nothing is probed when the manifest records no PHP version: a project
// that is not a PHP project must not be blocked by a gate it never opted into.
func checkArtifactParity(ctx context.Context, sc *StepContext) error {
	if sc.Opts.Build != BuildArtifact {
		return nil
	}

	artifactDir := strings.TrimSpace(sc.Opts.ArtifactDir)
	if artifactDir == "" {
		return fmt.Errorf("artifact mode needs an artifact directory: pass --artifact-dir <dir> or set deploy.artifact_dir")
	}
	manifest, err := ReadManifest(artifactDir)
	if err != nil {
		return err
	}

	revision := strings.TrimSpace(sc.Opts.Revision)
	if revision == "" {
		revision = strings.TrimSpace(sc.Opts.Tag)
	}
	if manifest.Revision != "" && revision != "" && manifest.Revision != revision {
		return fmt.Errorf("the artifact at %s was built for revision %s but %s is being deployed; rebuild it or pass --revision %s",
			artifactDir, manifest.Revision, revision, manifest.Revision)
	}
	sc.Notes = append(sc.Notes, fmt.Sprintf("artifact: %d files, %.1f MiB, revision %s",
		manifest.FileCount, float64(manifest.TotalBytes)/1024/1024, manifest.Revision))

	if manifest.PHPVersion == "" {
		sc.Notes = append(sc.Notes, "artifact records no PHP version; the target's PHP is not compared")
		return nil
	}

	actual, err := probeTargetPHP(ctx, sc)
	if err != nil {
		return fmt.Errorf("the artifact was built with PHP %s but the target's PHP could not be determined (set settings.php_bin, or deploy with --build=server and build on the target): %w",
			manifest.PHPVersion, err)
	}
	sc.Notes = append(sc.Notes, "artifact php "+manifest.PHPVersion+", target php "+actual)
	if !strings.HasPrefix(actual, phpSeries(manifest.PHPVersion)) {
		return fmt.Errorf("the artifact was built with PHP %s but the target runs PHP %s; rebuild it with `govard deploy build` in an image that matches the target, or deploy with --build=server",
			manifest.PHPVersion, actual)
	}
	return nil
}

// probeTargetPHP asks the target which PHP it runs. The binary is the project's
// configured one, or `php` — the same default the recipe commands expand.
func probeTargetPHP(ctx context.Context, sc *StepContext) (string, error) {
	phpBin := settingsString(sc.Opts.Settings, "php_bin")
	if phpBin == "" {
		phpBin = "php"
	}
	result, err := sc.Runner.Run(ctx, phpBin+" -r 'echo PHP_VERSION;'", RunOptions{Timeout: shortCommandTimeout, Out: sc.Live})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

// phpSeries reduces a full version to major.minor, which is the granularity a
// build machine and a server have to agree on. 8.2.11 and 8.2.27 are the same
// series; 8.2 and 8.3 are not.
func phpSeries(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return strings.TrimSpace(version)
	}
	return parts[0] + "." + parts[1]
}

// CoreLock acquires the deploy lock with a single atomic mkdir. Read-then-write
// over SSH is a race, and a second concurrent deploy against one target is the
// most damaging failure this pipeline can have.
func CoreLock(ctx context.Context, sc *StepContext) error {
	host := sc.Host

	if !sc.Opts.IgnoreDeployerLock {
		if _, err := sc.Runner.Run(ctx, "test -e "+Shell(host.DeployerLockPath()), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err == nil {
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
	if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
		var commandErr *CommandError
		if errors.As(err, &commandErr) && commandErr.ExitCode == lockHeldExitCode {
			// Name the holder, not just the path: an operator reading "deploy
			// lock is held" has to know whether it is their own CI run or
			// somebody else's deploy before deciding to unlock it.
			holder, heldFor := DescribeLockOwner(ctx, host)
			if heldFor >= 0 {
				return fmt.Errorf("%w: %s held for %s by %s", ErrLockHeld, host.LockPath(), heldFor.Round(time.Second), holder)
			}
			return fmt.Errorf("%w: %s held by %s", ErrLockHeld, host.LockPath(), holder)
		}
		return fmt.Errorf("acquire deploy lock: %w", err)
	}

	owner := fmt.Sprintf(
		`{"pid":%d,"actor":%q,"revision":%q,"branch":%q,"host":%q,"started_at":%q}`,
		os.Getpid(), currentActor(), sc.Opts.Revision, sc.Opts.Branch, host.Name, time.Now().UTC().Format(time.RFC3339),
	)
	ownerCommand := fmt.Sprintf("cat > %s", Shell(host.LockOwnerPath()))
	if _, err := sc.Runner.Run(ctx, ownerCommand, RunOptions{Timeout: shortCommandTimeout, Out: sc.Live, Stdin: owner}); err != nil {
		return fmt.Errorf("record lock owner: %w", err)
	}
	return nil
}

// lockHeldExitCode is the shell exit code CoreLock uses to signal "another
// deploy holds the lock", as opposed to any other mkdir failure.
const lockHeldExitCode = 9

// LockOwner is who holds the deploy lock, as recorded in owner.json.
type LockOwner struct {
	PID       int    `json:"pid"`
	Actor     string `json:"actor"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	Host      string `json:"host"`
	StartedAt string `json:"started_at"`
}

// DescribeLockOwner renders the lock's holder and how long it has been held.
//
// "unknown" and -1 are legitimate answers: the lock may predate these fields or
// owner.json may be unreadable, and refusing to talk about it would be worse
// than naming it unknown. A -1 duration means "cannot say", which every caller
// must treat as "not provably stale".
func DescribeLockOwner(ctx context.Context, host Host) (string, time.Duration) {
	return describeLockOwner(ctx, host, time.Now())
}

// DescribeLockOwnerForTest pins the clock so a test can assert the held-for
// duration.
func DescribeLockOwnerForTest(ctx context.Context, host Host, now time.Time) (string, time.Duration) {
	return describeLockOwner(ctx, host, now)
}

func describeLockOwner(ctx context.Context, host Host, now time.Time) (string, time.Duration) {
	result, err := host.Runner().Run(ctx, "cat "+Shell(host.LockOwnerPath()), RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return "unknown", -1
	}
	var owner LockOwner
	if err := json.Unmarshal([]byte(result.Stdout), &owner); err != nil {
		return "unknown", -1
	}

	who := strings.TrimSpace(owner.Actor)
	if who == "" {
		who = "unknown"
	}
	if revision := strings.TrimSpace(owner.Revision); revision != "" {
		who += " at " + ShortRevision(revision)
	}
	// The machine and the process are part of the answer to "is this mine?": an
	// operator reading the refusal has to be able to tell their own CI run from
	// a colleague's deploy, and the lock file has recorded both all along.
	if machine := strings.TrimSpace(owner.Host); machine != "" {
		who += " on " + machine
	}
	if owner.PID > 0 {
		who += " (pid " + strconv.Itoa(owner.PID) + ")"
	}

	started, err := time.Parse(time.RFC3339, strings.TrimSpace(owner.StartedAt))
	if err != nil {
		return who, -1
	}
	return who, now.Sub(started)
}

// CoreUnlock releases the lock.
func CoreUnlock(ctx context.Context, sc *StepContext) error {
	if _, err := sc.Runner.Run(ctx, "rm -rf "+Shell(sc.Host.LockPath()), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
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

	if _, err := sc.Runner.Run(ctx, "test -e "+Shell(host.ReleasePath(number)), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err == nil {
		return fmt.Errorf("%w: %s", ErrReleaseExists, host.ReleasePath(number))
	}

	if _, err := sc.Runner.Run(ctx, "mkdir -p "+Shell(host.ReleasePath(number)), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
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
//
// For the in-place strategy it is also where the docroot's own clone is brought
// up to date, before anything is created: see prepareInPlaceDocroot.
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

	// The docroot half of code preparation comes first, so a target that cannot
	// be reset is refused before a mirror is fetched for it.
	if err := prepareInPlaceDocroot(ctx, sc, repository); err != nil {
		return err
	}

	opts := RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}
	if _, err := sc.Runner.Run(ctx, "test -d "+Shell(host.RepoPath())+" || git init --bare -q "+Shell(host.RepoPath()), opts); err != nil {
		return fmt.Errorf("prepare mirror: %w", err)
	}
	branch := strings.TrimSpace(sc.Opts.Branch)
	fetch := "git --git-dir=" + Shell(host.RepoPath()) + " fetch -q " + Shell(repository)
	if branch != "" {
		fetch += " " + Shell(branch)
	}
	if _, err := sc.Runner.Run(ctx, fetch, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("fetch %s into the mirror: %w", repository, err)
	}

	if _, err := sc.Runner.Run(ctx, "git --git-dir="+Shell(host.RepoPath())+" cat-file -e "+Shell(revision)+"^{commit}", opts); err != nil {
		return fmt.Errorf("%w: %s (branch %q)", ErrRevisionMissing, revision, branch)
	}

	extract := "git --git-dir=" + Shell(host.RepoPath()) + " archive " + Shell(revision) + " | tar -x -C " + Shell(releasePath)
	if _, err := sc.Runner.Run(ctx, extract, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("extract %s: %w", revision, err)
	}
	return nil
}

// prepareInPlaceDocroot transfers the objects an in-place activation will need
// into the docroot's own clone, while prepare is still running.
//
// The in-place publish resets the docroot to the deployed revision, and that
// reset can only succeed once the objects are local. Fetching them next to the
// reset would put the pipeline's one network call inside an open maintenance
// window, where a stalled fetch holds the site down for as long as the command
// timeout allows — the failure mode spec 9.2 singles out as unacceptable. It
// happens here instead, before any window exists.
//
// A symlink target has no docroot to fetch into, so this is a no-op there.
// `git fetch <repository> <branch>` also follows the tags that point into the
// fetched history, which is what makes a `--tag` deploy resolvable afterwards.
func prepareInPlaceDocroot(ctx context.Context, sc *StepContext, repository string) error {
	strategy, err := ResolvePublishStrategy(sc.Host, sc.Opts)
	if err != nil {
		return err
	}
	if strategy != PublishInPlace {
		return nil
	}
	if repository == "" {
		// Nothing to fetch from. The docroot must already hold the revision;
		// the reset in publish:activate reports it clearly if it does not.
		return nil
	}
	if _, err := sc.Runner.Run(ctx, "git -C "+Shell(sc.Host.CurrentPath)+" rev-parse --git-dir", RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("%w: %s", ErrDocrootNotAGitCheckout, sc.Host.CurrentPath)
	}
	fetch := "git -C " + Shell(sc.Host.CurrentPath) + " fetch -q " + Shell(repository)
	if branch := strings.TrimSpace(sc.Opts.Branch); branch != "" {
		fetch += " " + Shell(branch)
	}
	if _, err := sc.Runner.Run(ctx, fetch, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("fetch %s into the docroot: %w", repository, err)
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
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
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

	mode := settingsString(sc.Opts.Settings, "writable_mode")
	if mode == "" {
		mode = writableModeChmod
	}
	if mode == writableModeSkip {
		return nil
	}
	// The modes mirror what another deploy tool calls them, so a project
	// migrating over keeps the semantics it already relies on. The deploying SSH
	// user is frequently not the web server's runtime user, which is why the
	// ownership half exists at all, and why `acl` exists: it is the only mode that
	// also covers the files the web server creates later.
	doChmod := mode == writableModeChmod || mode == writableModeChmodChown
	doChown := mode == writableModeChown || mode == writableModeChmodChown
	doACL := mode == writableModeACL
	if !doChmod && !doChown && !doACL {
		return fmt.Errorf("unsupported writable_mode %q; use %s, %s, %s, %s or %s",
			mode, writableModeChmod, writableModeChown, writableModeChmodChown, writableModeACL, writableModeSkip)
	}
	owner := settingsString(sc.Opts.Settings, "owner")
	if (doChown || doACL) && owner == "" {
		// Guessing an owner would silently produce a release the web server
		// cannot read, which is worse than refusing. An ACL without a subject is
		// not even a command.
		return fmt.Errorf("writable_mode %q needs deploy.settings.owner (user:group) to be set", mode)
	}
	modeBits := settingsString(sc.Opts.Settings, "writable_permissions")
	if modeBits == "" {
		modeBits = defaultWritablePermissions
	}
	aclEntries := ""
	if doACL {
		aclEntries = aclEntriesFor(owner)
	}

	paths := settingsStringList(sc.Opts.Settings, "writable_dirs")
	if len(paths) == 0 {
		paths = []string{"."}
	}
	for _, entry := range paths {
		target := releasePath + "/" + entry
		// A fresh checkout legitimately lacks generated/, pub/static and var/,
		// so the writable step creates them before applying the mode.
		command := "mkdir -p " + Shell(target)
		if doChmod {
			command += " && chmod -R " + Shell(modeBits) + " " + Shell(target)
		}
		if doChown {
			command += " && chown -R " + conventions.ShellQuote(owner) + " " + Shell(target)
		}
		if doACL {
			// Two passes, as the reference deploy tool does: the access ACL covers
			// what exists now, the default ACL covers what is created under it
			// later — which is the whole reason a project chooses this mode.
			command += " && setfacl -R -L -m " + aclEntries + " " + Shell(target) +
				" && setfacl -R -d -m " + aclEntries + " " + Shell(target)
		}
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
			return fmt.Errorf("apply writable mode to %s: %w", entry, err)
		}
	}
	return nil
}

// The writable modes a project may choose. They are settings, not task ids:
// every framework's release directory has the same permission problem.
const (
	writableModeChmod      = "chmod"
	writableModeChown      = "chown"
	writableModeChmodChown = "chmod+chown"
	writableModeACL        = "acl"
	writableModeSkip       = "skip"

	defaultWritablePermissions = "0775"
)

// aclEntriesFor renders the `setfacl -m` arguments for one `owner` setting.
// `owner` is `user` or `user:group`, and the group half becomes a group entry, so
// a project that already names both keeps both.
func aclEntriesFor(owner string) string {
	user := strings.TrimSpace(owner)
	group := ""
	if before, after, found := strings.Cut(owner, ":"); found {
		user = strings.TrimSpace(before)
		group = strings.TrimSpace(after)
	}

	entries := make([]string, 0, 2)
	if user != "" {
		entries = append(entries, conventions.ShellQuote("u:"+user+":rwX"))
	}
	if group != "" {
		entries = append(entries, conventions.ShellQuote("g:"+group+":rwX"))
	}
	return strings.Join(entries, " -m ")
}

// checkWritableMode refuses a mode the target cannot perform. `acl` needs
// setfacl, and discovering that at the writable step means discovering it after
// the release directory exists; the preflight owns the message.
func checkWritableMode(ctx context.Context, sc *StepContext) error {
	// An unknown mode is a configuration error and is refused before a plan exists
	// (see ValidateSettings); what is left for the target to answer is whether it can
	// perform the mode it was given.
	if settingsString(sc.Opts.Settings, "writable_mode") != writableModeACL {
		return nil
	}
	if _, err := sc.Runner.Run(ctx, "command -v setfacl >/dev/null 2>&1", RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("writable_mode %q needs setfacl on the target; install the acl package or use %s",
			writableModeACL, writableModeChmod)
	}
	return nil
}

// CheckWritableModeForTest exposes the writable-mode preflight to the tests/
// package.
func CheckWritableModeForTest(ctx context.Context, sc *StepContext) error {
	return checkWritableMode(ctx, sc)
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

// noteComposerCredentials records where a target-side build will get its Composer
// credentials, and warns when it will need them and none are available.
//
// Spec 10.4 asks `deploy check` to "verify that Composer can authenticate before
// the deploy starts". What is checkable without running Composer *on the target*
// — the dependency artifact mode exists to remove — is whether credentials are
// available at all, which is the failure that actually happens: an environment
// whose private repository is unreachable fails at build:vendors, half a pipeline
// in. A warning rather than a refusal, because the declaration govard can read is
// a URL and it cannot tell a private repository from a public one; refusing a
// working deploy over a URL would be worse than saying so.
// noteInPlaceSyncPaths warns when an in-place activation has nothing to copy.
//
// The activation resets the docroot to the exact revision and then copies the
// configured `sync_paths` from the built release. `git reset --hard` leaves the
// gitignored directories of the *previous* deployment alone, so an empty
// `sync_paths` publishes new code over the old `vendor/`, `generated/` and
// `pub/static/` — a mixed tree that still reports a successful deploy. Naming it
// before anything runs is the difference between reading a warning and debugging a
// storefront.
func noteInPlaceSyncPaths(ctx context.Context, sc *StepContext) error {
	strategy, err := ResolvePublishStrategy(sc.Host, sc.Opts)
	if err != nil {
		// The strategy is resolved and refused elsewhere; a note is not the step
		// that should fail.
		return nil
	}
	if strategy != PublishInPlace {
		return nil
	}
	paths := settingsStringList(sc.Opts.Settings, "sync_paths")
	if len(paths) == 0 {
		sc.Notes = append(sc.Notes, "warning: this target publishes in place and deploy.settings.sync_paths is empty: "+
			"the activation resets the docroot to the revision and copies nothing, so paths the release built "+
			"(vendor/, generated/, pub/static/) keep whatever the previous deployment left there")
		return nil
	}

	// A path the release links from `shared/` cannot travel into the docroot: the
	// link is relative to the release. Say which entries are affected before the
	// deploy runs, rather than only while it is copying.
	shared := settingsStringList(sc.Opts.Settings, "shared_files", "shared_dirs")
	for _, entry := range paths {
		if covering, isShared := sharedCovering(entry, shared); isShared {
			sc.Notes = append(sc.Notes, "warning: deploy.settings.sync_paths lists "+entry+
				", which the release links from shared/ ("+covering+"): it is not copied into the docroot, "+
				"where a relative link would resolve elsewhere")
		}
		for _, inside := range sharedInside(entry, shared) {
			sc.Notes = append(sc.Notes, "warning: deploy.settings.sync_paths lists "+entry+", which contains the shared path "+
				entry+"/"+inside+": that path is excluded from the copy so the docroot keeps its own")
		}
	}
	return nil
}

// NoteInPlaceSyncPathsForTest exposes the in-place warning to the tests/ package.
func NoteInPlaceSyncPathsForTest(ctx context.Context, sc *StepContext) error {
	return noteInPlaceSyncPaths(ctx, sc)
}

func noteComposerCredentials(ctx context.Context, sc *StepContext) error {
	if sc.Opts.Build == BuildArtifact {
		// Dependencies were installed where the artifact was built.
		return nil
	}
	repositories, err := privateComposerRepositories(sc.WorkDir)
	if err != nil {
		return err
	}
	if len(repositories) == 0 {
		return nil
	}

	if strings.TrimSpace(os.Getenv(ComposerAuthEnv)) != "" {
		sc.Notes = append(sc.Notes, "composer credentials: "+ComposerAuthEnv+" is set for this run")
		return nil
	}
	if _, err := sc.Runner.Run(ctx, "test -f "+Shell(path.Join(sc.Host.SharedPath(), "auth.json")), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err == nil {
		sc.Notes = append(sc.Notes, "composer credentials: shared/auth.json exists on the target")
		return nil
	}

	sc.Notes = append(sc.Notes, fmt.Sprintf(
		"warning: this project declares private composer repositories (%s) and the target builds the release, but no credentials are available: set %s in the environment, or provide %s on the target",
		strings.Join(repositories, ", "), ComposerAuthEnv, path.Join(sc.Host.SharedPath(), "auth.json")))
	return nil
}

// NoteComposerCredentialsForTest exposes noteComposerCredentials to the tests/
// package, which cannot reach the artifact-mode branch through CoreCheck without
// a real manifest and a PHP on the target.
func NoteComposerCredentialsForTest(ctx context.Context, sc *StepContext) error {
	return noteComposerCredentials(ctx, sc)
}

// privateComposerRepositories reads the `repositories` URLs a checkout declares
// that are not packagist, which is the only statically visible sign that an
// install will need credentials.
func privateComposerRepositories(workDir string) ([]string, error) {
	if strings.TrimSpace(workDir) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(filepath.Join(workDir, "composer.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read composer.json: %w", err)
	}
	var manifest struct {
		Repositories []struct {
			URL string `json:"url"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		// A manifest this shape cannot read is the recipe's business, not the
		// preflight's: a malformed composer.json fails the build with a better
		// message than this step could give.
		return nil, nil
	}

	repositories := make([]string, 0, len(manifest.Repositories))
	for _, repository := range manifest.Repositories {
		url := strings.TrimSpace(repository.URL)
		if url == "" || strings.Contains(url, "packagist.org") || strings.Contains(url, "packagist.com") {
			continue
		}
		repositories = append(repositories, url)
	}
	return repositories, nil
}
