//go:build integration
// +build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

// auditGlintGateEnv gates every test in this file. The Govard-owned Magento
// lint image is large and each scenario runs real containers, so these tests are
// opt-in rather than part of the default integration run.
const auditGlintGateEnv = "GOVARD_TEST_AUDIT_GLINT"

// auditGlintFixture is the checked-in Magento-shaped tree every scenario
// copies. It carries all three target modes; see its README for the exact
// markers each mode relies on.
const auditGlintFixture = "magento2/audit-module"

// requireAuditGlintLive enforces the two-step gate. An unset gate is a
// deliberate opt-out and skips, but a set gate with no reachable Docker daemon is
// an environment block rather than a pass: the operator asked for live evidence,
// so silently reporting success would be a lie about what was verified.
func requireAuditGlintLive(t *testing.T) {
	t.Helper()

	if os.Getenv(auditGlintGateEnv) != "1" {
		t.Skipf("live Magento lint audit coverage is opt-in; set %s=1 to run it against a real Docker daemon", auditGlintGateEnv)
	}
	if err := exec.Command("docker", "version").Run(); err != nil {
		t.Fatalf("%s=1 asked for live Docker coverage but the Docker daemon is unreachable: %v", auditGlintGateEnv, err)
	}
}

// auditGlintEnvironment prepares one isolated live scenario: a private Govard
// home so no run touches the real ~/.govard caches or persisted sessions, and a
// private copy of the fixture so no run can mutate the checked-in tree.
type auditGlintEnvironment struct {
	env        *TestEnvironment
	home       string
	fixture    string
	project    string
	module     string
	standalone string
}

func newAuditGlintEnvironment(t *testing.T) *auditGlintEnvironment {
	t.Helper()
	requireAuditGlintLive(t)

	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	env := NewTestEnvironment(t)
	fixture := env.CreateProjectFromFixture(t, auditGlintFixture, "audit-module")
	return &auditGlintEnvironment{
		env:        env,
		home:       home,
		fixture:    fixture,
		project:    filepath.Join(fixture, "project"),
		module:     filepath.Join(fixture, "project", "app", "code", "Govard", "AuditSample"),
		standalone: filepath.Join(fixture, "standalone"),
	}
}

// pinProjectPHP writes the local config layer that selects the project's active
// PHP version. Project and module-in-project targets analyze exactly that
// version, so a scenario that wants a different one states it here rather than
// passing --php, which the command rejects when it disagrees with the project.
func (scenario *auditGlintEnvironment) pinProjectPHP(t *testing.T, version string) {
	t.Helper()

	local := filepath.Join(scenario.project, ".govard.local.yml")
	content := fmt.Sprintf("stack:\n  php_version: %q\n", version)
	if err := os.WriteFile(local, []byte(content), 0o644); err != nil {
		t.Fatalf("pin project PHP %s: %v", version, err)
	}
}

// auditGlintOutcome pairs the command result with the provider report the run
// persisted. The report is the raw, schema-versioned lint evidence the container
// published, so assertions read the same bytes a later `govard audit result`
// would rather than a summary rebuilt by the test.
type auditGlintOutcome struct {
	command *CommandResult
	result  audit.RunResult
	report  audit.LintReport
}

// runAuditGlint runs one audit from a directory inside the fixture and reads
// back both the command result and the persisted provider report.
func (scenario *auditGlintEnvironment) runAuditGlint(t *testing.T, workDir string, args ...string) auditGlintOutcome {
	t.Helper()

	command := scenario.env.RunGovard(t, workDir, append([]string{"audit", "run", "--format", "json"}, args...)...)
	if !command.Success() {
		t.Fatalf("govard audit run in %s failed with exit code %d\nstdout: %s\nstderr: %s", workDir, command.ExitCode, command.Stdout, command.Stderr)
	}

	var result audit.RunResult
	if err := json.Unmarshal([]byte(command.Stdout), &result); err != nil {
		t.Fatalf("decode audit run result: %v\nstdout: %s", err, command.Stdout)
	}
	return auditGlintOutcome{
		command: command,
		result:  result,
		report:  scenario.readLintReport(t, result),
	}
}

