package tests

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// The read-only host and project commands the checklist reaches. Each runs on
// a healthy project with exit 0 and writes nothing, so none carries a guard.
var verifyHostRowsForTest = []struct {
	id   string
	argv []string
}{
	{"P1-08", []string{"capabilities", "--json"}},
	{"P1-09", []string{"config", "profile"}},
	{"P1-10", []string{"domain", "list"}},
	{"P1-11", []string{"custom", "list"}},
	{"P1-12", []string{"project", "orphans"}},
	{"P1-13", []string{"gateway", "status"}},
	{"P1-14", []string{"audit", "toolchain", "status"}},
	{"P1-15", []string{"sandbox", "status"}},
}

func TestVerifyHostRowsExistWithTheirArgv(t *testing.T) {
	cfg := engine.Config{Framework: "laravel", Domain: "sample.test"}
	argvs := captureItemArgvs(t, cfg, verify.VerifyOpts{ProjectRoot: t.TempDir()})
	byID := map[string]verify.Item{}
	for _, it := range verify.RegistryFor(cfg) {
		byID[it.ID] = it
	}
	for _, want := range verifyHostRowsForTest {
		it, ok := byID[want.id]
		if !ok {
			t.Errorf("%s is missing from the registry", want.id)
			continue
		}
		if it.Phase != 1 || it.Guard != "" {
			t.Errorf("%s phase=%d guard=%q, want phase 1 with no guard: it reads local state only", want.id, it.Phase, it.Guard)
		}
		if !strings.HasPrefix(it.Title, "govard "+strings.Join(want.argv, " ")) {
			t.Errorf("%s title %q does not name `govard %s`", want.id, it.Title, strings.Join(want.argv, " "))
		}
		if got := argvs[want.id]; !reflect.DeepEqual(got, want.argv) {
			t.Errorf("%s argv = %v, want %v", want.id, got, want.argv)
		}
	}
}

// `capabilities --json` is a machine contract: a zero exit with prose on
// stdout is a defect, so the row turns it red.
func TestVerifyCapabilitiesRowRequiresValidJSON(t *testing.T) {
	item, ok := findItem("P1-08")
	if !ok {
		t.Fatal("P1-08 is missing from the registry")
	}
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	for _, tc := range []struct {
		name     string
		ev       verify.Evidence
		wantExit int
	}{
		{"valid JSON passes", verify.Evidence{ExitCode: 0, JSONValid: true, OutputExcerpt: "{}"}, 0},
		{"exit 0 with prose is red", verify.Evidence{ExitCode: 0, JSONValid: false, OutputExcerpt: "hello"}, 1},
		{"a failing exit is kept", verify.Evidence{ExitCode: 3, JSONValid: false}, 3},
	} {
		verify.SetExecGovardFakeForTest(func(context.Context, engine.Config, verify.VerifyOpts, ...string) (verify.Evidence, bool) {
			return tc.ev, true
		})
		got := item.Run(context.Background(), engine.Config{Framework: "laravel"}, verify.VerifyOpts{ProjectRoot: t.TempDir()})
		if got.ExitCode != tc.wantExit {
			t.Errorf("%s: exit = %d, want %d", tc.name, got.ExitCode, tc.wantExit)
		}
	}
}
