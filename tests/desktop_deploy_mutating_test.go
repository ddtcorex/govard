package tests

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/desktop"
	"govard/internal/engine"
)

// tapeDeployRunner is a rehearse double for the mutating deploy paths: unlike
// the read-only dry fixture it tolerates state-changing commands and answers
// every transport question from a script, so Preview/Apply run their real
// orchestration (lock, activate, verify, release) without touching a target.
type tapeDeployRunner struct {
	t              *testing.T
	mu             sync.Mutex
	commands       []string
	releasesPath   string
	currentPath    string
	liveName       string
	targetName     string
	targetRevision string
	records        map[string]string
	symlinkMode    bool
	// failRecordWrite makes release-record writes fail, so the restore path
	// of a failed apply is exercised instead of its happy path.
	failRecordWrite bool
}

func newTapeDeployRunner(t *testing.T, releasesPath, currentPath string, symlinkMode bool) *tapeDeployRunner {
	t.Helper()
	targetRevision := "def456abc123def456abc123def456abc123abcd"
	return &tapeDeployRunner{
		t:              t,
		releasesPath:   releasesPath,
		currentPath:    currentPath,
		liveName:       "3",
		targetName:     "2",
		targetRevision: targetRevision,
		records: map[string]string{
			"3": releaseRecordJSON("3", "abc123def456abc123def456abc123def456abcd"),
			"2": releaseRecordJSON("2", targetRevision),
			"1": releaseRecordJSON("1", "123abc123abc123abc123abc123abc123abc123a"),
		},
		symlinkMode: symlinkMode,
	}
}

func (r *tapeDeployRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	r.mu.Lock()
	r.commands = append(r.commands, command)
	r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return deploy.Result{}, err
	}
	switch {
	case strings.HasPrefix(command, "ls -1 "):
		return deploy.Result{Stdout: "3\n2\n1\n"}, nil
	case strings.HasPrefix(command, "cat > "):
		// Writes (lock owner, release records) land unless the test asked
		// for a failing record write.
		if r.failRecordWrite && strings.Contains(command, "release.json") {
			return deploy.Result{}, errors.New("disk full on the tape")
		}
		return deploy.Result{}, nil
	case strings.HasPrefix(command, "cat "):
		if !strings.Contains(command, r.releasesPath) {
			return deploy.Result{}, errors.New("no such record on the tape")
		}
		for name, record := range r.records {
			if strings.Contains(command, "/"+name+"/") || strings.Contains(command, "/"+name+"\"") || strings.HasSuffix(strings.TrimSpace(command), "/"+name+"'") {
				return deploy.Result{Stdout: record}, nil
			}
		}
		if record, ok := r.records[r.liveName]; ok && strings.Contains(command, r.liveName) {
			return deploy.Result{Stdout: record}, nil
		}
		return deploy.Result{}, errors.New("no such record on the tape")
	case strings.Count(command, "readlink -f") >= 2:
		// The symlink verify prints current and release side by side: the
		// rehearse answers both as the target, which is what a swapped
		// current would resolve to.
		want := r.releasesPath + "/" + r.targetName + "\n"
		return deploy.Result{Stdout: want + want}, nil
	case strings.HasPrefix(command, "readlink -f "):
		return deploy.Result{Stdout: r.releasesPath + "/" + r.liveName + "\n"}, nil
	case strings.HasPrefix(command, "test -L "):
		if r.symlinkMode {
			return deploy.Result{}, nil
		}
		return deploy.Result{}, errors.New("not a symlink on the tape")
	case strings.HasPrefix(command, "test -e ") && strings.Contains(command, "deploy.lock"):
		return deploy.Result{}, errors.New("no deployer lock on the tape")
	case strings.HasPrefix(command, "test -e ") && strings.Contains(command, r.currentPath):
		return deploy.Result{}, nil
	case strings.HasPrefix(command, "test -e "):
		return deploy.Result{}, errors.New("absent on the tape")
	case strings.HasPrefix(command, "git -C ") && strings.Contains(command, "rev-parse HEAD"):
		return deploy.Result{Stdout: r.targetRevision + "\n"}, nil
	default:
		return deploy.Result{}, nil
	}
}

func (r *tapeDeployRunner) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.commands...)
}

func (r *tapeDeployRunner) saw(fragment string) bool {
	for _, command := range r.recorded() {
		if strings.Contains(command, fragment) {
			return true
		}
	}
	return false
}

// stubMutatingHost resolves any remote to a host bound to the tape runner.
func stubMutatingHost(t *testing.T, runner deploy.Runner, deployPath, currentPath string) {
	t.Helper()
	restore := desktop.SetDeployHostResolverForTest(func(cfg engine.Config, remote string, opts deploy.Options) (deploy.Host, error) {
		host := deploy.HostForTest(deployPath, runner)
		host.Name = strings.ToLower(strings.TrimSpace(remote))
		host.CurrentPath = currentPath
		host.Repository = opts.Repository
		return host, nil
	})
	t.Cleanup(restore)
}

