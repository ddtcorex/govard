package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"govard/internal/engine"
)

// ErrVerifyRedirect marks the one answer a preflight can refuse on its own: a
// redirect with following turned off is the same answer every time, so it names a
// configuration fault rather than an outage. It is separated from every other
// verify failure so the prepare stage can fail on it — discovering it after
// activation leaves a live site with a failed deploy and a held lock — while a
// timeout or a 5xx stays the warning a deploy that repairs the site must not be
// blocked by.
var ErrVerifyRedirect = errors.New("the verify URL redirects")

// VerifyPolicy is how the HTTP check treats a redirect, and which landing paths a
// recipe declares never-healthy.
type VerifyPolicy struct {
	// FollowRedirects follows redirects that stay on the verify URL's host. The
	// default is off: a verify URL should name the page that serves the site, and
	// a redirect followed to a 200 is how an install page or a store-code bounce
	// passes verification while the site is not serving the release.
	FollowRedirects bool
	// RejectPaths are path prefixes a recipe says never mean "the site is
	// serving" — a framework's installer landing page, for instance. They are
	// checked on the final URL, so they still apply when redirects are followed.
	RejectPaths []string
}

// VerifyURL performs the post-publish HTTP check from the machine running
// govard, using net/http rather than curl on the target: a missing curl must not
// be able to break a deploy, and the operator's vantage point is the useful one.
func VerifyURL(ctx context.Context, url string, timeout time.Duration, policy VerifyPolicy) error {
	if timeout <= 0 {
		timeout = DefaultVerifyTimeout
	}
	client := &http.Client{Timeout: timeout}
	if !policy.FollowRedirects {
		// Stop at the first response, whatever it is: the status is the answer.
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	} else {
		parsed, err := neturl.Parse(url)
		if err != nil {
			return fmt.Errorf("parse %s: %w", url, err)
		}
		client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
			if request.URL.Host != parsed.Host {
				return fmt.Errorf("refusing to follow a redirect off %s to %s", parsed.Host, request.URL.String())
			}
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", url, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 && response.StatusCode < 400 {
		location := response.Header.Get("Location")
		if location != "" {
			return fmt.Errorf("%w: %s answered HTTP %d to %s: the verify URL must name the page that serves the site, or set deploy.verify.follow_redirects: true to follow same-host redirects",
				ErrVerifyRedirect, url, response.StatusCode, location)
		}
		return fmt.Errorf("%w: %s answered HTTP %d: the verify URL must name the page that serves the site, or set deploy.verify.follow_redirects: true to follow same-host redirects",
			ErrVerifyRedirect, url, response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s returned HTTP %d", url, response.StatusCode)
	}
	for _, reject := range policy.RejectPaths {
		if reject == "" || response.Request == nil || response.Request.URL == nil {
			continue
		}
		if strings.HasPrefix(response.Request.URL.Path, reject) {
			return fmt.Errorf("%s landed on %s, which never means the site is serving the release", url, response.Request.URL.Path)
		}
	}
	return nil
}

// verifyInPlaceSync compares each sync path in the served docroot with the
// release it was copied from, using the same `--delete` and the same shared-path
// excludes the activation used, in dry-run mode. It walks the whole tree instead
// of comparing a marker: a hook or a process that rewrote a file after the copy
// is caught, and the check names the path and the first differences.
//
// A path the release did not build, or one the release links from `shared/`, is
// skipped for the same reason the activation skipped it: the docroot keeps its
// own copy and there is nothing to compare.
func verifyInPlaceSync(ctx context.Context, sc *StepContext, pass func(string, string), fail func(string, string, error) error) error {
	shared := settingsStringList(sc.Opts.Settings, "shared_files", "shared_dirs")
	for _, entry := range settingsStringList(sc.Opts.Settings, "sync_paths") {
		source := path.Join(sc.Release.Path, entry)
		if _, err := sc.Runner.Run(ctx, "test -e "+Shell(source), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
			continue
		}
		if _, isShared := sharedCovering(entry, shared); isShared {
			continue
		}
		command := "rsync -a --delete --dry-run --itemize-changes --checksum"
		for _, inside := range sharedInside(entry, shared) {
			command += " --exclude=" + Shell("/"+inside)
		}
		command += " " + Shell(source+"/") + " " + Shell(path.Join(sc.Host.CurrentPath, entry)+"/")
		result, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live})
		if err != nil {
			return fail("sync:"+entry, "the docroot cannot be compared with the release", err)
		}
		// `--checksum` is what makes this a check of the *content*: without it
		// rsync calls any mtime difference a transfer, and a hook that merely
		// touched a served file would fail the deploy. Attribute-only lines start
		// with `.` and are not differences at all.
		//
		// What the release has and the docroot does not — a transfer (`>`) or a
		// creation (`c`) — is a real difference: the activation was supposed to
		// copy it, and its absence means the served tree is not the release.
		//
		// `*deleting` is the other direction, and it is not a failure. It is a
		// path the docroot has and the release does not, which the activation's own
		// `--delete` removes; the application writes those while it runs — a
		// generated class, a compiled template, a log — and the steps that serve
		// the site run in the docroot before this check does. Failing on one would
		// fail a release that is healthy and already live, holding the lock. It is
		// reported instead, so an operator still sees what the next activation
		// would clean up.
		//
		// The cost is reading both trees: a large release makes this the slowest
		// part of verification, which the PR measures on a real target.
		var differences, deletions []string
		for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, ".") {
				continue
			}
			if strings.Contains(line, "*deleting") {
				deletions = append(deletions, line)
				continue
			}
			differences = append(differences, line)
		}
		if len(differences) == 0 {
			if len(deletions) > 0 {
				pass("sync:"+entry, fmt.Sprintf("the docroot matches the release; it also holds %d path(s) the release does not, which the next activation removes:\n%s",
					len(deletions), strings.Join(cappedLines(deletions), "\n")))
				continue
			}
			pass("sync:"+entry, "the docroot matches the release")
			continue
		}
		return fail("sync:"+entry, fmt.Sprintf("the docroot's %s differs from the release:\n%s", entry, strings.Join(cappedLines(differences), "\n")), nil)
	}
	return nil
}

