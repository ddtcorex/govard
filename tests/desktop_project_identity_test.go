package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// TestCanonicalProjectID_GuardCatchesCrossIdentifierSync pins the audit
// finding: a sync registered under the project name must block lifecycle
// actions addressed by domain or root path. The guard keys on the raw
// identifier string today, so a Toggle by domain slips past a sync started
// by name.
func TestCanonicalProjectID_GuardCatchesCrossIdentifierSync(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	called := false
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		called = true
		return "should not run", nil
	})
	defer restore()

	desktop.RegisterSyncingProject("sample-project", "staging")
	defer desktop.UnregisterSyncingProject("sample-project")

	// The guard must resolve the domain to the same in-flight sync.
	if got := desktop.GetSyncingRemote("sample-project.test"); got != "staging" {
		t.Fatalf("GetSyncingRemote(domain) = %q, want %q (same sync as name)", got, "staging")
	}

	// End to end: Toggle addressed by domain must be refused with the
	// guard error naming the sync remote, and must not run any command.
	app := desktop.NewApp()
	if _, err := app.Environment.ToggleEnvironment("sample-project.test"); err == nil {
		t.Fatalf("expected ToggleEnvironment(domain) to refuse during sync started by name")
	} else if !strings.Contains(err.Error(), "staging") || !strings.Contains(strings.ToLower(err.Error()), "sync") {
		t.Fatalf("expected ToggleEnvironment(domain) guard error naming the sync remote, got: %v", err)
	}
	if called {
		t.Fatal("expected no CLI command to run during sync")
	}
}

// TestCanonicalProjectID_ResolvesNameDomainPathFuzzyEqual pins the core
// contract: every identifier form for one project canonicalizes to the
// registry project name, so later tasks can rely on stable keys.
func TestCanonicalProjectID_ResolvesNameDomainPathFuzzyEqual(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	want := "sample-project"
	for _, input := range []string{"sample-project", "sample-project.test", root, "sample", "  sample-project  "} {
		if got := desktop.CanonicalProjectIDForTest(input); got != want {
			t.Fatalf("CanonicalProjectID(%q) = %q, want %q", input, got, want)
		}
	}
	if got := desktop.CanonicalProjectIDForTest(""); got != "" {
		t.Fatalf("CanonicalProjectID(\"\") = %q, want empty", got)
	}
}

// TestCanonicalProjectID_FallsBackToRootPath pins the human decision:
// a project the registry does not know resolves to its root path.
func TestCanonicalProjectID_FallsBackToRootPath(t *testing.T) {
	desktop.ResetStateForTest()

	// Fresh empty registry: nothing is registered here.
	t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "orphan-project", "laravel", "orphan-project.test")

	if got, want := desktop.CanonicalProjectIDForTest(root), wantTestRoot(t, root); got != want {
		t.Fatalf("CanonicalProjectID(path) = %q, want root path %q", got, want)
	}
	if got := desktop.CanonicalProjectIDForTest("unknown-project"); got != "unknown-project" {
		t.Fatalf("CanonicalProjectID(unknown) = %q, want stable fallback %q", got, "unknown-project")
	}
}

// wantTestRoot resolves symlinks the way production keys do, so the
// expectation holds on hosts where TempDir itself sits under a symlink
// (e.g. /tmp on macOS workers).
func wantTestRoot(t *testing.T, root string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve test root %q: %v", root, err)
	}
	return filepath.Clean(resolved)
}

// TestCanonicalProjectID_WeakFuzzyDoesNotClaimProjectName pins the typo
// guard: a weak subsequence query must never canonicalize to a project
// name (DeleteProject refuses such matches for the same reason), while a
// strong prefix still resolves.
func TestCanonicalProjectID_WeakFuzzyDoesNotClaimProjectName(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	if got := desktop.CanonicalProjectIDForTest("sample-pro"); got != "sample-project" {
		t.Fatalf("CanonicalProjectID(strong prefix) = %q, want %q", got, "sample-project")
	}
	if got := desktop.CanonicalProjectIDForTest("smlpoet"); got == "sample-project" {
		t.Fatalf("CanonicalProjectID(weak subsequence %q) = project name, want path fallback (never a name)", "smlpoet")
	}
}