func releasesFixturePaths(t *testing.T) (root, deployPath, currentPath, releasesPath string) {
	t.Helper()
	root, deployPath, currentPath = writeDeployReadonlyFixture(t)
	return root, deployPath, currentPath, deployPath + "/releases"
}

// TestMutatingApplyRollbackWithoutTokenIsRefused pins the first safety
// property: no preview token means no rollback, even with confirm set.
func TestMutatingApplyRollbackWithoutTokenIsRefused(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)

	if _, err := desktop.NewDeployService().ApplyRollback(root, "", true); err == nil {
		t.Fatal("ApplyRollback without a preview token must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("the refusal must name the missing preview, got: %v", err)
	}
	if len(tape.recorded()) != 0 {
		t.Fatalf("a refused apply must touch no transport, saw %q", tape.recorded())
	}
}

// TestMutatingSandboxCallsWithoutTokenAreRefused pins the same gate on the
// sandbox paths: up and down need a preview token too, no exceptions.
func TestMutatingSandboxCallsWithoutTokenAreRefused(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := absentContainerFake()
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)

	svc := desktop.NewDeployService()
	if _, err := svc.SandboxUp(root, "", true); err == nil {
		t.Fatal("SandboxUp without a preview token must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("the refusal must name the missing preview, got: %v", err)
	}
	if _, err := svc.SandboxDown(root, "", true); err == nil {
		t.Fatal("SandboxDown without a preview token must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("the refusal must name the missing preview, got: %v", err)
	}
	if fake.has("run --detach") {
		t.Fatal("a refused sandbox up must create no container")
	}
}

// TestMutatingApplyRollbackNeedsExplicitConfirm pins the second half of the
// gate: a valid token alone is not enough, the confirm flag must be set.
func TestMutatingApplyRollbackNeedsExplicitConfirm(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	if strings.TrimSpace(preview.Token) == "" {
		t.Fatal("a preview must mint a token")
	}
	if _, err := svc.ApplyRollback(root, preview.Token, false); err == nil {
		t.Fatal("ApplyRollback without explicit confirm must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "confirm") {
		t.Fatalf("the refusal must name the missing confirmation, got: %v", err)
	}
}

// TestMutatingForeignTokenIsRefused pins token binding: a token minted for
// one action cannot be spent on another.
func TestMutatingForeignTokenIsRefused(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := absentContainerFake()
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)
	restoreProbe := desktop.SetDeploySandboxProbeForTest(func(context.Context, string, int, time.Duration) error {
		return nil
	})
	t.Cleanup(restoreProbe)

	svc := desktop.NewDeployService()
	upPreview, err := svc.PreviewSandboxUp(root, "", false)
	if err != nil {
		t.Fatalf("PreviewSandboxUp: %v", err)
	}
	if _, err := svc.ApplyRollback(root, upPreview.Token, true); err == nil {
		t.Fatal("a sandbox token spent on a rollback must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("the refusal must name the token mismatch, got: %v", err)
	}
}

// TestMutatingRollbackPreviewThenApplySymlink runs the full two-step flow
// against the tape: preview names release 2, apply swaps to it.
func TestMutatingRollbackPreviewThenApplySymlink(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	if preview.Target.Release != "2" {
		t.Fatalf("preview must name release 2 (before live 3), got %#v", preview.Target)
	}
	if preview.Previous != "3" {
		t.Fatalf("preview must name the live release it replaces, got %#v", preview)
	}
	if preview.Strategy != deploy.PublishSymlink {
		t.Fatalf("want the symlink strategy, got %#v", preview)
	}

	result, err := svc.ApplyRollback(root, preview.Token, true)
	if err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}
	if result.Cancelled {
		t.Fatal("a completed apply must not read cancelled")
	}
	if result.Release != "2" {
		t.Fatalf("apply must serve release 2, got %#v", result)
	}
	if !tape.saw("mv -T") && !tape.saw("ln -sfn") {
		t.Fatalf("a symlink apply must re-point current, saw %q", tape.recorded())
	}
	if !tape.saw("rm -rf") {
		t.Fatalf("a completed apply must release the deploy lock, saw %q", tape.recorded())
	}
}

// TestMutatingStaleTokenReuseIsRefused pins single-use tokens: the second
// click needs a fresh preview, per the plan review focus.
func TestMutatingStaleTokenReuseIsRefused(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	if _, err := svc.ApplyRollback(root, preview.Token, true); err != nil {
		t.Fatalf("first ApplyRollback: %v", err)
	}
	if _, err := svc.ApplyRollback(root, preview.Token, true); err == nil {
		t.Fatal("reusing a spent preview token must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("the reuse refusal must say a fresh preview is needed, got: %v", err)
	}
}

// TestMutatingRollbackAppliesTheInPlaceTail covers the second strategy: the
// tail rewrites the docroot instead of swapping a link.
func TestMutatingRollbackAppliesTheInPlaceTail(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, false)
	stubMutatingHost(t, tape, deployPath, currentPath)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	if preview.Strategy != deploy.PublishInPlace {
		t.Fatalf("want the in-place strategy, got %#v", preview)
	}
	result, err := svc.ApplyRollback(root, preview.Token, true)
	if err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}
	if result.Release != "2" {
		t.Fatalf("apply must serve release 2, got %#v", result)
	}
	if !tape.saw("reset --hard") {
		t.Fatalf("an in-place apply must rewrite the docroot, saw %q", tape.recorded())
	}
}

