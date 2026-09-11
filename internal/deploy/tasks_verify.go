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
		if _, err := sc.Runner.Run(ctx, "test -r "+Shell(path.Join(sc.Release.Path, entry)), RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fail("shared:"+entry, "shared file is missing or unreadable in the release")
		}
		pass("shared:"+entry, "linked and readable")
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

// CoreCleanup prunes old releases.
//
// It only ever removes a directory that carries a govard release record. A
// directory created by another deploy tool has no such record and is left alone,
// which is what makes running next to another tool during a migration safe.
func CoreCleanup(ctx context.Context, sc *StepContext) error {
	host := sc.Host
	keep := sc.Opts.KeepReleases
	if keep <= 0 {
		keep = engine.DefaultKeepReleases
	}
	// The live release is skipped below, so `keep` never has to account for it.

	result, err := sc.Runner.Run(ctx, "ls -1 "+Shell(host.ReleasesPath())+" 2>/dev/null || true", RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return fmt.Errorf("list releases: %w", err)
	}

	type candidate struct {
		name   string
		number int
	}
	candidates := make([]candidate, 0, 8)
	foreign := make([]string, 0, 4)
	for _, line := range strings.Split(result.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if sc.Release != nil && name == sc.Release.Release {
			continue
		}
		record, err := ReadRelease(ctx, host, name)
		if err != nil || record.Tool != ReleaseTool {
			foreign = append(foreign, name)
			continue
		}
		number, err := strconv.Atoi(name)
		if err != nil {
			number = 1 << 30
		}
		candidates = append(candidates, candidate{name: name, number: number})
	}

	// Newest first, then keep the requested number.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].number > candidates[j-1].number; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	if len(candidates) <= keep {
		reportForeignReleases(sc, foreign)
		return nil
	}
	for _, stale := range candidates[keep:] {
		if _, err := sc.Runner.Run(ctx, "rm -rf "+Shell(host.ReleasePath(stale.name)), RunOptions{Timeout: sc.Opts.CommandTimeout}); err != nil {
			return fmt.Errorf("prune release %s: %w", stale.name, err)
		}
	}
	reportForeignReleases(sc, foreign)
	return nil
}

// reportForeignReleases tells the operator which directories were left alone,
// so "cleanup kept something" is never a mystery.
func reportForeignReleases(sc *StepContext, foreign []string) {
	if len(foreign) == 0 || sc.Out == nil {
		return
	}
	fmt.Fprintf(sc.Out, "  kept %d release(s) without a govard record: %s\n", len(foreign), strings.Join(foreign, ", "))
}