// TestCanonicalProjectID_NormalizesSymlinkedPath pins that a symlinked
// project path canonicalizes to the same key as the real path, so a sync
// started through the link and a guard addressed by the real path meet.
func TestCanonicalProjectID_NormalizesSymlinkedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinked fixture requires POSIX symlinks")
	}
	desktop.ResetStateForTest()

	// Fresh empty registry: nothing is registered here.
	t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "orphan-project", "laravel", "orphan-project.test")
	link := filepath.Join(t.TempDir(), "link-project")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("symlink test fixture: %v", err)
	}

	want := wantTestRoot(t, root)
	if got := desktop.CanonicalProjectIDForTest(link); got != want {
		t.Fatalf("CanonicalProjectID(symlink) = %q, want resolved root %q", got, want)
	}
	if got := desktop.CanonicalProjectIDForTest(root); got != want {
		t.Fatalf("CanonicalProjectID(real path) = %q, want resolved root %q", got, want)
	}
}

// TestCanonicalProjectID_SymlinkedPathResolvesToRegistryName pins that a
// symlinked path to a REGISTERED project still canonicalizes to the
// registry name, like the name, domain and real path do.
func TestCanonicalProjectID_SymlinkedPathResolvesToRegistryName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinked fixture requires POSIX symlinks")
	}
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")
	link := filepath.Join(t.TempDir(), "link-project")
	if err := os.Symlink(root, link); err != nil {
		t.Fatalf("symlink test fixture: %v", err)
	}

	for _, query := range []string{"sample-project", "sample-project.test", root, link} {
		if got := desktop.CanonicalProjectIDForTest(query); got != "sample-project" {
			t.Fatalf("CanonicalProjectID(%q) = %q, want %q", query, got, "sample-project")
		}
	}
}

// TestCanonicalProjectID_GuardCatchesPathStartedSync covers the review
// focus direction: sync started by path blocks a Toggle by domain.
func TestCanonicalProjectID_GuardCatchesPathStartedSync(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	called := false
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		called = true
		return "should not run", nil
	})
	defer restore()

	desktop.RegisterSyncingProject(root, "staging")
	defer desktop.UnregisterSyncingProject(root)

	app := desktop.NewApp()
	for _, query := range []string{"sample-project", "sample-project.test"} {
		if _, err := app.Environment.ToggleEnvironment(query); err == nil {
			t.Fatalf("expected ToggleEnvironment(%q) to refuse during sync started by path", query)
		} else if !strings.Contains(err.Error(), "staging") || !strings.Contains(strings.ToLower(err.Error()), "sync") {
			t.Fatalf("expected ToggleEnvironment(%q) guard error naming the sync remote, got: %v", query, err)
		}
	}
	if called {
		t.Fatal("expected no CLI command to run during sync")
	}
}

// TestCanonicalProjectID_ConcurrentClaimSingleWinner pins the single-mutex
// check+register: concurrent starters racing with mixed identifiers for
// the same project produce exactly one winner, and every identifier view
// reports the winner's remote. The suite runs under -race, so a split
// check-then-register fails here.
func TestCanonicalProjectID_ConcurrentClaimSingleWinner(t *testing.T) {
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	identifiers := []string{"sample-project", "sample-project.test", root, "sample"}

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if desktop.TryRegisterSyncingProject(identifiers[i%len(identifiers)], "staging") {
				wins.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("concurrent claims won %d times, want exactly 1", got)
	}
	for _, query := range identifiers {
		if got := desktop.GetSyncingRemote(query); got != "staging" {
			t.Fatalf("GetSyncingRemote(%q) = %q, want winner remote %q", query, got, "staging")
		}
	}

	// A second claim while the winner holds the project is rejected and
	// leaves the winner's entry untouched.
	if desktop.TryRegisterSyncingProject("sample-project.test", "other") {
		t.Fatalf("TryRegisterSyncingProject during in-flight sync = true, want false")
	}
	if got := desktop.GetSyncingRemote("sample-project"); got != "staging" {
		t.Fatalf("GetSyncingRemote after rejected claim = %q, want winner remote %q", got, "staging")
	}

	desktop.UnregisterSyncingProject("sample-project")
	for _, query := range identifiers {
		if got := desktop.GetSyncingRemote(query); got != "" {
			t.Fatalf("GetSyncingRemote(%q) after unregister = %q, want empty", query, got)
		}
	}
	// The project is claimable again after unregister.
	if !desktop.TryRegisterSyncingProject("sample-project.test", "staging") {
		t.Fatalf("TryRegisterSyncingProject after unregister = false, want true")
	}
	desktop.UnregisterSyncingProject("sample-project")
}