// readLintReport loads the provider report belonging to exactly this run. The
// path is derived from the run's own identity instead of picking the newest file,
// so a scenario that performs several runs can never assert against another
// run's evidence.
func (scenario *auditGlintEnvironment) readLintReport(t *testing.T, result audit.RunResult) audit.LintReport {
	t.Helper()

	path := filepath.Join(scenario.home, "audit", result.ProjectID, "sessions", result.SessionID, "runs", result.RunID, "report.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted lint report for run %s: %v", result.RunID, err)
	}
	var report audit.LintReport
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode persisted lint report %s: %v", path, err)
	}
	if report.SchemaVersion != audit.LintReportSchemaVersion {
		t.Fatalf("persisted lint report schema version is %d, want %d", report.SchemaVersion, audit.LintReportSchemaVersion)
	}
	if report.Provider != audit.GovardLintProvider {
		t.Fatalf("persisted lint report provider is %q, want %q", report.Provider, audit.GovardLintProvider)
	}
	return report
}

// assertLintMatrix checks that the report analyzed exactly the expected PHP
// versions and that every one of them produced usable evidence.
func assertLintMatrix(t *testing.T, report audit.LintReport, mode types.AuditTargetMode, wantVersions []string, wantCache string) {
	t.Helper()

	if report.TargetMode != mode {
		t.Fatalf("lint report target mode is %q, want %q", report.TargetMode, mode)
	}
	if report.Status != "passed" {
		t.Fatalf("lint report status is %q, want passed; php results: %+v", report.Status, report.PHPResults)
	}
	analyzed := make([]string, 0, len(report.PHPResults))
	for _, result := range report.PHPResults {
		analyzed = append(analyzed, result.PHPVersion)
		if result.Outcome != "passed" {
			t.Errorf("PHP %s outcome is %q, want passed; findings: %+v", result.PHPVersion, result.Outcome, result.Findings)
		}
		if wantCache != "" && result.Cache.State != wantCache {
			t.Errorf("PHP %s cache state is %q, want %q (%s)", result.PHPVersion, result.Cache.State, wantCache, result.Cache.Reason)
		}
		if len(result.Phases) == 0 {
			t.Errorf("PHP %s carries no phase evidence", result.PHPVersion)
		}
	}
	sort.Strings(analyzed)
	want := append([]string(nil), wantVersions...)
	sort.Strings(want)
	if strings.Join(analyzed, ",") != strings.Join(want, ",") {
		t.Fatalf("lint report analyzed PHP versions %v, want %v", analyzed, want)
	}
}

// TestAuditGlintProjectPHP74 covers the oldest supported launcher on a whole
// project target, which is only reachable for project and module-in-project
// modes.
func TestAuditGlintProjectPHP74(t *testing.T) {
	scenario := newAuditGlintEnvironment(t)

	outcome := scenario.runAuditGlint(t, scenario.project)

	if outcome.result.Status != audit.StatusPassed {
		t.Fatalf("audit run status is %q, want %q", outcome.result.Status, audit.StatusPassed)
	}
	assertLintMatrix(t, outcome.report, types.AuditTargetProject, []string{"7.4"}, "cold")
	if !outcome.report.MatrixComplete {
		t.Error("a project target analyzing its active PHP version must report a complete matrix")
	}
	if outcome.report.TargetPath != canonicalPathForTest(t, scenario.project) {
		t.Errorf("lint report target path is %q, want the canonical project root %q", outcome.report.TargetPath, canonicalPathForTest(t, scenario.project))
	}
}

// TestAuditGlintModuleInProjectPHP85 covers the newest supported launcher on a
// module resolved through its etc/module.xml declaration inside a project, which
// mounts the whole project read only but analyzes only the module.
func TestAuditGlintModuleInProjectPHP85(t *testing.T) {
	scenario := newAuditGlintEnvironment(t)
	scenario.pinProjectPHP(t, "8.5")

	outcome := scenario.runAuditGlint(t, scenario.module)

	assertLintMatrix(t, outcome.report, types.AuditTargetModule, []string{"8.5"}, "cold")
	if outcome.report.TargetPath != canonicalPathForTest(t, scenario.module) {
		t.Errorf("lint report target path is %q, want the module root %q", outcome.report.TargetPath, canonicalPathForTest(t, scenario.module))
	}
}

