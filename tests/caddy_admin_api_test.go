package tests

import (
	"errors"
	"slices"
	"testing"

	"govard/internal/proxy"
)

func TestLoadCaddyConfigFailsWhenAdminAPIRejectsRequest(t *testing.T) {
	restore := proxy.SetCaddyExecRunnerForTest(func(_ string, args ...string) ([]byte, error) {
		if !slices.Contains(args, "--fail") {
			t.Fatalf("Caddy load curl args = %#v, want --fail for non-2xx responses", args)
		}
		return []byte("invalid configuration"), errors.New("exit status 22")
	})
	defer restore()

	err := proxy.LoadCaddyConfigForTest("synthetic-caddy", map[string]interface{}{"apps": map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected rejected Caddy Admin API load to fail")
	}
}