// TestMutatingCancelMidRollbackCleansUp pins honest cancel: the op is
// registered while it runs, CancelOp stops it, the lock is released, and
// the result says cancelled instead of failed.
func TestMutatingCancelMidRollbackCleansUp(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)
	entered := make(chan struct{})
	var once sync.Once
	restoreStep := desktop.SetDeployRunStepForTest(func(ctx context.Context, host deploy.Host, opts deploy.Options, vars deploy.Vars, step deploy.Step, release *deploy.Release, out io.Writer) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	})
	t.Cleanup(restoreStep)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}

	type outcome struct {
		result desktop.RollbackResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.ApplyRollback(root, preview.Token, true)
		done <- outcome{result: result, err: err}
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the apply never reached its first step")
	}
	var opID string
	deadline := time.Now().Add(10 * time.Second)
	for opID == "" && time.Now().Before(deadline) {
		for _, op := range desktop.ListOps() {
			if op.Kind == "deploy-rollback" {
				opID = op.ID
			}
		}
		if opID == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if opID == "" {
		t.Fatal("the running rollback must be registered as a cancellable op")
	}
	if !desktop.CancelOp(opID) {
		t.Fatal("CancelOp must accept the running rollback")
	}

	var out outcome
	select {
	case out = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled apply never returned")
	}
	if out.err != nil {
		t.Fatalf("a cancelled apply reports through its result, got error: %v", out.err)
	}
	if !out.result.Cancelled {
		t.Fatalf("a cancelled apply must read cancelled, got %#v", out.result)
	}
	if !strings.Contains(strings.ToLower(out.result.Message), "cancel") {
		t.Fatalf("a cancelled apply must say so honestly, got %#v", out.result)
	}
	if !tape.saw("rm -rf") {
		t.Fatalf("a cancelled apply must still release the deploy lock, saw %q", tape.recorded())
	}
	for _, op := range desktop.ListOps() {
		if op.Kind == "deploy-rollback" {
			t.Fatalf("a finished apply must leave no op behind, still running: %#v", op)
		}
	}
}

// TestMutatingSandboxConflictingProfileRefusesWithRecreateHint pins the
// never-silent-reshape rule: a profile that disagrees with the existing
// container is refused, and the refusal names --recreate.
func TestMutatingSandboxConflictingProfileRefusesWithRecreateHint(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile(deploy.SandboxProfileFull)
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)

	svc := desktop.NewDeployService()
	_, err := svc.PreviewSandboxUp(root, deploy.SandboxProfilePHP, false)
	if err == nil {
		t.Fatal("a conflicting profile must be refused, never silently reshaped")
	}
	if !strings.Contains(err.Error(), "--recreate") {
		t.Fatalf("the refusal must name --recreate, got: %v", err)
	}
	if fake.has("run --detach") || fake.has("build --file") {
		t.Fatal("a refused preview must build and create nothing")
	}
}

// TestMutatingSandboxUpRefusesWhenProfileChangedSincePreview pins the
// apply-time re-check: a container built for another profile after the
// preview passed must be refused with the recreate hint, not reshaped.
// It mirrors the identity re-check SandboxDown already runs at apply.
func TestMutatingSandboxUpRefusesWhenProfileChangedSincePreview(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := absentContainerFake()
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)
	restoreProbe := desktop.SetDeploySandboxProbeForTest(func(context.Context, string, int, time.Duration) error {
		return nil
	})
	t.Cleanup(restoreProbe)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewSandboxUp(root, deploy.SandboxProfilePHP, false)
	if err != nil {
		t.Fatalf("PreviewSandboxUp: %v", err)
	}
	// Another profile lands between the preview and the apply.
	delete(fake.fail, "inspect")
	fake.containerProfile(deploy.SandboxProfileFull)

	if _, err := svc.SandboxUp(root, preview.Token, true); err == nil {
		t.Fatal("applying against a container built for another profile must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview again") {
		t.Fatalf("the apply-time re-check must refuse with a fresh-preview hint (not the engine backstop), got: %v", err)
	}
	if fake.has("run --detach") || fake.has("build --file") {
		t.Fatal("a refused creation must build and create nothing")
	}
}