// cappedLines keeps a report to the first few lines: enough to name what is
// wrong, short enough to read in a terminal and to store in a release record.
func cappedLines(lines []string) []string {
	if len(lines) > 5 {
		return append(lines[:5:5], "...")
	}
	return lines
}

// CoreVerify proves the live target serves what was just deployed. It is
// skipped entirely with --no-verify.
func CoreVerify(ctx context.Context, sc *StepContext) error {
	if !sc.Opts.Verify {
		return nil
	}
	if sc.Release == nil {
		return fmt.Errorf("no release record to verify")
	}

	checks := make([]CheckResult, 0, 4)
	// fail takes the error it is reporting, rather than a sentence built from it.
	// The verification is a step like any other, so a transport that died while it
	// read the target has to keep the runner's sentinel: stringifying the error
	// here throws away the one fact the recovery hint is built on, and the operator
	// gets the generic "the lock was kept" text for a connection that may have
	// dropped mid-step.
	fail := func(id, detail string, cause error) error {
		checks = append(checks, CheckResult{ID: id, Status: "failed", Detail: detail})
		sc.Release.Verify = VerifyRecord{Status: "failed", Checks: checks}
		if cause == nil {
			return fmt.Errorf("verify %s: %s", id, detail)
		}
		return fmt.Errorf("verify %s: %s: %w", id, detail, cause)
	}
	pass := func(id, detail string) {
		checks = append(checks, CheckResult{ID: id, Status: "ok", Detail: detail})
	}

	switch sc.Release.Publish.Strategy {
	case PublishSymlink:
		// Both sides are canonicalised on the target: a deploy path that reaches
		// the release through a symlinked parent (`~` is a built-in discovered
		// deploy path) makes `readlink -f current` and the recorded path two
		// spellings of one directory, and comparing them as strings failed every
		// deploy after the site had already switched.
		command := fmt.Sprintf("printf '%%s\n%%s\n' \"$(readlink -f %s)\" \"$(readlink -f %s)\"",
			Shell(sc.Host.CurrentPath), Shell(sc.Release.Path))
		result, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Out: sc.Live})
		if err != nil {
			return fail("revision", "current symlink is unreadable", err)
		}
		lines := strings.Split(strings.TrimRight(result.Stdout, "\n"), "\n")
		if len(lines) != 2 || strings.TrimSpace(lines[1]) == "" {
			return fail("revision", "the release path cannot be resolved on the target: "+strings.TrimSpace(result.Stdout), nil)
		}
		if strings.TrimSpace(lines[0]) != strings.TrimSpace(lines[1]) {
			return fail("revision", fmt.Sprintf("current resolves to %q, want %q", strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])), nil)
		}
		pass("revision", "current resolves to the release")
	case PublishInPlace:
		result, err := sc.Runner.Run(ctx, "git -C "+Shell(sc.Host.CurrentPath)+" rev-parse HEAD", RunOptions{Timeout: shortCommandTimeout, Out: sc.Live})
		if err != nil {
			return fail("revision", "docroot HEAD is unreadable", err)
		}
		if strings.TrimSpace(result.Stdout) != sc.Release.Revision {
			return fail("revision", fmt.Sprintf("docroot HEAD is %q, want %q", strings.TrimSpace(result.Stdout), sc.Release.Revision), nil)
		}
		pass("revision", "docroot HEAD matches the deployed revision")
		// The activation copies the built paths into the docroot; a dry run of
		// that same copy is what proves the site is still serving them. The one
		// comparison this replaces compared a version file with the copy the
		// activation had just made from it, which could only fail if the copy
		// failed — a tautology, and blind to everything the release actually
		// contains.
		if err := verifyInPlaceSync(ctx, sc, pass, fail); err != nil {
			return err
		}
	default:
		// No default would mean "the record does not say how the release went
		// live, so check nothing and pass" — the one answer the verify stage
		// must never give. A record can only lose this field by being
		// reconstructed from part of itself, which is a bug worth failing on.
		return fail("revision", fmt.Sprintf("the release record names publish strategy %q, so the live revision cannot be checked", sc.Release.Publish.Strategy), nil)
	}

	for _, entry := range settingsStringList(sc.Opts.Settings, "shared_files") {
		// The requirement is conditional on the shared entry existing, because
		// that is what `deploy:shared` does: the first deploy of a project
		// legitimately has no shared state yet, and treating that as a broken
		// link failed the deploy after it had already activated. A shared file
		// that *does* exist must be readable where the site reads it, which is the
		// served path and not the release directory: in place the two are different
		// trees, and a shared file linked into the release while missing from the
		// docroot is exactly the broken state this check exists for. For a symlink
		// the swap has already happened, so `current` resolves to the release.
		command := fmt.Sprintf("if [ -e %s ]; then test -r %s; fi",
			Shell(path.Join(sc.Host.SharedPath(), entry)), Shell(path.Join(sc.Host.CurrentPath, entry)))
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
			return fail("shared:"+entry, "the shared file exists on the target but is missing or unreadable where the site reads it", nil)
		}
		pass("shared:"+entry, "linked and readable")
	}

	for _, entry := range settingsStringList(sc.Opts.Settings, "shared_dirs") {
		// A shared *directory* is checked in the release, not in the served path,
		// and that is the one place where the two differ from the rule above. An
		// in-place docroot that owns its own copy of a directory keeps it —
		// `ensureInPlaceShared` refuses to delete the operator's data — so a real
		// directory there is a correct target, and demanding a link would fail one.
		// Everywhere else the release and the served tree are the same directory
		// (a symlink target) or the release is what the activation copies from.
		//
		// The condition mirrors `deploy:shared` exactly: a shared directory that
		// does not exist yet is not a promise, so nothing is required of the
		// release. What is required, once the target holds the directory, is that
		// the release reads it through a link — which is the assertion that catches
		// a link that silently landed somewhere else.
		//
		// Both halves are needed. `test -L` alone accepts a *dangling* link, and a
		// release whose shared directory resolves to nothing reads an empty tree:
		// `-e` follows the link and rejects exactly that. `-e` alone is the failure
		// this check was added for — the release's own real directory passes it —
		// which is why neither predicate is enough on its own.
		command := fmt.Sprintf("if [ -e %s ]; then test -L %s && test -e %s; fi",
			Shell(path.Join(sc.Host.SharedPath(), entry)),
			Shell(path.Join(sc.Release.Path, entry)), Shell(path.Join(sc.Release.Path, entry)))
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
			return fail("shared:"+entry, "the shared directory exists on the target but the release does not read it through a link", nil)
		}
		pass("shared:"+entry, "linked from shared/")
	}

	for _, check := range sc.Checks {
		if strings.TrimSpace(check.Command) == "" {
			continue
		}
		if check.OnlyForPublishStrategy != "" && check.OnlyForPublishStrategy != sc.Release.Publish.Strategy {
			continue
		}
		command, err := sc.Vars.Expand(check.Command)
		if err != nil {
			return fail(check.ID, "expand the check command", err)
		}
		timeout := sc.Opts.CommandTimeout
		if timeout <= 0 {
			timeout = DefaultCommandTimeout
		}
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: timeout, Out: sc.Live}); err != nil {
			return fail(check.ID, check.Title, err)
		}
		pass(check.ID, check.Title)
	}

	if url := strings.TrimSpace(sc.Opts.VerifyURL); url != "" {
		if err := VerifyURL(ctx, url, sc.Opts.VerifyTimeout, VerifyPolicy{
			FollowRedirects: sc.Opts.VerifyFollowRedirects,
			RejectPaths:     sc.Opts.VerifyRejectPaths,
		}); err != nil {
			return fail("http", "the verify URL did not answer 2xx", err)
		}
		pass("http", url+" is serving")
	}

	sc.Release.Verify = VerifyRecord{Status: "ok", Checks: checks}
	return nil
}

