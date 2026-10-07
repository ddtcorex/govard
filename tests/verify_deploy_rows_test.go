package tests

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// The deploy and remote rows the sandbox can answer. The remote always comes
// from --remote; the argv is what the title says it is.
var verifyDeployRowsForTest = []struct {
	id    string
	phase int
	guard string
	argv  []string
	// remoteless rows do not name a remote, so they run without --remote.
	remoteless bool
}{
	{"P4-17", 4, verify.GuardRemoteProbe, []string{"deploy", "check", "--remote", "sandbox"}, false},
	{"P4-18", 4, verify.GuardRemoteProbe, []string{"remote", "exec", "sandbox", "--", "hostname"}, false},
	{"P4-19", 4, "", []string{"remote", "audit", "stats"}, true},
	{"P4-20", 4, "", []string{"deploy", "rollback", "--help"}, true},
	{"P4-21", 4, "", []string{"deploy", "unlock", "--help"}, true},
	{"P2-15", 2, verify.GuardRemoteWrite, []string{"deploy", "--remote", "sandbox", "--yes", "--force"}, false},
}

func TestVerifyDeployRowsExistWithTheirArgvGuardAndPhase(t *testing.T) {
	cfg := engine.Config{Framework: "magento2", Domain: "sample.test"}
	argvs := captureItemArgvs(t, cfg, verify.VerifyOpts{Remote: "sandbox", ProjectRoot: t.TempDir()})
	byID := map[string]verify.Item{}
	for _, it := range verify.RegistryFor(cfg) {
		byID[it.ID] = it
	}
	for _, want := range verifyDeployRowsForTest {
		it, ok := byID[want.id]
		if !ok {
			t.Errorf("%s is missing from the registry", want.id)
			continue
		}
		if it.Phase != want.phase || it.Guard != want.guard {
			t.Errorf("%s phase=%d guard=%q, want phase %d guard %q", want.id, it.Phase, it.Guard, want.phase, want.guard)
		}
		if got := argvs[want.id]; !reflect.DeepEqual(got, want.argv) {
			t.Errorf("%s argv = %v, want %v", want.id, got, want.argv)
		}
		// The title is a claim about the argv: it must name the command and,
		// for a remote row, the placeholder the run resolves.
		command := strings.Join(want.argv, " ")
		command = strings.ReplaceAll(command, "sandbox", "<remote>")
		if !strings.Contains(it.Title, command) {
			t.Errorf("%s title %q does not contain its argv `%s`", want.id, it.Title, command)
		}
	}
}

func TestVerifyDeployRowsTakeTheRemoteFromTheFlagOnly(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	cfg := engine.Config{Framework: "magento2"}
	for _, want := range verifyDeployRowsForTest {
		item, ok := findItem(want.id)
		if !ok {
			t.Fatalf("%s is missing from the registry", want.id)
		}
		probe := installGuardProbe(t)
		ev := item.Run(context.Background(), cfg, verify.VerifyOpts{ProjectRoot: t.TempDir()})
		if want.remoteless {
			if ev.Skipped || probe.called() != 1 {
				t.Errorf("%s names no remote and must run without --remote: skipped=%v calls=%d", want.id, ev.Skipped, probe.called())
			}
			continue
		}
		if !ev.Skipped || !strings.Contains(ev.SkipReason, "--remote") || probe.called() != 0 {
			t.Errorf("%s without --remote: skipped=%v reason=%q calls=%d, want a skip naming --remote and no call", want.id, ev.Skipped, ev.SkipReason, probe.called())
		}
	}
}

// The rehearsal deploy writes through the remote: it is skipped unless the
// operator opted in, and it never runs in plan mode.
func TestVerifyRehearsalDeployNeedsRemoteWriteOptIn(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	cfg := engine.Config{Framework: "magento2"}

	probe := installGuardProbe(t)
	res, err := verify.RunPhase(context.Background(), cfg, 2, verify.VerifyOpts{Remote: "sandbox", ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	row := findGuardRow(t, res, "P2-15")
	if !row.Skipped || !strings.Contains(row.SkipReason, "--allow-remote-write") {
		t.Fatalf("P2-15 without opt-in: %+v, want a skip naming --allow-remote-write", row)
	}
	for _, argv := range probe.argv {
		if argv[0] == "deploy" {
			t.Fatalf("a deploy ran without --allow-remote-write: %v", argv)
		}
	}

	probe = installGuardProbe(t)
	res, err = verify.RunPhase(context.Background(), cfg, 2, verify.VerifyOpts{Remote: "sandbox", AllowRemoteWrite: true, ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if row := findGuardRow(t, res, "P2-15"); row.Skipped {
		t.Fatalf("P2-15 skipped with the opt-in and a remote: %+v", row)
	}
	ran := false
	for _, argv := range probe.argv {
		if reflect.DeepEqual(argv, []string{"deploy", "--remote", "sandbox", "--yes", "--force"}) {
			ran = true
		}
	}
	if !ran {
		t.Fatalf("P2-15 did not run `deploy --remote sandbox --yes --force`: %v", probe.argv)
	}
}