// TestMutatingSandboxUpPreviewThenApply runs the rehearse two-step flow
// against the fake runtime: preview mints the token, apply creates.
func TestMutatingSandboxUpPreviewThenApply(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := absentContainerFake()
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)
	restoreProbe := desktop.SetDeploySandboxProbeForTest(func(context.Context, string, int, time.Duration) error {
		return nil
	})
	t.Cleanup(restoreProbe)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewSandboxUp(root, deploy.SandboxProfilePHP, false)
	if err != nil {
		t.Fatalf("PreviewSandboxUp: %v", err)
	}
	if strings.TrimSpace(preview.Token) == "" {
		t.Fatal("an up preview must mint a token")
	}
	if fake.has("run --detach") {
		t.Fatal("a preview must create no container")
	}

	result, err := svc.SandboxUp(root, preview.Token, true)
	if err != nil {
		t.Fatalf("SandboxUp: %v", err)
	}
	if !result.Running {
		t.Fatalf("the rehearsed sandbox must be running, got %#v", result)
	}
	if result.Port != 49153 {
		t.Fatalf("port = %d, want the published port 49153", result.Port)
	}
	if !fake.has("run --detach") {
		t.Fatal("an applied up must create the container")
	}
	if _, err := svc.SandboxUp(root, preview.Token, true); err == nil {
		t.Fatal("reusing a spent sandbox token must be refused")
	}
}

// TestMutatingSandboxDownPreviewThenApply removes the rehearsed container
// through the same two-step gate.
func TestMutatingSandboxDownPreviewThenApply(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile(deploy.SandboxProfilePHP)
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewSandboxDown(root, false, false)
	if err != nil {
		t.Fatalf("PreviewSandboxDown: %v", err)
	}
	if strings.TrimSpace(preview.Token) == "" {
		t.Fatal("a down preview must mint a token")
	}
	if strings.TrimSpace(preview.Container) == "" {
		t.Fatalf("a down preview must name the container it removes, got %#v", preview)
	}

	result, err := svc.SandboxDown(root, preview.Token, true)
	if err != nil {
		t.Fatalf("SandboxDown: %v", err)
	}
	if result.Exists {
		t.Fatalf("the sandbox must be gone after down, got %#v", result)
	}
	if !fake.has("rm --force") && !fake.has("rm ") {
		t.Fatalf("a down must remove the container, saw %v", fake.calls)
	}
}

// TestMutatingSandboxSSHInfoOpensViaTerminalPath pins the human ruling: the
// info carries the ssh command for the existing terminal path, and no key
// material ever appears in any output.
func TestMutatingSandboxSSHInfoOpensViaTerminalPath(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile(deploy.SandboxProfilePHP)
	fake.answers["port "] = "127.0.0.1:49153\n"
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)

	svc := desktop.NewDeployService()
	info, err := svc.SandboxSSHinfo(root)
	if err != nil {
		t.Fatalf("SandboxSSHinfo: %v", err)
	}
	if info.Port != 49153 {
		t.Fatalf("port = %d, want the published port 49153", info.Port)
	}
	if !strings.Contains(info.Command, "deployer@127.0.0.1") {
		t.Fatalf("the ssh command must target the sandbox user, got %q", info.Command)
	}
	if !strings.Contains(info.Command, "-i") {
		t.Fatalf("the ssh command must reuse the sandbox key path, got %q", info.Command)
	}
	if !strings.Contains(info.Command, filepath.Join(".govard", "sandbox", "id_ed25519")) {
		t.Fatalf("the ssh command must point at the sandbox key, got %q", info.Command)
	}
	for _, output := range []string{info.Command, info.KeyPath, info.User} {
		if strings.Contains(output, "PRIVATE KEY") {
			t.Fatalf("key material must never appear in outputs, got %q", output)
		}
	}

	var launched []string
	restoreLaunch := desktop.SetLaunchInTerminalForTest(func(workingDir, command string) error {
		launched = []string{workingDir, command}
		return nil
	})
	t.Cleanup(restoreLaunch)
	message, err := svc.OpenSandboxSSHInTerminal(root)
	if err != nil {
		t.Fatalf("OpenSandboxSSHInTerminal: %v", err)
	}
	if strings.TrimSpace(message) == "" {
		t.Fatal("opening the terminal must acknowledge the launch")
	}
	if len(launched) != 2 || !strings.Contains(launched[1], "ssh") {
		t.Fatalf("the session must open through the existing terminal path, launched %q", launched)
	}
	if strings.Contains(launched[1], "PRIVATE KEY") {
		t.Fatalf("the terminal command must carry the key path, never key material: %q", launched[1])
	}
}

