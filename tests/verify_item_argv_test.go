package tests

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/verify"
)

// Every verify item is a command string, and a string the command tree does not
// accept still produces a verdict: an unreachable word gets cobra's help page and
// exit 0 — a green from a page nobody ran — while an unknown flag gets a usage
// error, a red the item can never clear. These tests pin each item that carried
// one of those defects to the argv the command tree actually accepts.
//
// They capture through captureItemArgvs (tests/verify_guard_contract_test.go)
// rather than installing a fake of their own: the helper already answers every
// item with ExitCode 1, which ends an item at its first govard invocation, and
// both items below make exactly one — so the captured argv is the item's argv.

// `tool` is a fixed subcommand registry with no RunE (internal/cmd/frameworks.go),
// so `govard tool redis-cli ping` left `redis-cli ping` as a leftover argument:
// cobra hit its not-runnable branch, printed help and returned nil — exit 0,
// before the runtime capability gate could run, because that gate lives in
// PersistentPreRunE of a command that never got past help. The item recorded a
// green from that help path (issue #475).
//
// `govard redis cli ping` is the real command, dual-registered under root and
// `env` (internal/cmd/redis.go): it resolves redis-cli or valkey-cli from
// stack.services.cache and exits non-zero when the cache is down, which is what
// the item claims to check.
func TestP410RunsTheRealRedisCommand(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := captureItemArgvs(t, engine.Config{Framework: "magento2"}, verify.VerifyOpts{
		ProjectRoot: t.TempDir(),
	})

	want := []string{"redis", "cli", "ping"}
	if got := argvs["P4-10"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("P4-10 invoked %v, want %v", got, want)
	}
}

// `env up` has no --build flag (internal/cmd/up.go's addUpFlags registers nine,
// and --build is not one of them), so the item was a guaranteed exit-2 usage
// error in every phase-2 run: cobra rejects an unknown flag before RunE, and a
// usage error is not a check that failed — the phase could never go green
// (issue #491). --force-recreate keeps the item's intent, a re-create pass that
// P2-01's plain `env up` does not exercise.
//
// The second half resolves that argv against the real command tree, so editing
// the registry string alone can no longer reintroduce the class: the flag is
// looked up in the command's own flag set, the same one cobra parses at run
// time. Find reads the command path out of the argv, so a rename of the command
// fails here too.
func TestP203UsesAFlagTheCommandAccepts(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	argvs := captureItemArgvs(t, engine.Config{Framework: "magento2"}, verify.VerifyOpts{
		ProjectRoot: t.TempDir(),
	})

	want := []string{"env", "up", "--force-recreate"}
	argv := argvs["P2-03"]
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("P2-03 invoked %v, want %v", argv, want)
	}

	command, rest, err := cmd.RootCommandForTest().Find(argv)
	if err != nil {
		t.Fatalf("resolve `govard %s`: %v", strings.Join(argv, " "), err)
	}
	if err := command.Flags().Parse(rest); err != nil {
		t.Fatalf("`govard %s` resolves to `%s`, which rejects it: %v",
			strings.Join(argv, " "), command.CommandPath(), err)
	}
}
