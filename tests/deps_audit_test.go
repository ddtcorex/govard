package tests

import (
	"errors"
	"strings"
	"testing"

	"govard/internal/depsaudit"
)

func TestParseModulesReadsTheGoListStream(t *testing.T) {
	stream := `{"Path":"github.com/gobwas/glob","Version":"v0.2.3","Update":{"Path":"github.com/gobwas/glob","Version":"v1.1.0"},"Indirect":true}
{"Path":"github.com/spf13/cobra","Version":"v1.10.2"}
`
	mods, err := depsaudit.ParseModules(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mods) != 2 || mods[0].Update != "v1.1.0" || !mods[0].Indirect || mods[1].Update != "" {
		t.Fatalf("unexpected modules: %+v", mods)
	}
}

func TestAnalyzeSplitsHeldFromMovable(t *testing.T) {
	mods := []depsaudit.Module{
		{Path: "github.com/gobwas/glob", Version: "v0.2.3", Update: "v1.1.0"},
		{Path: "golang.org/x/text", Version: "v0.44.0", Update: "v0.45.0"},
		{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
	}
	holds := []depsaudit.Hold{{Path: "github.com/gobwas/glob", Version: "v0.2.3", Reason: "removed subtrees"}}
	report := depsaudit.Analyze(mods, holds)
	if len(report.Held) != 1 || len(report.Movable) != 1 || len(report.StaleHolds) != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Movable[0].Path != "golang.org/x/text" {
		t.Fatalf("movable = %+v, want golang.org/x/text", report.Movable)
	}
}

func TestAnalyzeReportsAHoldThatMoved(t *testing.T) {
	// One outdated module keeps the update data non-empty, so this case is about
	// the pinned module moving off its hold, not about an unverifiable run.
	mods := []depsaudit.Module{
		{Path: "github.com/gobwas/glob", Version: "v1.1.0"},
		{Path: "golang.org/x/text", Version: "v0.44.0", Update: "v0.45.0"},
	}
	holds := []depsaudit.Hold{{Path: "github.com/gobwas/glob", Version: "v0.2.3", Reason: "removed subtrees"}}
	report := depsaudit.Analyze(mods, holds)
	if len(report.StaleHolds) != 1 {
		t.Fatalf("stale holds = %+v, want the moved hold", report.StaleHolds)
	}
}

func TestAnalyzeReportsAHoldThatIsNoLongerOutdated(t *testing.T) {
	// One outdated module keeps the update data non-empty, so this case is about
	// the hold expiring, not about a run that could not reach the proxy.
	mods := []depsaudit.Module{
		{Path: "github.com/gobwas/glob", Version: "v0.2.3"},
		{Path: "golang.org/x/text", Version: "v0.44.0", Update: "v0.45.0"},
	}
	holds := []depsaudit.Hold{{Path: "github.com/gobwas/glob", Version: "v0.2.3", Reason: "removed subtrees"}}
	report := depsaudit.Analyze(mods, holds)
	if len(report.StaleHolds) != 1 {
		t.Fatalf("stale holds = %+v, want the expired hold", report.StaleHolds)
	}
}

// `go list -m -u` succeeds with no Update fields when the module proxy is
// unreachable, which is indistinguishable from "every hold is stale" unless the
// audit says so. A run that cannot be verified must never recommend editing the
// holds, because the two holds are load-bearing.
func TestAnalyzeMarksARunWithoutUpdateDataAsUnverified(t *testing.T) {
	mods := []depsaudit.Module{{Path: "github.com/gobwas/glob", Version: "v0.2.3"}}
	holds := []depsaudit.Hold{{Path: "github.com/gobwas/glob", Version: "v0.2.3", Reason: "removed subtrees", Retry: "when upstream ships"}}
	report := depsaudit.Analyze(mods, holds)
	if !report.Unverified {
		t.Fatalf("report = %+v, want Unverified for a list with no update data", report)
	}
	if len(report.StaleHolds) != 0 {
		t.Fatalf("stale holds = %+v, want none when the run could not be verified", report.StaleHolds)
	}
}

func TestExitCodeSeparatesStaleHoldsFromAnUnverifiedRun(t *testing.T) {
	ok := depsaudit.Analyze([]depsaudit.Module{
		{Path: "github.com/gobwas/glob", Version: "v0.2.3", Update: "v1.1.0"},
	}, []depsaudit.Hold{{Path: "github.com/gobwas/glob", Version: "v0.2.3", Reason: "r", Retry: "t"}})
	if code := depsaudit.ExitCode(ok); code != depsaudit.ExitOK {
		t.Fatalf("ExitCode(held) = %d, want %d", code, depsaudit.ExitOK)
	}

	stale := depsaudit.Analyze([]depsaudit.Module{
		{Path: "github.com/gobwas/glob", Version: "v1.1.0", Update: "v1.2.0"},
	}, []depsaudit.Hold{{Path: "github.com/gobwas/glob", Version: "v0.2.3", Reason: "r", Retry: "t"}})
	if code := depsaudit.ExitCode(stale); code != depsaudit.ExitStaleHolds {
		t.Fatalf("ExitCode(stale) = %d, want %d", code, depsaudit.ExitStaleHolds)
	}

	unverified := depsaudit.Analyze([]depsaudit.Module{
		{Path: "github.com/gobwas/glob", Version: "v0.2.3"},
	}, []depsaudit.Hold{{Path: "github.com/gobwas/glob", Version: "v0.2.3", Reason: "r", Retry: "t"}})
	if code := depsaudit.ExitCode(unverified); code != depsaudit.ExitUnverified {
		t.Fatalf("ExitCode(unverified) = %d, want %d", code, depsaudit.ExitUnverified)
	}
}

func TestShippedHoldsCoverTheTwoKnownModules(t *testing.T) {
	holds, err := depsaudit.Holds()
	if err != nil {
		t.Fatalf("holds: %v", err)
	}
	pinned := map[string]string{}
	for _, h := range holds {
		if h.Reason == "" || h.Retry == "" {
			t.Fatalf("hold %s needs a reason and a retry trigger", h.Path)
		}
		pinned[h.Path] = h.Version
	}
	if pinned["github.com/gobwas/glob"] != "v0.2.3" || pinned["github.com/wailsapp/task/v3"] != "v3.40.1-patched3" {
		t.Fatalf("shipped holds = %+v", pinned)
	}
}

func TestParseHoldsRejectsAHoldWithoutAReasonOrRetry(t *testing.T) {
	for name, data := range map[string]string{
		"missing reason": `[{"Path": "example.com/m", "Version": "v1.0.0", "Retry": "when upstream ships"}]`,
		"missing retry":  `[{"Path": "example.com/m", "Version": "v1.0.0", "Reason": "removed packages"}]`,
		"missing path":   `[{"Version": "v1.0.0", "Reason": "removed packages", "Retry": "when upstream ships"}]`,
	} {
		if _, err := depsaudit.ParseHolds([]byte(data)); err == nil {
			t.Fatalf("%s: ParseHolds accepted a hold with no validation", name)
		}
	}
}

func TestParseHoldsAcceptsTheDeclaredShape(t *testing.T) {
	holds, err := depsaudit.ParseHolds([]byte(`[{"Path": "example.com/m", "Version": "v1.0.0", "Reason": "removed packages", "Retry": "when upstream ships"}]`))
	if err != nil {
		t.Fatalf("parse holds: %v", err)
	}
	if len(holds) != 1 || holds[0].Version != "v1.0.0" {
		t.Fatalf("holds = %+v", holds)
	}
}

func TestClassifyMovesUnimportedModulesToUpstreamOwned(t *testing.T) {
	report := depsaudit.Analyze([]depsaudit.Module{
		{Path: "example.com/unused", Version: "v1.0.0", Update: "v1.1.0"},
		{Path: "example.com/used", Version: "v1.0.0", Update: "v1.1.0"},
	}, nil)
	report, errs := depsaudit.Classify(report, func(path string) (bool, error) {
		return path == "example.com/used", nil
	})
	if len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	if len(report.UpstreamOwned) != 1 || report.UpstreamOwned[0].Path != "example.com/unused" {
		t.Fatalf("upstream owned = %+v", report.UpstreamOwned)
	}
	if len(report.Movable) != 2 {
		t.Fatalf("movable = %+v, want both rows kept for the caller to print", report.Movable)
	}
}

func TestClassifyReportsAModuleItCannotClassify(t *testing.T) {
	report := depsaudit.Analyze([]depsaudit.Module{
		{Path: "example.com/broken", Version: "v1.0.0", Update: "v1.1.0"},
	}, nil)
	_, errs := depsaudit.Classify(report, func(string) (bool, error) {
		return false, errors.New("go command failed")
	})
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want one classification failure", errs)
	}
}
