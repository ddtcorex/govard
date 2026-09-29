package tests

import (
	"reflect"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// Every verify item is a command string, and a string no command accepts still
// exits 0: cobra answers an unreachable word with its help page and returns nil,
// so the item records a green from a page nobody ran. These tests pin the argv of
// the items that had that defect to the argv the command tree actually accepts.
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
