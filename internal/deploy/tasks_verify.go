package deploy

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"govard/internal/engine"
)

// VerifyURL performs the post-publish HTTP check from the machine running
// govard, using net/http rather than curl on the target: a missing curl must not
// be able to break a deploy, and the operator's vantage point is the useful one.
func VerifyURL(ctx context.Context, url string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultVerifyTimeout
	}
	client := &http.Client{Timeout: timeout}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", url, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return fmt.Errorf("%s returned HTTP %d", url, response.StatusCode)
	}
	return nil
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
	fail := func(id, detail string) error {
		checks = append(checks, CheckResult{ID: id, Status: "failed", Detail: detail})
		sc.Release.Verify = VerifyRecord{Status: "failed", Checks: checks}
		return fmt.Errorf("verify %s: %s", id, detail)
	}
	pass := func(id, detail string) {
		checks = append(checks, CheckResult{ID: id, Status: "ok", Detail: detail})
	}

	switch sc.Release.Publish.Strategy {
	case PublishSymlink:
		result, err := sc.Runner.Run(ctx, "readlink -f "+Shell(sc.Host.CurrentPath), RunOptions{Timeout: shortCommandTimeout})
		if err != nil {
			return fail("revision", "current symlink is unreadable: "+err.Error())
		}
		if strings.TrimSpace(result.Stdout) != sc.Release.Path {
			return fail("revision", fmt.Sprintf("current resolves to %q, want %q", strings.TrimSpace(result.Stdout), sc.Release.Path))
		}
		pass("revision", "current resolves to the release")
	case PublishInPlace:
		result, err := sc.Runner.Run(ctx, "git -C "+Shell(sc.Host.CurrentPath)+" rev-parse HEAD", RunOptions{Timeout: shortCommandTimeout})
		if err != nil {
			return fail("revision", "docroot HEAD is unreadable: "+err.Error())
		}
		if strings.TrimSpace(result.Stdout) != sc.Release.Revision {
			return fail("revision", fmt.Sprintf("docroot HEAD is %q, want %q", strings.TrimSpace(result.Stdout), sc.Release.Revision))
		}
		pass("revision", "docroot HEAD matches the deployed revision")
	default:
		// No default would mean "the record does not say how the release went
		// live, so check nothing and pass" — the one answer the verify stage
		// must never give. A record can only lose this field by being
		// reconstructed from part of itself, which is a bug worth failing on.
		return fail("revision", fmt.Sprintf("the release record names publish strategy %q, so the live revision cannot be checked", sc.Release.Publish.Strategy))
	}

	for _, entry := range settingsStringList(sc.Opts.Settings, "shared_files") {
		// The requirement is conditional on the shared entry existing, because
		// that is what `deploy:shared` does: the first deploy of a project
		// legitimately has no shared state yet, and treating that as a broken
		// link failed the deploy after it had already activated. A shared file
		// that *does* exist must be readable in the release, which is what
		// catches the broken symlink this check is here for.
		command := fmt.Sprintf("if [ -e %s ]; then test -r %s; fi",
			Shell(path.Join(sc.Host.SharedPath(), entry)), Shell(path.Join(sc.Release.Path, entry)))
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fail("shared:"+entry, "the shared file exists on the target but is missing or unreadable in the release")
		}
		pass("shared:"+entry, "linked and readable")
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
			return fail(check.ID, "expand the check command: "+err.Error())
		}
		timeout := sc.Opts.CommandTimeout
		if timeout <= 0 {
			timeout = DefaultCommandTimeout
		}
		if _, err := sc.Runner.Run(ctx, command, RunOptions{Timeout: timeout}); err != nil {
			return fail(check.ID, check.Title+": "+err.Error())
		}
		pass(check.ID, check.Title)
	}

	if url := strings.TrimSpace(sc.Opts.VerifyURL); url != "" {
		if err := VerifyURL(ctx, url, sc.Opts.VerifyTimeout); err != nil {
			return fail("http", err.Error())
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
		if _, err := sc.Runner.Run(ctx, "rm -rf "+Shell(host.ReleasePath(name)), RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
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
		if _, err := sc.Runner.Run(ctx, "rm -rf "+Shell(path.Join(root, name)), RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
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
	result, err := sc.Runner.Run(ctx, "ls -1 "+Shell(directory)+" 2>/dev/null || true", RunOptions{Timeout: shortCommandTimeout})
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