// TestMutatingCancelMessagesAreHonest pins the shared cancel shape for the
// new ops: no failure title, no retry prompt for a deliberate cancel.
func TestMutatingCancelMessagesAreHonest(t *testing.T) {
	for op, headline := range map[string]string{
		"rollback":     "Rollback was cancelled",
		"sandbox-up":   "Sandbox start was cancelled",
		"sandbox-down": "Sandbox removal was cancelled",
	} {
		message := desktop.HumanOpMessageForTest(op, context.Canceled)
		if message.Title != headline {
			t.Fatalf("HumanOpMessage(%q, canceled) title = %q, want %q", op, message.Title, headline)
		}
		for _, action := range message.Actions {
			if action.ID == "retry" {
				t.Fatalf("HumanOpMessage(%q, canceled) must not prompt retry, got %+v", op, message.Actions)
			}
		}
	}
}

// TestMutatingPreviewTokenExpires pins the token TTL: a preview that outlives
// it is refused, and the refusal says the token expired.
func TestMutatingPreviewTokenExpires(t *testing.T) {
	desktop.ResetStateForTest()
	restoreTTL := desktop.SetDeployPreviewTTLForTest(time.Nanosecond)
	t.Cleanup(restoreTTL)
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	if _, err := svc.ApplyRollback(root, preview.Token, true); err == nil {
		t.Fatal("an expired preview token must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "expir") {
		t.Fatalf("the refusal must say the token expired, got: %v", err)
	}
}

// TestMutatingPreviewTokenMapStaysBounded pins eviction: minting past the cap
// drops the oldest entries, and the map never grows past its bound.
func TestMutatingPreviewTokenMapStaysBounded(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)

	svc := desktop.NewDeployService()
	first, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	for i := 0; i < 150; i++ {
		if _, err := svc.PreviewRollback(root, "fixture", ""); err != nil {
			t.Fatalf("PreviewRollback %d: %v", i, err)
		}
	}
	if count := desktop.DeployPreviewTokenCountForTest(); count > 128 {
		t.Fatalf("the token map holds %d entries, want it bounded at 128", count)
	}
	if _, err := svc.ApplyRollback(root, first.Token, true); err == nil {
		t.Fatal("the oldest token must have been evicted")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("the eviction refusal must name the preview, got: %v", err)
	}
}

// TestMutatingPreviewMintRandFailureReturnsError pins the hard stop without
// a panic: when randomness fails, previews fail with an error.
func TestMutatingPreviewMintRandFailureReturnsError(t *testing.T) {
	desktop.ResetStateForTest()
	restoreRand := desktop.SetDeployPreviewRandForTest(func([]byte) (int, error) {
		return 0, errors.New("no entropy on this machine")
	})
	t.Cleanup(restoreRand)
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("a randomness failure must return an error, not panic: %v", recovered)
		}
	}()
	if _, err := desktop.NewDeployService().PreviewRollback(root, "fixture", ""); err == nil {
		t.Fatal("a preview without randomness must fail")
	} else if !strings.Contains(strings.ToLower(err.Error()), "token") {
		t.Fatalf("the failure must name the token, got: %v", err)
	}
}

// TestDeployVarsSharedHelperCarriesTheCLIMapping pins the contract the
// desktop relies on from the shared helper: paths, release facts, command
// words quoted one by one, fragments verbatim, and the deterministic
// content version.
func TestDeployVarsSharedHelperCarriesTheCLIMapping(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), stubDeployRunner{run: func(context.Context, string, deploy.RunOptions) (deploy.Result, error) {
		return deploy.Result{}, nil
	}})
	options := deploy.Options{
		Branch:   "main",
		Revision: "abc123def456abc123def456abc123def456abcd",
		Settings: map[string]any{
			"php_bin":         "php -d memory_limit=-1",
			"deploy_cmd":      "deploy --force",
			"deploy_args":     "--skip-check",
			"content_version": "",
		},
		RawSettings: map[string]bool{"deploy_cmd": true},
	}
	vars := deploy.VarsFor(host, options)

	expanded, err := vars.Expand("{{deploy_path}} {{current_path}} {{branch}} {{revision}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !strings.Contains(expanded, host.DeployPath) || !strings.Contains(expanded, "main") {
		t.Fatalf("paths and branch must expand, got %q", expanded)
	}
	words, err := vars.Expand("{{php_bin}} -v")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if words != "'php' '-d' 'memory_limit=-1' -v" {
		t.Fatalf("php_bin expanded to %q, want the words quoted one by one", words)
	}
	fragment, err := vars.Expand("{{settings.deploy_cmd}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if fragment != "deploy --force" {
		t.Fatalf("a shell fragment must reach the command verbatim, got %q", fragment)
	}
	emptyFragment, err := vars.Expand("{{settings.deploy_args}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if emptyFragment != "--skip-check" {
		t.Fatalf("an argument list must reach the command verbatim, got %q", emptyFragment)
	}
	version, err := vars.Expand("{{settings.content_version}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if version != "'abc123de'" {
		t.Fatalf("an empty content version must default to the short revision, got %q", version)
	}
}

