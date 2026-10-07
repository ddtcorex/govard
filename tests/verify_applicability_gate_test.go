package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// planRow runs one phase in plan mode and returns the row of id. Plan mode
// stubs every Run, so a row that is not skipped reports "plan: <title>" and a
// gated one reports skipped with its reason: the gate decision alone.
func planRow(t *testing.T, cfg engine.Config, phase int, id string) verify.RunItem {
	t.Helper()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	res, err := verify.RunPhase(context.Background(), cfg, phase, verify.VerifyOpts{Plan: true, ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase %d: %v", phase, err)
	}
	for _, row := range res.Items {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("%s left no row in phase %d (%d rows)", id, phase, len(res.Items))
	return verify.RunItem{}
}

func wantSkipped(t *testing.T, row verify.RunItem, reasonPart string) {
	t.Helper()
	if !row.Skipped {
		t.Fatalf("%s Skipped = false, want true: %+v", row.ID, row)
	}
	if !strings.Contains(row.SkipReason, reasonPart) {
		t.Fatalf("%s skip reason = %q, want it to contain %q", row.ID, row.SkipReason, reasonPart)
	}
}

func wantRuns(t *testing.T, row verify.RunItem) {
	t.Helper()
	if row.Skipped {
		t.Fatalf("%s is skipped (%q), want it to run", row.ID, row.SkipReason)
	}
}

func cfgWithServices(framework, cache, search string) engine.Config {
	cfg := engine.Config{Framework: framework}
	cfg.Stack.Services.Cache = cache
	cfg.Stack.Services.Search = search
	return cfg
}

// The redis probe and the search probe only mean something on a stack that
// declares the service. An empty or "none" setting is a project that does not
// run one, and probing it was a permanent red.
func TestVerifyCacheProbeGatedOnConfiguredCacheService(t *testing.T) {
	for _, cache := range []string{"", "none"} {
		row := planRow(t, cfgWithServices("laravel", cache, ""), 4, "P4-10")
		wantSkipped(t, row, "stack.services.cache")
	}
	for _, cache := range []string{"redis", "valkey"} {
		wantRuns(t, planRow(t, cfgWithServices("magento2", cache, ""), 4, "P4-10"))
	}
}

func TestVerifySearchProbeGatedOnConfiguredSearchService(t *testing.T) {
	for _, search := range []string{"", "none"} {
		row := planRow(t, cfgWithServices("wordpress", "", search), 4, "P4-11")
		wantSkipped(t, row, "stack.services.search")
	}
	for _, search := range []string{"opensearch", "elasticsearch"} {
		wantRuns(t, planRow(t, cfgWithServices("magento2", "", search), 4, "P4-11"))
	}
}

// Audit rows follow what the framework definition declares. The audit command
// itself rejects the check with exit 1 otherwise.
func TestVerifyAuditRowsGatedOnFrameworkSupport(t *testing.T) {
	if engine.FrameworkSupportsAuditIntegrity("laravel") || engine.FrameworkSupportsAuditProfiler("laravel") {
		t.Fatal("fixture drift: laravel now declares integrity or profiler, pick another framework for this test")
	}
	if !engine.FrameworkSupportsAuditIntegrity("magento2") || !engine.FrameworkSupportsAuditProfiler("magento2") {
		t.Fatal("fixture drift: magento2 no longer declares integrity and profiler")
	}

	laravel := engine.Config{Framework: "laravel", Domain: "x.test"}
	wantSkipped(t, planRow(t, laravel, 3, "P3-12"), "profiler")
	wantSkipped(t, planRow(t, laravel, 3, "P3-15"), "integrity")

	magento := engine.Config{Framework: "magento2", Domain: "x.test"}
	wantRuns(t, planRow(t, magento, 3, "P3-12"))
	wantRuns(t, planRow(t, magento, 3, "P3-15"))

	// Lint is declared by the four main frameworks and by no others.
	wantRuns(t, planRow(t, laravel, 3, "P3-10"))
	wantSkipped(t, planRow(t, engine.Config{Framework: "django"}, 3, "P3-10"), "lint")
	wantSkipped(t, planRow(t, engine.Config{Framework: "django"}, 5, "P5-04"), "lint")
}

func runWithFake(t *testing.T, id string, cfg engine.Config, root string) (verify.Evidence, [][]string) {
	t.Helper()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	item, ok := findItem(id)
	if !ok {
		t.Fatalf("%s is missing from the registry", id)
	}
	var argvs [][]string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		argvs = append(argvs, append([]string(nil), args...))
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return item.Run(context.Background(), cfg, verify.VerifyOpts{ProjectRoot: root}), argvs
}

func writeProjectFile(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// `tool composer validate` and the phpstan row need the files they act on.
func TestVerifyComposerAndPhpstanRowsSkipWithoutTheirMarker(t *testing.T) {
	cfg := engine.Config{Framework: "wordpress"}

	bare := t.TempDir()
	for id, marker := range map[string]string{"P2-12": "composer.json", "P3-07": "vendor/bin/phpstan"} {
		ev, argvs := runWithFake(t, id, cfg, bare)
		if !ev.Skipped || !strings.Contains(ev.SkipReason, marker) {
			t.Errorf("%s on a root without %s: Skipped=%v reason=%q, want a skip naming the marker", id, marker, ev.Skipped, ev.SkipReason)
		}
		if len(argvs) != 0 {
			t.Errorf("%s ran %v without its marker", id, argvs)
		}
	}

	full := t.TempDir()
	writeProjectFile(t, full, "composer.json")
	writeProjectFile(t, full, "vendor/bin/phpstan")
	for _, id := range []string{"P2-12", "P3-07"} {
		ev, argvs := runWithFake(t, id, cfg, full)
		if ev.Skipped || len(argvs) != 1 {
			t.Errorf("%s with its marker: Skipped=%v argvs=%v, want one run", id, ev.Skipped, argvs)
		}
	}
}

// `doctor trust` installs into the system trust store through sudo. The verify
// child has no terminal, so without passwordless sudo it can only fail: that
// is an environment fact, reported as a skip, not a defect.
func TestVerifyDoctorTrustSkipsWithoutNonInteractiveSudo(t *testing.T) {
	cfg := engine.Config{Framework: "laravel"}

	verify.SetSudoProbeForTest(func(context.Context) bool { return false })
	t.Cleanup(func() { verify.SetSudoProbeForTest(nil) })
	ev, argvs := runWithFake(t, "P1-03", cfg, t.TempDir())
	if !ev.Skipped || !strings.Contains(ev.SkipReason, "sudo") {
		t.Fatalf("P1-03 without sudo: Skipped=%v reason=%q, want a skip naming sudo", ev.Skipped, ev.SkipReason)
	}
	if len(argvs) != 0 {
		t.Fatalf("P1-03 ran %v without sudo", argvs)
	}

	verify.SetSudoProbeForTest(func(context.Context) bool { return true })
	ev, argvs = runWithFake(t, "P1-03", cfg, t.TempDir())
	if ev.Skipped || len(argvs) != 1 || strings.Join(argvs[0], " ") != "doctor trust" {
		t.Fatalf("P1-03 with sudo: Skipped=%v argvs=%v, want one `doctor trust`", ev.Skipped, argvs)
	}
}