// TestAuditGlintStandaloneDefaultMatrix covers the full standalone default
// matrix. Standalone modules deliberately exclude 7.4 and 8.0, so all five
// remaining versions must run and the matrix must report itself complete.
func TestAuditGlintStandaloneDefaultMatrix(t *testing.T) {
	scenario := newAuditGlintEnvironment(t)

	outcome := scenario.runAuditGlint(t, scenario.standalone)

	assertLintMatrix(t, outcome.report, types.AuditTargetStandalone, []string{"8.1", "8.2", "8.3", "8.4", "8.5"}, "cold")
	if !outcome.report.MatrixComplete {
		t.Error("the standalone default matrix must report itself complete")
	}
}

// TestAuditGlintRejectsStandalonePHP80BeforeImageWork proves the PHP policy is
// enforced in Go before any image work is attempted. PHP 8.0 exists in the image
// but is valid only for project and module-in-project targets, so a standalone
// request for it must be refused without pulling, building, or running anything.
func TestAuditGlintRejectsStandalonePHP80BeforeImageWork(t *testing.T) {
	scenario := newAuditGlintEnvironment(t)

	// Every Docker call the audit path can make goes through the "docker"
	// binary, so shadowing it on PATH with a logging shim turns "no image work
	// happened" into an assertion about an empty log rather than a guess.
	shims := scenario.env.SetupRuntimeShims(t, nil)
	command := scenario.env.RunGovardWithEnv(t, scenario.standalone, shims.Env(), "audit", "run", "--format", "json", "--php", "8.0")

	if command.Success() {
		t.Fatalf("a standalone audit at PHP 8.0 must fail; stdout: %s", command.Stdout)
	}
	// Both streams are searched because the root command renders terminal errors
	// through pterm, which writes them to stdout rather than stderr; the test
	// asserts the diagnostic reached the operator, not which stream carried it.
	if diagnostics := command.Stdout + command.Stderr; !strings.Contains(diagnostics, "unsupported_php") {
		t.Errorf("standalone PHP 8.0 rejection must name the unsupported PHP policy, got: %s", diagnostics)
	}
	if log := shims.ReadLog(t); strings.TrimSpace(log) != "" {
		t.Errorf("standalone PHP 8.0 must be rejected before any Docker work, but Docker was invoked:\n%s", log)
	}
}

// TestAuditGlintWarmCacheAndInvalidation covers the reusable cache contract on
// a standalone target, which is the only mode that also populates a Composer
// download cache. A repeat run must reuse analyzer state, a changed project
// manifest must discard that state, and the Composer download cache - the
// expensive half - must survive the invalidation.
func TestAuditGlintWarmCacheAndInvalidation(t *testing.T) {
	scenario := newAuditGlintEnvironment(t)

	cold := scenario.runAuditGlint(t, scenario.standalone, "--php", "8.3")
	assertLintMatrix(t, cold.report, types.AuditTargetStandalone, []string{"8.3"}, "cold")

	warm := scenario.runAuditGlint(t, scenario.standalone, "--php", "8.3")
	assertLintMatrix(t, warm.report, types.AuditTargetStandalone, []string{"8.3"}, "warm")

	generation := scenario.cacheGeneration(t, warm.report.TargetID)
	composerBefore := auditGlintDirectoryDigest(t, filepath.Join(generation, "composer"))
	if composerBefore == "" {
		t.Fatal("a standalone run must populate the Composer download cache inside its cache generation")
	}

	// A changed manifest is exactly the input that must invalidate cached
	// analysis without discarding the Composer download cache with it.
	scenario.touchStandaloneManifest(t)

	invalidated := scenario.runAuditGlint(t, scenario.standalone, "--php", "8.3")
	assertLintMatrix(t, invalidated.report, types.AuditTargetStandalone, []string{"8.3"}, "cold")

	if invalidated.report.TargetID != warm.report.TargetID {
		t.Fatalf("a manifest change must not move the target namespace: %q became %q", warm.report.TargetID, invalidated.report.TargetID)
	}
	if after := scenario.cacheGeneration(t, invalidated.report.TargetID); after != generation {
		t.Errorf("a manifest change must reuse the toolchain cache generation %q, got %q", generation, after)
	}
	if auditGlintDirectoryDigest(t, filepath.Join(generation, "composer")) == "" {
		t.Error("a manifest change discarded the Composer download cache; only analyzer state may be invalidated")
	}

	// An explicit bypass must be reported as such, never as a warm reuse.
	bypassed := scenario.runAuditGlint(t, scenario.standalone, "--php", "8.3", "--no-lint-result-cache")
	assertLintMatrix(t, bypassed.report, types.AuditTargetStandalone, []string{"8.3"}, "bypassed")
}