// blockingSandboxRuntime wraps a real runtime and stalls one method until
// its context dies, so a cancel lands mid-flight deterministically. It
// stalls the first call only: the cancel-path status re-read must pass
// through instead of hanging on the same block.
type blockingSandboxRuntime struct {
	deploy.SandboxRuntime
	entered chan struct{}
	once    sync.Once
	mu      sync.Mutex
	blocked bool
	block   string
}

func (b *blockingSandboxRuntime) signal() {
	b.once.Do(func() { close(b.entered) })
}

func (b *blockingSandboxRuntime) stall() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.blocked {
		return false
	}
	b.blocked = true
	return true
}

func (b *blockingSandboxRuntime) RunContainer(ctx context.Context, request deploy.SandboxRunRequest) error {
	if b.block == "RunContainer" && b.stall() {
		b.signal()
		<-ctx.Done()
		return ctx.Err()
	}
	return b.SandboxRuntime.RunContainer(ctx, request)
}

func (b *blockingSandboxRuntime) PublishedPort(ctx context.Context, name string, port int) (int, error) {
	if b.block == "PublishedPort" && b.stall() {
		b.signal()
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return b.SandboxRuntime.PublishedPort(ctx, name, port)
}

func (b *blockingSandboxRuntime) StopContainer(ctx context.Context, name string) error {
	if b.block == "StopContainer" && b.stall() {
		b.signal()
		<-ctx.Done()
		return ctx.Err()
	}
	return b.SandboxRuntime.StopContainer(ctx, name)
}

func waitForSandboxOp(t *testing.T, kind string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, op := range desktop.ListOps() {
			if op.Kind == kind {
				return op.ID
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the running op never registered")
	return ""
}

// TestMutatingCancelMidInPlaceRollbackRestoresAndReports pins truthful
// cancel on the heaviest path: the stop still attempts the record restore
// and the maintenance close, and the message says exactly what remains.
func TestMutatingCancelMidInPlaceRollbackRestoresAndReports(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, false)
	stubMutatingHost(t, tape, deployPath, currentPath)
	var mu sync.Mutex
	var stepped []string
	restoreStep := desktop.SetDeployRunStepForTest(func(ctx context.Context, host deploy.Host, opts deploy.Options, vars deploy.Vars, step deploy.Step, release *deploy.Release, out io.Writer) error {
		mu.Lock()
		stepped = append(stepped, step.ID)
		mu.Unlock()
		return nil
	})
	t.Cleanup(restoreStep)
	entered := make(chan struct{})
	var once sync.Once
	restoreExec := desktop.SetDeployExecutorRunForTest(func(ctx context.Context, host deploy.Host, opts deploy.Options, out io.Writer, plan deploy.Plan, vars deploy.Vars, release *deploy.Release) (deploy.Outcome, error) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return deploy.Outcome{}, ctx.Err()
	})
	t.Cleanup(restoreExec)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	type outcome struct {
		result desktop.RollbackResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.ApplyRollback(root, preview.Token, true)
		done <- outcome{result: result, err: err}
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the apply never reached its tail")
	}
	opID := waitForSandboxOp(t, "deploy-rollback")
	if !desktop.CancelOp(opID) {
		t.Fatal("CancelOp must accept the running rollback")
	}
	var out outcome
	select {
	case out = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled apply never returned")
	}
	if out.err != nil {
		t.Fatalf("a cancelled apply reports through its result, got error: %v", out.err)
	}
	if !out.result.Cancelled {
		t.Fatalf("a cancelled apply must read cancelled, got %#v", out.result)
	}
	if !tape.saw("release.json") {
		t.Fatalf("a stopped tail must still attempt the record restore, saw %q", tape.recorded())
	}
	mu.Lock()
	sawDisable := false
	for _, id := range stepped {
		if id == deploy.TaskMaintenanceDisable {
			sawDisable = true
		}
	}
	mu.Unlock()
	if !sawDisable {
		t.Fatalf("a stopped tail must still attempt the maintenance close, stepped %q", stepped)
	}
	for _, want := range []string{"cancel", "record", "maintenance"} {
		if !strings.Contains(strings.ToLower(out.result.Message), want) {
			t.Fatalf("the cancel message must report the remaining state (%q), got %q", want, out.result.Message)
		}
	}
}

// TestMutatingFailedInPlaceRollbackReportsRemainingState pins the same
// honesty on the error path: when the restore and the close both fail, the
// error says so instead of reporting only the tail failure.
func TestMutatingFailedInPlaceRollbackReportsRemainingState(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, false)
	tape.failRecordWrite = true
	stubMutatingHost(t, tape, deployPath, currentPath)
	restoreStep := desktop.SetDeployRunStepForTest(func(ctx context.Context, host deploy.Host, opts deploy.Options, vars deploy.Vars, step deploy.Step, release *deploy.Release, out io.Writer) error {
		if step.ID == deploy.TaskMaintenanceDisable {
			return errors.New("the target went away")
		}
		return nil
	})
	t.Cleanup(restoreStep)
	restoreExec := desktop.SetDeployExecutorRunForTest(func(ctx context.Context, host deploy.Host, opts deploy.Options, out io.Writer, plan deploy.Plan, vars deploy.Vars, release *deploy.Release) (deploy.Outcome, error) {
		return deploy.Outcome{}, errors.New("the tail failed halfway")
	})
	t.Cleanup(restoreExec)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	_, err = svc.ApplyRollback(root, preview.Token, true)
	if err == nil {
		t.Fatal("a failed tail must fail the apply")
	}
	for _, want := range []string{"tail failed halfway", "record", "maintenance"} {
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
			t.Fatalf("the error must report the remaining state (%q), got: %v", want, err)
		}
	}
}