// CoreCleanup prunes old releases and the database dumps that belong to them.
//
// It only ever removes a release directory that carries a govard release record.
// A directory created by another deploy tool has no such record and is left
// alone, which is what makes running next to another tool during a migration safe.
func CoreCleanup(ctx context.Context, sc *StepContext) error {
	host := sc.Host
	keep := sc.Opts.KeepReleases
	if keep <= 0 {
		keep = engine.DefaultKeepReleases
	}
	// The live release is skipped below, so `keep` never has to account for it.

	stale, err := numberedEntries(ctx, sc, host.ReleasesPath())
	if err != nil {
		return fmt.Errorf("list releases: %w", err)
	}

	foreign := make([]string, 0, 4)
	candidates := make([]string, 0, len(stale))
	for _, name := range stale {
		if sc.Release != nil && name == sc.Release.Release {
			continue
		}
		record, err := ReadRelease(ctx, host, name)
		if err != nil || record.Tool != ReleaseTool {
			foreign = append(foreign, name)
			continue
		}
		candidates = append(candidates, name)
	}

	for _, name := range pruneWindow(candidates, keep) {
		if _, err := sc.Runner.Run(ctx, "rm -rf "+Shell(host.ReleasePath(name)), RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
			return fmt.Errorf("prune release %s: %w", name, err)
		}
	}
	if err := pruneBackups(ctx, sc, keep); err != nil {
		return err
	}
	reportForeignReleases(sc, foreign)
	return nil
}

