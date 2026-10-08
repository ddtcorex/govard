package tests

import (
	"context"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

func cloneRow(t *testing.T) verify.Item {
	t.Helper()
	for _, it := range verify.RegistryFor(engine.Config{Framework: "magento2"}) {
		if it.ID == "P2-08" {
			return it
		}
	}
	t.Fatal("P2-08 is missing from the registry")
	return verify.Item{}
}

func runCloneRow(t *testing.T, releasesOut string, releasesExit int) (verify.Evidence, [][]string) {
	t.Helper()
	var calls [][]string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		calls = append(calls, append([]string(nil), args...))
		if len(args) > 1 && args[0] == "deploy" && args[1] == "releases" {
			return verify.Evidence{ExitCode: releasesExit, OutputExcerpt: releasesOut}, true
		}
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "bootstrapped"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	ev := cloneRow(t).Run(context.Background(), engine.Config{Framework: "magento2"}, verify.VerifyOpts{Remote: "sandbox", ProjectRoot: t.TempDir()})
	return ev, calls
}

func TestCloneBootstrapSkipsWhenRemoteHasNoRelease(t *testing.T) {
	ev, calls := runCloneRow(t, "sandbox has no releases under /srv/app/releases", 0)
	if !ev.Skipped || !strings.Contains(ev.SkipReason, "no release") {
		t.Fatalf("want a skip naming the missing release, got %+v", ev)
	}
	for _, c := range calls {
		if c[0] == "bootstrap" {
			t.Fatalf("bootstrap must not run without a release: %v", calls)
		}
	}
}

func TestCloneBootstrapRunsWhenRemoteHasRelease(t *testing.T) {
	ev, calls := runCloneRow(t, `[{"release":"20260101T000000Z","live":true}]`, 0)
	if ev.Skipped {
		t.Fatalf("must not skip when a release exists: %+v", ev)
	}
	last := calls[len(calls)-1]
	if last[0] != "bootstrap" {
		t.Fatalf("bootstrap must run after the probe: %v", calls)
	}
}

func TestCloneBootstrapRunsWhenProbeFails(t *testing.T) {
	ev, calls := runCloneRow(t, "ssh: connection refused", 1)
	if ev.Skipped || calls[len(calls)-1][0] != "bootstrap" {
		t.Fatalf("an unreachable probe must not hide the real failure: %+v %v", ev, calls)
	}
}
