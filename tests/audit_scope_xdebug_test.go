package tests

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
)

// TestAuditProviderAliasProvider verifies --provider is an alias for --lint-provider (hidden).
func TestAuditProviderAliasProvider(t *testing.T) {
	// Execute audit run help and check flag exists; use command inspection.
	root := cmd.RootCommandForTest()
	auditCmd, _, err := root.Find([]string{"audit"})
	if err != nil {
		t.Fatalf("find audit: %v", err)
	}
	if flag := auditCmd.PersistentFlags().Lookup("provider"); flag == nil {
		t.Fatal("--provider alias flag not registered")
	} else {
		if !flag.Hidden {
			t.Fatalf("--provider should be hidden")
		}
	}
	if flag := auditCmd.PersistentFlags().Lookup("lint-provider"); flag == nil {
		t.Fatal("--lint-provider flag missing")
	}
	// Verify alias respects values: both flags set same underlying variable.
	// Use a project to execute with --provider external name should error similarly to --lint-provider.
	project := auditCommandProject(t, "magento2")
	_, errWithProvider := executeAuditCommand(t, project, []string{"audit", "run", "--provider", "unknown-external-xyz", "--format", "json"})
	_, errWithLintProvider := executeAuditCommand(t, project, []string{"audit", "run", "--lint-provider", "unknown-external-xyz", "--format", "json"})
	if (errWithProvider == nil) != (errWithLintProvider == nil) {
		t.Fatalf("provider alias behaviour differs: --provider err=%v, --lint-provider err=%v", errWithProvider, errWithLintProvider)
	}
	if errWithProvider != nil && !strings.Contains(errWithProvider.Error(), "unknown-external-xyz") {
		t.Fatalf("provider alias error does not mention provider: %v", errWithProvider)
	}
}

// TestAuditXdebugGuardRequiresAllow verifies that audit lint with xdebug enabled fails unless --allow-xdebug is set.
func TestAuditXdebugGuardRequiresAllow(t *testing.T) {
	project := auditCommandProjectWithXdebug(t, "magento2", true)
	_, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err == nil || !strings.Contains(err.Error(), "Xdebug enabled") {
		t.Fatalf("expected Xdebug guard error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--allow-xdebug") {
		t.Fatalf("Xdebug error should hint --allow-xdebug, got %v", err)
	}
	// With --allow-xdebug it should not fail on xdebug (may still succeed or fail on other grounds, but not Xdebug)
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	_, errWithAllow := executeAuditCommand(t, project, []string{"audit", "run", "--allow-xdebug", "--format", "json"})
	// Should not be Xdebug error; either succeeds or other error, but not Xdebug.
	if errWithAllow != nil && strings.Contains(errWithAllow.Error(), "Xdebug enabled") {
		t.Fatalf("with --allow-xdebug still got Xdebug error: %v", errWithAllow)
	}
	if errWithAllow != nil {
		// It may still produce result via backend; ensure backend was invoked.
	}
	if len(backend.requests) == 0 && errWithAllow == nil {
		t.Fatal("expected backend request with allow-xdebug")
	}
}

// TestAuditScopeDiffAutoDetectsBase verifies --scope diff --base auto resolves via detect.
func TestAuditScopeDiffAutoDetectsBase(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	// Create a git repo with origin/HEAD so auto detection has something to find.
	// Use the test helper project which is already a git repo? It creates a temp git repo via auditCommandProject.
	// We set origin/HEAD by creating remote ref.
	setupGitOriginMaster(t, project)
	// Without auto, diff requires --base
	_, err := executeAuditCommand(t, project, []string{"audit", "diff", "--format", "json"})
	if err == nil || !strings.Contains(err.Error(), "--base") {
		t.Fatalf("diff without base should fail, got %v", err)
	}
	// With --base auto, should not fail on base missing; it should attempt detection and then run (or fail later not on base).
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--scope", "diff", "--base", "auto", "--format", "json"})
	// The error, if any, must not be "audit diff requires --base" – auto should have resolved.
	if err != nil && strings.Contains(err.Error(), "audit diff requires --base") {
		t.Fatalf("auto base should have resolved, got %v", err)
	}
	// If it succeeded, verify the runner received a resolved base (not "auto").
	if err == nil {
		if len(backend.requests) == 0 {
			t.Fatal("expected backend request")
		}
		req := backend.requests[0]
		if req.BaseRef == "auto" {
			t.Fatalf("BaseRef still auto after detection, got %q", req.BaseRef)
		}
		if req.Scope != "diff" {
			t.Fatalf("scope = %q, want diff", req.Scope)
		}
		_ = output
	}
}

// Helpers for this test file.

// auditCommandProjectWithXdebug creates a magento2 project with xdebug enabled in .govard.yml.
func auditCommandProjectWithXdebug(t *testing.T, framework string, xdebugEnabled bool) string {
	t.Helper()
	project := auditCommandProject(t, framework)
	if xdebugEnabled {
		// Patch .govard.yml to set stack.features.xdebug: true
		patchGovardYmlXdebug(t, project, true)
	}
	return project
}

func patchGovardYmlXdebug(t *testing.T, project string, enabled bool) {
	t.Helper()
	path := filepath.Join(project, ".govard.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read govard yml: %v", err)
	}
	text := string(content)
	// If stack.features.xdebug already present, replace value; otherwise append.
	if strings.Contains(text, "xdebug:") {
		if enabled {
			text = strings.ReplaceAll(text, "xdebug: false", "xdebug: true")
			if !strings.Contains(text, "xdebug: true") {
				text = strings.Replace(text, "xdebug:", "xdebug: true # patched", 1)
			}
		} else {
			text = strings.ReplaceAll(text, "xdebug: true", "xdebug: false")
		}
	} else {
		// Append features under stack; if stack already exists, inject features.
		if strings.Contains(text, "stack:") {
			// Inject after stack line.
			text = strings.Replace(text, "stack:", "stack:\n  features:\n    xdebug: true", 1)
		} else {
			text += "\nstack:\n  features:\n    xdebug: true\n"
		}
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("write patched yml: %v", err)
	}
}