// pruneBackups keeps the newest `keep` dump directories under
// shared/backups/deploy/.
//
// The window is the releases' own: a dump is the way back from the migration a
// specific release ran, so keeping more dumps than releases keeps nothing useful.
// There is no record to consult here — the directory name is the release number a
// backup belongs to — so a name govard did not write is left out of the window
// entirely, exactly as it is for releases. Counting it would push a real dump out
// of the window and then keep the stranger, which is the one outcome a cleanup
// must not produce: the dump is the way back from a migration.
func pruneBackups(ctx context.Context, sc *StepContext, keep int) error {
	root := sc.Host.BackupRootPath()
	entries, err := numberedEntries(ctx, sc, root)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}

	candidates := make([]string, 0, len(entries))
	foreign := make([]string, 0, 2)
	for _, name := range entries {
		if _, err := strconv.Atoi(name); err != nil {
			foreign = append(foreign, name)
			continue
		}
		candidates = append(candidates, name)
	}

	for _, name := range pruneWindow(candidates, keep) {
		if _, err := sc.Runner.Run(ctx, "rm -rf "+Shell(path.Join(root, name)), RunOptions{Timeout: sc.Opts.CommandTimeout, Out: sc.Live}); err != nil {
			return fmt.Errorf("prune backup %s: %w", name, err)
		}
	}
	reportForeignBackups(sc, foreign)
	return nil
}