// cacheGeneration resolves the single toolchain cache generation directory for a
// target. There is exactly one per toolchain identity, so more than one means the
// generation key drifted between runs that should have shared it.
func (scenario *auditGlintEnvironment) cacheGeneration(t *testing.T, targetID string) string {
	t.Helper()

	namespace := filepath.Join(scenario.home, "cache", "audit", "lint", targetID)
	entries, err := os.ReadDir(namespace)
	if err != nil {
		t.Fatalf("read lint cache namespace %s: %v", namespace, err)
	}
	generations := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			generations = append(generations, entry.Name())
		}
	}
	if len(generations) != 1 {
		t.Fatalf("lint cache namespace %s holds %d generations (%v), want exactly one", namespace, len(generations), generations)
	}
	return filepath.Join(namespace, generations[0])
}

func (scenario *auditGlintEnvironment) touchStandaloneManifest(t *testing.T) {
	t.Helper()

	path := filepath.Join(scenario.standalone, "composer.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read standalone manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode standalone manifest: %v", err)
	}
	manifest["description"] = "Invalidated by the live lint audit cache test."
	updated, err := json.MarshalIndent(manifest, "", "    ")
	if err != nil {
		t.Fatalf("encode standalone manifest: %v", err)
	}
	if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
		t.Fatalf("write standalone manifest: %v", err)
	}
}

// TestAuditGlintLeavesSourceUnchanged proves the source mount really is read
// only end to end. The image's own contract suite makes the same claim about the
// entrypoint; this asserts it for a full run driven through the CLI, where the
// mount flags, the container user, and the analyzer working directories all take
// part.
func TestAuditGlintLeavesSourceUnchanged(t *testing.T) {
	scenario := newAuditGlintEnvironment(t)

	before := auditGlintDirectoryDigest(t, scenario.fixture)
	if before == "" {
		t.Fatal("the fixture copy digested as empty")
	}

	scenario.runAuditGlint(t, scenario.project)
	scenario.runAuditGlint(t, scenario.standalone, "--php", "8.3")

	if after := auditGlintDirectoryDigest(t, scenario.fixture); after != before {
		t.Errorf("a lint audit modified its source tree: digest %s became %s", before, after)
	}
}

