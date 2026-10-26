package tests

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func installXdebugProbe(t *testing.T, loaded bool, err error) *int {
	t.Helper()
	calls := 0
	restore := cmd.SetAuditXdebugProbeForTest(func(context.Context, *engine.Config) (bool, error) {
		calls++
		return loaded, err
	})
	t.Cleanup(restore)
	return &calls
}

func profilerAuditArgs(extra ...string) []string {
	return append([]string{"audit", "run", "--checks", "profiler", "--url", "https://shop.test/", "--format", "json"}, extra...)
}

func installFakeProfiler(t *testing.T) {
	t.Helper()
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{ProfilerRuntime: &fakeProfilerRuntime{csv: []byte("type,timer\nfoo,1\n")}})
	t.Cleanup(restore)
}

func TestXdebugLintOnlyNeverGated(t *testing.T) {
	project := auditCommandProjectWithXdebug(t, "magento2", true)
	installAuditCommandDependencies(t, &commandLintBackend{})
	calls := installXdebugProbe(t, true, nil)
	_, err := executeAuditCommand(t, project, []string{"audit", "run", "--checks", "lint", "--format", "json"})
	if err != nil && strings.Contains(err.Error(), "Xdebug") {
		t.Fatalf("lint must never be gated by Xdebug: %v", err)
	}
	if *calls != 0 {
		t.Fatalf("probe calls = %d, lint must not start a probe", *calls)
	}
}

func TestXdebugProfilerBlockedWhenLoaded(t *testing.T) {
	project := auditCommandProjectWithXdebug(t, "magento2", true)
	installFakeProfiler(t)
	installXdebugProbe(t, true, nil)
	_, err := executeAuditCommand(t, project, profilerAuditArgs())
	if err == nil || !strings.Contains(err.Error(), "--allow-xdebug") {
		t.Fatalf("profiler with Xdebug loaded must block naming --allow-xdebug, got %v", err)
	}
}

func TestXdebugProfilerAllowedWhenNotLoaded(t *testing.T) {
	project := auditCommandProjectWithXdebug(t, "magento2", true)
	installFakeProfiler(t)
	installXdebugProbe(t, false, nil)
	var stderr bytes.Buffer
	_, err := executeAuditCommandWithStderr(t, project, profilerAuditArgs(), &stderr)
	if err != nil {
		t.Fatalf("profiler without Xdebug in the php container must run: %v", err)
	}
	if strings.Count(stderr.String(), "php-debug") != 1 {
		t.Fatalf("stderr = %q, want one warning naming php-debug", stderr.String())
	}
}

func TestXdebugProfilerAllowFlagSkipsProbe(t *testing.T) {
	project := auditCommandProjectWithXdebug(t, "magento2", true)
	installFakeProfiler(t)
	calls := installXdebugProbe(t, true, nil)
	if _, err := executeAuditCommand(t, project, profilerAuditArgs("--allow-xdebug")); err != nil {
		t.Fatalf("--allow-xdebug: %v", err)
	}
	if *calls != 0 {
		t.Fatalf("probe calls = %d, --allow-xdebug must skip the probe", *calls)
	}
}

func TestXdebugProbeFailureFailsSafe(t *testing.T) {
	project := auditCommandProjectWithXdebug(t, "magento2", true)
	installFakeProfiler(t)
	installXdebugProbe(t, false, errors.New("No such container: shop-php-1"))
	_, err := executeAuditCommand(t, project, profilerAuditArgs())
	if err == nil || !strings.Contains(err.Error(), "govard env up") || !strings.Contains(err.Error(), "--allow-xdebug") {
		t.Fatalf("an unreachable probe must block naming govard env up and --allow-xdebug, got %v", err)
	}
}