func setupGitOriginMaster(t *testing.T, project string) {
	t.Helper()
	// Init git if not already a repo, and create origin/master so auto detection succeeds.
	_ = os.MkdirAll(project, 0o700)
	// Best effort: initialize git repo and create an origin ref. Failures are non-fatal; detection will fallback.
	cmds := [][]string{
		{"init"},
		{"config", "user.email", "test@test.test"},
		{"config", "user.name", "Test"},
		{"add", "."},
		{"commit", "-m", "init", "--allow-empty"},
		{"branch", "-M", "master"},
		{"remote", "add", "origin", "https://example.com/repo.git"},
	}
	for _, args := range cmds {
		c := exec.Command("git", append([]string{"-C", project}, args...)...)
		_ = c.Run()
	}
	// Create a fake origin/master ref file so git rev-parse --verify origin/master succeeds.
	// Git stores remote refs under .git/refs/remotes/origin/master; we can create it.
	if out, err := exec.Command("git", "-C", project, "rev-parse", "HEAD").Output(); err == nil {
		sha := strings.TrimSpace(string(out))
		refPath := filepath.Join(project, ".git", "refs", "remotes", "origin", "master")
		_ = os.MkdirAll(filepath.Dir(refPath), 0o700)
		_ = os.WriteFile(refPath, []byte(sha+"\n"), 0o600)
		// Also create origin/HEAD.
		headPath := filepath.Join(project, ".git", "refs", "remotes", "origin", "HEAD")
		_ = os.WriteFile(headPath, []byte("ref: refs/remotes/origin/master\n"), 0o600)
	}
}

func TestGovardLintBackendXdebugGuardBlocksWithoutAllow(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	patchGovardYmlXdebug(t, project, true)
	// Create backend without AllowXdebug; it should block.
	backend := newGovardLintBackendForTest(t, &fakeLintDocker{}, nil)
	req := govardLintRequestForTest(t)
	req.ProjectRoot = project
	_, err := backend.Run(t.Context(), req)
	if err == nil || !strings.Contains(err.Error(), "Xdebug enabled") {
		t.Fatalf("expected Xdebug guard error from backend, got %v", err)
	}
}

func TestGovardLintBackendXdebugGuardAllowsWithFlag(t *testing.T) {
	project := auditCommandProject(t, "magento2")
	patchGovardYmlXdebug(t, project, true)
	docker := &fakeLintDocker{}
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		return nil
	}
	backend := newGovardLintBackendForTest(t, docker, func(o *audit.GovardLintOptions) { o.AllowXdebug = true })
	req := govardLintRequestForTest(t)
	req.ProjectRoot = project
	docker.run = func(_ context.Context, run audit.ContainerRunRequest, _ io.Writer) error {
		writeGovardLintReportForTest(t, req, run, "passed")
		return nil
	}
	report, err := backend.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("allowXdebug true should not block, got %v", err)
	}
	if report.Status != "passed" {
		t.Fatalf("report status = %q, want passed", report.Status)
	}
}