// auditGlintDirectoryDigest digests a whole tree by relative path, file mode,
// and content, so a changed byte, a changed permission, or an added or removed
// file all move the digest. An absent directory digests as the empty string,
// which callers use to distinguish "never created" from "changed".
func auditGlintDirectoryDigest(t *testing.T, root string) string {
	t.Helper()

	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}
		t.Fatalf("stat %s: %v", root, err)
	}

	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\n%t\n%04o\n", filepath.ToSlash(relative), entry.IsDir(), info.Mode().Perm())
		if entry.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%d\n", len(content))
		hash.Write(content)
		return nil
	})
	if err != nil {
		t.Fatalf("digest %s: %v", root, err)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// TestAuditGlintCancellationRemovesContainer covers cancellation and container
// cleanup. It drives the lint backend directly rather than the CLI because the
// CLI runs on a background context and has no signal-to-cancellation path, so a
// signal there would kill the process and orphan the container instead of
// exercising the backend's stop-then-remove cleanup.
func TestAuditGlintCancellationRemovesContainer(t *testing.T) {
	scenario := newAuditGlintEnvironment(t)

	definition, target, err := frameworks.ResolveAuditTarget(types.AuditTargetResolveRequest{
		StartPath:    scenario.standalone,
		ModeOverride: types.AuditTargetStandalone,
	})
	if err != nil {
		t.Fatalf("resolve the standalone audit target: %v", err)
	}
	if definition.AuditLint == nil {
		t.Fatalf("framework %q declares no lint audit profile", definition.Name)
	}

	toolchain := audit.NewToolchainManager(audit.NewExecDockerClient(nil), scenario.home)
	backend, err := audit.NewGovardLintBackend(audit.GovardLintOptions{
		Toolchain: toolchain,
		UID:       os.Getuid(),
		GID:       os.Getgid(),
	})
	if err != nil {
		t.Fatalf("construct the Govard lint backend: %v", err)
	}

	// The image is resolved up front, outside the cancellable context, so
	// cancellation lands on the lint container itself rather than on a first-time
	// image build that would never have reached one.
	if _, err := toolchain.Ensure(context.Background()); err != nil {
		t.Fatalf("prepare the lint toolchain image: %v", err)
	}

	request := audit.LintRequest{
		ProjectRoot:         target.TargetPath,
		ProjectID:           "project-livecancel",
		SessionID:           "session-livecancel",
		RunID:               "run-livecancel",
		Provider:            audit.GovardLintProvider,
		TargetID:            "target-livecancel",
		Target:              target,
		RunDir:              filepath.Join(t.TempDir(), "run"),
		CacheRoot:           filepath.Join(scenario.home, "cache", "audit", "lint"),
		Scope:               audit.ScopeProject,
		Jobs:                2,
		Profile:             *definition.AuditLint,
		SelectedPHPVersions: append([]string(nil), definition.AuditLint.StandalonePHPVersions...),
		MatrixComplete:      true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type runOutcome struct {
		report audit.LintReport
		err    error
	}
	done := make(chan runOutcome, 1)
	go func() {
		report, runErr := backend.Run(ctx, request)
		done <- runOutcome{report: report, err: runErr}
	}()

	containers := auditGlintWaitForContainer(t, request.SessionID)
	if len(containers) != 1 {
		t.Fatalf("expected exactly one lint container for session %q, got %v", request.SessionID, containers)
	}
	cancel()

	var outcome runOutcome
	select {
	case outcome = <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("the cancelled lint run did not return")
	}

	if outcome.err == nil {
		t.Fatal("a cancelled lint run must report an error")
	}
	if outcome.report.Status != "cancelled" {
		t.Errorf("a cancelled lint run must report status %q, got %q (%v)", "cancelled", outcome.report.Status, outcome.err)
	}

	// Cleanup is stop-then-remove, so the container must be gone entirely rather
	// than merely stopped: a lingering stopped container would collide with the
	// same name on a rerun.
	deadline := time.Now().Add(90 * time.Second)
	for {
		remaining := auditGlintContainers(t, request.SessionID)
		if len(remaining) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the cancelled lint container was not removed: %v", remaining)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// auditGlintWaitForContainer waits for the lint container of one session to
// exist. Containers are found by the audit session label the backend stamps on
// them, so the test never has to reimplement the backend's container naming.
func auditGlintWaitForContainer(t *testing.T, sessionID string) []string {
	t.Helper()

	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if containers := auditGlintContainers(t, sessionID); len(containers) > 0 {
			return containers
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no lint container appeared for session %q", sessionID)
	return nil
}

func auditGlintContainers(t *testing.T, sessionID string) []string {
	t.Helper()

	output, err := exec.Command("docker", "ps", "--all", "--no-trunc",
		"--filter", "label=io.govard.audit.session="+sessionID,
		"--format", "{{.Names}} {{.State}}").Output()
	if err != nil {
		t.Fatalf("list lint containers for session %q: %v", sessionID, err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	containers := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			containers = append(containers, trimmed)
		}
	}
	return containers
}