// TestMutatingCancelMidSandboxUpReportsContainerState pins the up-cancel
// message in both shapes: nothing left behind on a fresh creation, and the
// named remaining container when one exists.
func TestMutatingCancelMidSandboxUpReportsContainerState(t *testing.T) {
	desktop.ResetStateForTest()
	restoreProbe := desktop.SetDeploySandboxProbeForTest(func(context.Context, string, int, time.Duration) error {
		return nil
	})
	t.Cleanup(restoreProbe)

	t.Run("fresh", func(t *testing.T) {
		desktop.ResetStateForTest()
		restoreProbe := desktop.SetDeploySandboxProbeForTest(func(context.Context, string, int, time.Duration) error {
			return nil
		})
		t.Cleanup(restoreProbe)
		root := sandboxProject(t)
		fake := absentContainerFake()
		restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
			return deploy.NewDockerCLIForTest(fake.run)
		})
		t.Cleanup(restoreRuntime)

		svc := desktop.NewDeployService()
		preview, err := svc.PreviewSandboxUp(root, deploy.SandboxProfilePHP, false)
		if err != nil {
			t.Fatalf("PreviewSandboxUp: %v", err)
		}
		// The blocker goes in after the preview: the preview's own status
		// read must not consume the one stall.
		blocker := &blockingSandboxRuntime{
			SandboxRuntime: deploy.NewDockerCLIForTest(fake.run),
			entered:        make(chan struct{}),
			block:          "RunContainer",
		}
		restoreRuntime()
		restoreBlocked := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
			return blocker
		})
		t.Cleanup(restoreBlocked)
		type outcome struct {
			result desktop.SandboxUpResult
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, err := svc.SandboxUp(root, preview.Token, true)
			done <- outcome{result: result, err: err}
		}()
		select {
		case <-blocker.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the creation never reached the container")
		}
		opID := waitForSandboxOp(t, "sandbox-up")
		if !desktop.CancelOp(opID) {
			t.Fatal("CancelOp must accept the running creation")
		}
		var out outcome
		select {
		case out = <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the cancelled creation never returned")
		}
		if out.err != nil {
			t.Fatalf("a cancelled creation reports through its result, got error: %v", out.err)
		}
		if !out.result.Cancelled {
			t.Fatalf("a cancelled creation must read cancelled, got %#v", out.result)
		}
		if !strings.Contains(strings.ToLower(out.result.Message), "no container") {
			t.Fatalf("stopping before creation must say no container remains, got %q", out.result.Message)
		}
	})

	t.Run("existing", func(t *testing.T) {
		desktop.ResetStateForTest()
		restoreProbe := desktop.SetDeploySandboxProbeForTest(func(context.Context, string, int, time.Duration) error {
			return nil
		})
		t.Cleanup(restoreProbe)
		root := sandboxProject(t)
		fake := sandboxFake().containerProfile(deploy.SandboxProfilePHP)
		restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
			return deploy.NewDockerCLIForTest(fake.run)
		})
		t.Cleanup(restoreRuntime)

		svc := desktop.NewDeployService()
		preview, err := svc.PreviewSandboxUp(root, deploy.SandboxProfilePHP, false)
		if err != nil {
			t.Fatalf("PreviewSandboxUp: %v", err)
		}
		blocker := &blockingSandboxRuntime{
			SandboxRuntime: deploy.NewDockerCLIForTest(fake.run),
			entered:        make(chan struct{}),
			block:          "PublishedPort",
		}
		restoreRuntime()
		restoreBlocked := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
			return blocker
		})
		t.Cleanup(restoreBlocked)
		type outcome struct {
			result desktop.SandboxUpResult
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, err := svc.SandboxUp(root, preview.Token, true)
			done <- outcome{result: result, err: err}
		}()
		select {
		case <-blocker.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the creation never reached the port read")
		}
		opID := waitForSandboxOp(t, "sandbox-up")
		if !desktop.CancelOp(opID) {
			t.Fatal("CancelOp must accept the running creation")
		}
		var out outcome
		select {
		case out = <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the cancelled creation never returned")
		}
		if out.err != nil {
			t.Fatalf("a cancelled creation reports through its result, got error: %v", out.err)
		}
		if !out.result.Cancelled {
			t.Fatalf("a cancelled creation must read cancelled, got %#v", out.result)
		}
		if !strings.Contains(out.result.Message, "govard-sample-project-sandbox") {
			t.Fatalf("stopping with a container present must name it, got %q", out.result.Message)
		}
		if !strings.Contains(strings.ToLower(out.result.Message), "running") {
			t.Fatalf("stopping with a container present must report its state, got %q", out.result.Message)
		}
	})
}

