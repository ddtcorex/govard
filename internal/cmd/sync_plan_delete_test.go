package cmd

import (
	"io"
	"strings"
	"testing"

	"govard/internal/engine"
)

// protectedSyncPatterns are the paths a `--delete` run must never remove from
// either side of a sync: source control, local secrets and installed
// dependencies cannot be regenerated from a deploy release.
var protectedSyncPatterns = []string{".git/", "node_modules/", ".env", "auth.json", "*.pem", "*.key", ".govard*.yml", ".govard/"}

// frameworkProtectedSyncPatterns are the paths the framework itself declares as
// local only (its clone excludes). app/etc/env.php holds the database
// credentials and the crypt key, which no deploy release can give back.
var frameworkProtectedSyncPatterns = []string{"app/etc/env.php"}

func syncEndpointsForDeleteTest(destinationLocal bool) ResolvedSyncEndpoints {
	remoteEndpoint := SyncEndpoint{Name: "staging", IsLocal: false, RootPath: "/var/www/sample-project/current"}
	localEndpoint := SyncEndpoint{Name: "local", IsLocal: true, RootPath: "/srv/sample-project"}
	if destinationLocal {
		return ResolvedSyncEndpoints{Source: remoteEndpoint, Destination: localEndpoint}
	}
	return ResolvedSyncEndpoints{Source: localEndpoint, Destination: remoteEndpoint}
}

func assertArgsContainExclude(t *testing.T, args []string, pattern string) {
	t.Helper()
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--exclude" && args[i+1] == pattern {
			return
		}
	}
	t.Fatalf("rsync args %v carry no --exclude %q", args, pattern)
}

func TestSyncDeleteKeepsProtectedPathsWithoutNoNoise(t *testing.T) {
	plan, err := buildSyncExecutionPlan(
		engine.Config{Framework: "magento2"},
		syncEndpointsForDeleteTest(true),
		SyncExecutionOptions{Files: true, Delete: true},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("buildSyncExecutionPlan: %v", err)
	}
	if len(plan.RsyncCommands) != 1 {
		t.Fatalf("got %d rsync commands, want 1", len(plan.RsyncCommands))
	}
	args := plan.RsyncCommands[0].Args
	found := false
	for _, arg := range args {
		if arg == "--delete" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rsync args %v carry no --delete", args)
	}
	for _, pattern := range append(protectedSyncPatterns, frameworkProtectedSyncPatterns...) {
		assertArgsContainExclude(t, args, pattern)
	}
}

func TestSyncDeleteKeepsProtectedPathsOnRemoteDestination(t *testing.T) {
	plan, err := buildSyncExecutionPlan(
		engine.Config{Framework: "magento2"},
		syncEndpointsForDeleteTest(false),
		SyncExecutionOptions{Files: true, Delete: true},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("buildSyncExecutionPlan: %v", err)
	}
	if len(plan.RsyncCommands) != 1 {
		t.Fatalf("got %d rsync commands, want 1", len(plan.RsyncCommands))
	}
	args := plan.RsyncCommands[0].Args
	for _, pattern := range append(protectedSyncPatterns, frameworkProtectedSyncPatterns...) {
		assertArgsContainExclude(t, args, pattern)
	}
}

func TestSyncWithoutDeleteDoesNotAddProtectedExcludes(t *testing.T) {
	plan, err := buildSyncExecutionPlan(
		engine.Config{Framework: "magento2"},
		syncEndpointsForDeleteTest(true),
		SyncExecutionOptions{Files: true},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("buildSyncExecutionPlan: %v", err)
	}
	if len(plan.RsyncCommands) != 1 {
		t.Fatalf("got %d rsync commands, want 1", len(plan.RsyncCommands))
	}
	for _, arg := range plan.RsyncCommands[0].Args {
		if arg == ".git/" || strings.HasPrefix(arg, ".git") {
			t.Fatalf("a plain sync copied .git once already, no exclude expected, got args %v", plan.RsyncCommands[0].Args)
		}
	}
}

// A media sync with --delete is destructive in the same way a files sync is, so
// it carries the same protected excludes.
func TestSyncDeleteKeepsProtectedPathsOnMediaSync(t *testing.T) {
	plan, err := buildSyncExecutionPlan(
		engine.Config{Framework: "magento2"},
		syncEndpointsForDeleteTest(true),
		SyncExecutionOptions{Media: "all", Delete: true},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("buildSyncExecutionPlan: %v", err)
	}
	if len(plan.RsyncCommands) != 1 {
		t.Fatalf("got %d rsync commands, want 1", len(plan.RsyncCommands))
	}
	args := plan.RsyncCommands[0].Args
	for _, pattern := range append(protectedSyncPatterns, frameworkProtectedSyncPatterns...) {
		assertArgsContainExclude(t, args, pattern)
	}
}