// TestSyncingRemoteLookupUsesCheapAliasKeysNoDocker pins the dashboard perf
// contract: once a sync is claimed, every sync-status lookup resolves from
// registry/alias keys and strings only, and shells docker zero times. The
// claim itself may shell docker once; the lookups may not.
func TestSyncingRemoteLookupUsesCheapAliasKeysNoDocker(t *testing.T) {
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	// Fresh empty registry: nothing is registered here.
	t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))

	// Docker shim: every invocation appends a line to the marker and fails
	// fast, so any docker shelling is observable and harmless.
	marker := filepath.Join(t.TempDir(), "docker-shim-marker.log")
	shimDir := t.TempDir()
	shim := filepath.Join(shimDir, "docker")
	script := "#!/bin/sh\necho invoke >> \"$GOVARD_TEST_DOCKER_SHIM_MARKER\"\nexit 1\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}
	t.Setenv("GOVARD_TEST_DOCKER_SHIM_MARKER", marker)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	const project = "sample-orphan"
	if !desktop.TryRegisterSyncingProject(project, "staging") {
		t.Fatalf("TryRegisterSyncingProject(%q) = false, want true", project)
	}
	defer desktop.UnregisterSyncingProject(project)

	rowsAfterClaim := readShimMarkerRows(t, marker)

	// The lookups must find the guard state without touching docker.
	for _, query := range []string{project, "  " + project + "  "} {
		if got := desktop.GetSyncingRemote(query); got != "staging" {
			t.Fatalf("GetSyncingRemote(%q) = %q, want %q", query, got, "staging")
		}
	}

	if rows := readShimMarkerRows(t, marker); rows != rowsAfterClaim {
		t.Fatalf("sync lookups shelled docker: %d new invocation rows (shim) after %d at claim", rows-rowsAfterClaim, rowsAfterClaim)
	}
}

func readShimMarkerRows(t *testing.T, marker string) int {
	t.Helper()
	data, err := os.ReadFile(marker)
	if err != nil {
		return 0
	}
	return len(strings.Split(strings.TrimSpace(string(data)), "\n"))
}

// TestSyncingAliasUnregisterScopesToItsOwnClaim pins alias ownership: a
// claim registers the alias keys it can produce cheaply, and unregister of
// one project sweeps only that claim's aliases, never a sibling project's.
func TestSyncingAliasUnregisterScopesToItsOwnClaim(t *testing.T) {
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	rootOne, rootTwo := t.TempDir(), t.TempDir()
	writeDesktopProjectConfigForTest(t, rootOne, "proj-one", "laravel", "proj-one.test")
	writeDesktopProjectConfigForTest(t, rootTwo, "proj-two", "laravel", "proj-two.test")
	registerTwoDesktopProjectsForTest(t, rootOne, rootTwo)

	for project, remote := range map[string]string{"proj-one": "staging", "proj-two": "qa"} {
		if !desktop.TryRegisterSyncingProject(project, remote) {
			t.Fatalf("TryRegisterSyncingProject(%q) = false, want true", project)
		}
	}
	desktop.UnregisterSyncingProject("proj-one")

	if got := desktop.GetSyncingRemote("proj-one"); got != "" {
		t.Fatalf("GetSyncingRemote(proj-one) after unregister = %q, want empty", got)
	}
	for _, query := range []string{"proj-two", "proj-two.test"} {
		if got := desktop.GetSyncingRemote(query); got != "qa" {
			t.Fatalf("GetSyncingRemote(%q) = %q, want sibling sync intact (%q)", query, got, "qa")
		}
	}
	// proj-one is claimable again after its unregister.
	if !desktop.TryRegisterSyncingProject("proj-one.test", "reclaim") {
		t.Fatalf("TryRegisterSyncingProject(proj-one.test) after unregister = false, want true")
	}
	desktop.UnregisterSyncingProject("proj-one")
}