// TestMutatingSandboxDownRefusesWhenContainerChanged pins the identity
// revalidation: a project renamed between preview and apply removes
// nothing, and the refusal asks for a fresh preview.
func TestMutatingSandboxDownRefusesWhenContainerChanged(t *testing.T) {
	desktop.ResetStateForTest()
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile(deploy.SandboxProfilePHP)
	restoreRuntime := desktop.SetDeploySandboxRuntimeForTest(func() deploy.SandboxRuntime {
		return deploy.NewDockerCLIForTest(fake.run)
	})
	t.Cleanup(restoreRuntime)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewSandboxDown(root, false, false)
	if err != nil {
		t.Fatalf("PreviewSandboxDown: %v", err)
	}
	configPath := filepath.Join(root, ".govard.yml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read fixture config: %v", err)
	}
	renamed := strings.Replace(string(raw), "sample-project", "renamed-project", -1)
	if err := os.WriteFile(configPath, []byte(renamed), 0o644); err != nil {
		t.Fatalf("rename the fixture project: %v", err)
	}
	if _, err := svc.SandboxDown(root, preview.Token, true); err == nil {
		t.Fatal("applying against a renamed project must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("the refusal must ask for a fresh preview, got: %v", err)
	}
	if fake.has("stop ") || fake.has("rm --force") {
		t.Fatal("a refused removal must stop and remove nothing")
	}
}

// TestMutatingRollbackSymlinkFlushesCachesBeforeVerify pins the CLI parity
// the desktop path missed: a symlink rollback must run the recipe's
// app:cache:flush between activate and verify, or the GUI reports success
// while the target serves stale caches.
func TestMutatingRollbackSymlinkFlushesCachesBeforeVerify(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath, releasesPath := releasesFixturePaths(t)
	tape := newTapeDeployRunner(t, releasesPath, currentPath, true)
	stubMutatingHost(t, tape, deployPath, currentPath)
	var mu sync.Mutex
	var ids []string
	restoreStep := desktop.SetDeployRunStepForTest(func(ctx context.Context, host deploy.Host, opts deploy.Options, vars deploy.Vars, step deploy.Step, release *deploy.Release, out io.Writer) error {
		mu.Lock()
		ids = append(ids, step.ID)
		mu.Unlock()
		return nil
	})
	t.Cleanup(restoreStep)

	svc := desktop.NewDeployService()
	preview, err := svc.PreviewRollback(root, "fixture", "")
	if err != nil {
		t.Fatalf("PreviewRollback: %v", err)
	}
	if _, err := svc.ApplyRollback(root, preview.Token, true); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}
	mu.Lock()
	got := append([]string{}, ids...)
	mu.Unlock()
	want := []string{deploy.TaskActivate, deploy.TaskAppCacheFlush, deploy.TaskVerify}
	if len(got) != len(want) {
		t.Fatalf("a symlink apply must run %v, ran %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("a symlink apply must run %v in order, ran %v", want, got)
		}
	}
}