// reportForeignBackups says which backup directories were left alone, so "cleanup
// kept something" is never a mystery in the deploy log.
func reportForeignBackups(sc *StepContext, foreign []string) {
	if len(foreign) == 0 || sc.Out == nil {
		return
	}
	noun := "directories"
	if len(foreign) == 1 {
		noun = "directory"
	}
	fmt.Fprintf(sc.Out, "  kept %d backup %s govard did not name: %s\n", len(foreign), noun, strings.Join(foreign, ", "))
}

// numberedEntries lists a directory whose children are release numbers. A
// missing directory is an empty one: a target with no releases yet, or a project
// that never ran --db-backup, is not an error.
func numberedEntries(ctx context.Context, sc *StepContext, directory string) ([]string, error) {
	result, err := sc.Runner.Run(ctx, "ls -1 "+Shell(directory)+" 2>/dev/null || true", RunOptions{Timeout: shortCommandTimeout, Out: sc.Live})
	if err != nil {
		return nil, err
	}
	entries := make([]string, 0, 8)
	for _, line := range strings.Split(result.Stdout, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			entries = append(entries, name)
		}
	}
	return entries, nil
}

// pruneWindow returns the entries outside the newest `keep`, newest kept. A name
// that is not a number sorts as the *newest*, so it is kept: both callers filter
// those names out before they get here, and the sentinel is the safe answer for a
// caller that forgets — a directory govard did not name is never deleted, and
// never counts against the window.
func pruneWindow(entries []string, keep int) []string {
	sorted := make([]string, len(entries))
	copy(sorted, entries)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && entryNumber(sorted[j]) > entryNumber(sorted[j-1]); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	if len(sorted) <= keep {
		return nil
	}
	return sorted[keep:]
}

// entryNumber is the release number a directory name encodes. An unparsable name
// is treated as the largest, so a caller that does not filter it keeps it rather
// than deletes it — the safe direction for a name govard did not write.
func entryNumber(name string) int {
	number, err := strconv.Atoi(name)
	if err != nil {
		return 1 << 30
	}
	return number
}

// reportForeignReleases tells the operator which directories were left alone,
// so "cleanup kept something" is never a mystery.
func reportForeignReleases(sc *StepContext, foreign []string) {
	if len(foreign) == 0 || sc.Out == nil {
		return
	}
	fmt.Fprintf(sc.Out, "  kept %d release(s) without a govard record: %s\n", len(foreign), strings.Join(foreign, ", "))
}