// TestSyncingExportedUnregisterSweepsAliasesForReclaim closes the review
// finding: the claim side differs from the lookup candidates (claim by an
// absolute path yields a path primary while the lookup walks the lowercase
// alias), a lookup without docker finds the claim through the alias branch,
// and the exported UnregisterSyncingProject sweeps that alias so a later
// identifier view of the same project can claim again. Pre-fix the stale
// alias blocked the reclaim: the conflict check reads it as another live
// claim's, which is exactly the regression this pins.
func TestSyncingExportedUnregisterSweepsAliasesForReclaim(t *testing.T) {
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	// Fresh empty registry and a docker shim that never succeeds, so any
	// docker shelling beyond the claim itself is traceable through the
	// marker; the claim here resolves without docker (path with config).
	t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))
	marker := filepath.Join(t.TempDir(), "docker-shim-marker.log")
	shimDir := t.TempDir()
	shim := filepath.Join(shimDir, "docker")
	script := "#!/bin/sh\necho invoke >> \"$GOVARD_TEST_DOCKER_SHIM_MARKER\"\nexit 1\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}
	t.Setenv("GOVARD_TEST_DOCKER_SHIM_MARKER", marker)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	claimPath := filepath.Join(root, "OrphanProj")
	if err := os.MkdirAll(claimPath, 0o755); err != nil {
		t.Fatalf("mkdir claim path: %v", err)
	}
	writeDesktopProjectConfigForTest(t, claimPath, "orphan-project", "laravel", "orphan-project.test")

	// Claim by path: primary is the canonical path key; the lowercase
	// candidate ends up in the alias map (it differs from the primary).
	if !desktop.TryRegisterSyncingProject(claimPath, "staging") {
		t.Fatalf("TryRegisterSyncingProject(%q) = false, want true", claimPath)
	}
	defer desktop.UnregisterSyncingProject(claimPath)

	rowsAfterClaim := readShimMarkerRows(t, marker)

	// Path view: primary hit. Lowercase view: alias hit. Both without
	// touching docker.
	for _, query := range []string{claimPath, strings.ToLower(claimPath)} {
		if got := desktop.GetSyncingRemote(query); got != "staging" {
			t.Fatalf("GetSyncingRemote(%q) = %q, want %q", query, got, "staging")
		}
	}
	if rows := readShimMarkerRows(t, marker); rows != rowsAfterClaim {
		t.Fatalf("lookups shelled docker: %d new shim rows after %d at claim", rows-rowsAfterClaim, rowsAfterClaim)
	}

	desktop.UnregisterSyncingProject(claimPath)

	// The lowercase view must be reclaimable now that the claim released.
	if !desktop.TryRegisterSyncingProject(strings.ToLower(claimPath), "reclaim") {
		t.Fatalf("TryRegisterSyncingProject(lowercase path) after unregister = false, want true (registry/alias scoping)")
	}
	desktop.UnregisterSyncingProject(strings.ToLower(claimPath))
	if got := desktop.GetSyncingRemote(strings.ToLower(claimPath)); got != "" {
		t.Fatalf("GetSyncingRemote(lowercase path) after release = %q, want empty", got)
	}
}

// TestSyncingUnregisterSweepsItsAliases pins alias hygiene: a claim that
// writes alias candidates (claim by path yields a path primary and a
// lowercase alias) leaves no alias behind after the exported
// UnregisterSyncingProject release. A stale alias cannot misdirect a
// lookup's result today, but it keeps claiming session state for a project
// whose sync is long gone.
func TestSyncingUnregisterSweepsItsAliases(t *testing.T) {
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	root := t.TempDir()
	claimPath := root + "/OrphanProj"
	if err := os.MkdirAll(claimPath, 0o755); err != nil {
		t.Fatalf("mkdir claim path: %v", err)
	}
	writeDesktopProjectConfigForTest(t, claimPath, "orphan-project", "laravel", "orphan-project.test")

	if !desktop.TryRegisterSyncingProject(claimPath, "staging") {
		t.Fatalf("TryRegisterSyncingProject(%q) = false, want true", claimPath)
	}
	if len(desktop.SyncingAliasesForTest()) == 0 {
		t.Fatalf("expected the claim to register an alias candidate (path primary plus lowercase alias)")
	}

	desktop.UnregisterSyncingProject(claimPath)
	if len(desktop.SyncingAliasesForTest()) != 0 {
		t.Fatalf("UnregisterSyncingProject left aliases behind: %v", desktop.SyncingAliasesForTest())
	}
}
