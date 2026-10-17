package tests

import (
	"context"
	"errors"
	"fmt"
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

// dryDeployRunner is a read-only fixture transport: it serves canned answers
// for the commands the read paths issue and fails the test on any command
// that would change state, proving no M1 method mutates the target.
type dryDeployRunner struct {
	t            *testing.T
	mu           sync.Mutex
	commands     []string
	releasesPath string
	currentPath  string
	liveName     string
	records      map[string]string
}

func releaseRecordJSON(release, revision string) string {
	return fmt.Sprintf(
		`{"schema_version":1,"tool":"govard","release":%q,"revision":%q,"branch":"main",`+
			`"created_at":"2026-10-17T10:00:00Z","created_by":"tester",`+
			`"build":{"mode":"server"},"publish":{"strategy":"in_place"},`+
			`"verify":{"status":"ok"},"status":"ok"}`,
		release, revision,
	)
}

func newDryDeployRunner(t *testing.T, releasesPath, currentPath string) *dryDeployRunner {
	t.Helper()
	rev := "abc123def456abc123def456abc123def456abcd"
	return &dryDeployRunner{
		t:            t,
		releasesPath: releasesPath,
		currentPath:  currentPath,
		liveName:     "3",
		records: map[string]string{
			"3": releaseRecordJSON("3", rev),
			"2": releaseRecordJSON("2", "def456abc123def456abc123def456abc123abcd"),
			"1": releaseRecordJSON("1", "123abc123abc123abc123abc123abc123abc123a"),
		},
	}
}

// mutatingCommandFragments are shell shapes the dry fixture never issues: it
// resolves the in-place strategy, so the atomic-rename probe never runs there.
// The symlink test below runs the real probe against a real directory instead
// and asserts full cleanup, which is the case this list cannot cover.
var mutatingCommandFragments = []string{
	"mkdir", "rmdir", "rm -f", "rm -rf", "ln -s", "mv -T", "chmod", "chown",
	"rsync", "scp ", "git clone", "git push", "composer install", "npm ci",
	"systemctl", "docker exec",
}

func (r *dryDeployRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	r.mu.Lock()
	r.commands = append(r.commands, command)
	r.mu.Unlock()
	for _, banned := range mutatingCommandFragments {
		if strings.Contains(command, banned) {
			r.t.Errorf("read-only deploy path issued mutating command %q (matched %q)", command, banned)
			return deploy.Result{}, errors.New("mutating command refused by dry fixture")
		}
	}
	switch {
	case strings.HasPrefix(command, "ls -1 "):
		return deploy.Result{Stdout: "3\n2\n1\n"}, nil
	case strings.HasPrefix(command, "cat "):
		for name, record := range r.records {
			if strings.Contains(command, "/"+name+"/") || strings.Contains(command, "/"+name+"\"") || strings.HasSuffix(strings.TrimSpace(command), "/"+name+"'") {
				return deploy.Result{Stdout: record}, nil
			}
		}
		// The live-record read names the live release; fall back to it.
		if record, ok := r.records[r.liveName]; ok && strings.Contains(command, r.liveName) {
			return deploy.Result{Stdout: record}, nil
		}
		return deploy.Result{}, errors.New("no such record in dry fixture")
	case strings.HasPrefix(command, "readlink -f "):
		return deploy.Result{Stdout: filepath.Join(r.releasesPath, r.liveName) + "\n"}, nil
	case strings.HasPrefix(command, "git -C "):
		return deploy.Result{Stdout: "abc123def456abc123def456abc123def456abcd\n"}, nil
	case strings.HasPrefix(command, "true"):
		return deploy.Result{}, nil
	case strings.Contains(command, "test -L "):
		return deploy.Result{}, errors.New("not a symlink in dry fixture")
	case strings.Contains(command, "test -e "), strings.Contains(command, "test -w "):
		return deploy.Result{}, nil
	case strings.Contains(command, "df -Pk"):
		return deploy.Result{Stdout: "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/root 100000000 40000000 60000000 40% /\n"}, nil
	case strings.Contains(command, "PHP_VERSION"):
		return deploy.Result{Stdout: "8.3.0"}, nil
	case strings.Contains(command, "Zend OPcache"):
		return deploy.Result{Stdout: "0"}, nil
	default:
		return deploy.Result{}, nil
	}
}

func (r *dryDeployRunner) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.commands...)
}

// writeDeployReadonlyFixture creates a project with one SSH remote carrying a
// deploy branch and path but no repository, so the preflight reports the
// missing-repository warning instead of reaching the network.
func writeDeployReadonlyFixture(t *testing.T) (root, deployPath, currentPath string) {
	t.Helper()
	root = t.TempDir()
	deployPath = filepath.Join(root, "target")
	currentPath = filepath.Join(deployPath, "current")
	cfg := "project_name: sample-project\n" +
		"framework: generic\n" +
		"domain: sample-project.test\n" +
		"remotes:\n" +
		"  fixture:\n" +
		"    host: 127.0.0.1\n" +
		"    user: deployer\n" +
		"    path: " + currentPath + "\n" +
		"    deploy:\n" +
		"      deploy_path: " + deployPath + "\n" +
		"      branch: main\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(cfg), 0644); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	return root, deployPath, currentPath
}

// stubDeployHostWithRunner resolves any remote to a host bound to the dry
// runner, so the real engine read paths run without touching the network.
func stubDeployHostWithRunner(t *testing.T, runner deploy.Runner, deployPath, currentPath string) {
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

func TestDeployReadOnlyPlanReturnsStructuredData(t *testing.T) {
	desktop.ResetStateForTest()
	root, _, _ := writeDeployReadonlyFixture(t)

	svc := desktop.NewDeployService()
	plan, err := svc.DeployPlan(root, "fixture")
	if err != nil {
		t.Fatalf("DeployPlan: %v", err)
	}
	if plan.Remote == "" || plan.Branch != "main" {
		t.Fatalf("unexpected plan header: %#v", plan)
	}
	if plan.BuildMode == "" || plan.PublishStrategy == "" {
		t.Fatalf("plan must resolve build and publish modes: %#v", plan)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("plan must carry at least one step")
	}
	seen := map[string]bool{}
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.Title) == "" {
			t.Fatalf("plan step must carry id and title: %#v", step)
		}
		seen[step.ID] = true
	}
	if !seen["deploy:code"] {
		t.Fatalf("plan must include deploy:code, got %v", plan.Steps)
	}
}

func TestDeployReadOnlyCheckReportsLayout(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath := writeDeployReadonlyFixture(t)
	runner := newDryDeployRunner(t, filepath.Join(deployPath, "releases"), currentPath)
	stubDeployHostWithRunner(t, runner, deployPath, currentPath)

	svc := desktop.NewDeployService()
	check, err := svc.DeployCheck(root, "fixture")
	if err != nil {
		t.Fatalf("DeployCheck: %v", err)
	}
	if !check.Deployable {
		t.Fatalf("dry fixture must be deployable: %#v", check)
	}
	if check.PublishStrategy != deploy.PublishInPlace {
		t.Fatalf("want in-place strategy from existing current path, got %#v", check)
	}
	if check.ProbePerformed {
		t.Fatalf("in-place check runs no atomic-rename probe: %#v", check)
	}
	if check.DeployPath == "" || check.CurrentPath == "" {
		t.Fatalf("check must report both paths: %#v", check)
	}
	foundRepoWarning := false
	for _, warning := range check.Warnings {
		if strings.Contains(warning, "repository") {
			foundRepoWarning = true
		}
	}
	if !foundRepoWarning {
		t.Fatalf("check must warn about the missing repository: %#v", check)
	}
}

// TestDeployReadOnlyCheckProbesSymlinkTargetsTransiently runs the check
// against a real directory behind a real symlink, so the strategy resolves to
// symlink and the real atomic-rename probe runs. It asserts the probe ran
// (ProbePerformed), that it left nothing behind, and that the served tree is
// untouched: CLI parity without silent residue.
func TestDeployReadOnlyCheckProbesSymlinkTargetsTransiently(t *testing.T) {
	desktop.ResetStateForTest()
	root := t.TempDir()
	deployPath := filepath.Join(root, "srv", "app")
	if err := os.MkdirAll(deployPath, 0o755); err != nil {
		t.Fatalf("seed deploy path: %v", err)
	}
	liveTarget := filepath.Join(root, "srv", "live")
	if err := os.MkdirAll(liveTarget, 0o755); err != nil {
		t.Fatalf("seed live target: %v", err)
	}
	currentPath := filepath.Join(root, "srv", "current")
	if err := os.Symlink(liveTarget, currentPath); err != nil {
		t.Fatalf("seed current symlink: %v", err)
	}
	cfg := "project_name: sample-project\n" +
		"framework: generic\n" +
		"domain: sample-project.test\n" +
		"remotes:\n" +
		"  symremote:\n" +
		"    host: 127.0.0.1\n" +
		"    user: deployer\n" +
		"    path: " + currentPath + "\n" +
		"    deploy:\n" +
		"      deploy_path: " + deployPath + "\n" +
		"      branch: main\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("seed fixture config: %v", err)
	}
	restore := desktop.SetDeployHostResolverForTest(func(cfg engine.Config, remote string, opts deploy.Options) (deploy.Host, error) {
		host := deploy.HostForTest(deployPath, deploy.LocalRunner{})
		host.Name = strings.ToLower(strings.TrimSpace(remote))
		host.CurrentPath = currentPath
		host.Repository = opts.Repository
		return host, nil
	})
	t.Cleanup(restore)

	svc := desktop.NewDeployService()
	check, err := svc.DeployCheck(root, "symremote")
	if err != nil {
		t.Fatalf("DeployCheck: %v", err)
	}
	if !check.Deployable {
		t.Fatalf("symlink fixture must be deployable: %#v", check)
	}
	if check.PublishStrategy != deploy.PublishSymlink {
		t.Fatalf("want symlink strategy from symlinked current path, got %#v", check)
	}
	if !check.ProbePerformed {
		t.Fatal("symlink check must disclose its transient probe")
	}
	if !strings.Contains(check.Layout, "swapped atomically") {
		t.Fatalf("symlink layout must name the atomic swap: %#v", check)
	}

	if _, err := os.Stat(filepath.Join(deployPath, ".dep")); !os.IsNotExist(err) {
		t.Fatalf("the probe left %s/.dep behind (stat err = %v)", deployPath, err)
	}
	var leftovers []string
	walkErr := filepath.WalkDir(deployPath, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(path, ".govard-mvprobe") {
			leftovers = append(leftovers, path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk deploy path: %v", walkErr)
	}
	if len(leftovers) > 0 {
		t.Fatalf("the probe left residue behind: %q", leftovers)
	}
	resolved, err := filepath.EvalSymlinks(currentPath)
	if err != nil || resolved != liveTarget {
		t.Fatalf("the probe must not touch the served link: resolved = %q, err = %v", resolved, err)
	}
}

// TestDeployCoreCheckResetIsQuiet pins the nil-restored preflight seam: after
// a stub is reset away, running the seam must succeed without printing engine
// notes to the host stderr.
func TestDeployCoreCheckResetIsQuiet(t *testing.T) {
	desktop.ResetStateForTest()
	desktop.SetDeployCoreCheckForTest(func(context.Context, deploy.Host, deploy.Options, string) error {
		return errors.New("stubbed preflight")
	})
	restore := desktop.SetDeployCoreCheckForTest(nil)
	t.Cleanup(restore)

	workDir := t.TempDir()
	deployPath := filepath.Join(workDir, "target")
	runner := newDryDeployRunner(t, filepath.Join(deployPath, "releases"), filepath.Join(deployPath, "current"))
	host := deploy.HostForTest(deployPath, runner)
	host.CurrentPath = filepath.Join(deployPath, "current")
	opts := deploy.Options{Branch: "main", Publish: deploy.PublishInPlace}

	captured := captureStderrForDeployCheck(t, func() {
		if err := desktop.RunDeployCoreCheckForTest(context.Background(), host, opts, workDir); err != nil {
			t.Fatalf("nil-restored preflight: %v", err)
		}
	})
	if captured != "" {
		t.Fatalf("nil-restored preflight must stay quiet, printed %q", captured)
	}
}

func captureStderrForDeployCheck(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = previous }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	captured, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr pipe: %v", err)
	}
	return string(captured)
}

func TestDeployReadOnlyStatusReportsLiveRelease(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath := writeDeployReadonlyFixture(t)
	runner := newDryDeployRunner(t, filepath.Join(deployPath, "releases"), currentPath)
	stubDeployHostWithRunner(t, runner, deployPath, currentPath)

	svc := desktop.NewDeployService()
	status, err := svc.DeployStatus(root, "fixture")
	if err != nil {
		t.Fatalf("DeployStatus: %v", err)
	}
	if !status.Reachable {
		t.Fatalf("dry fixture must be reachable: %#v", status)
	}
	if status.Release != "3" {
		t.Fatalf("want live release 3, got %#v", status)
	}
	if !strings.HasPrefix(status.Revision, "abc123") {
		t.Fatalf("status must carry the live revision: %#v", status)
	}
	if status.Error != "" {
		t.Fatalf("reachable status must carry no error: %#v", status)
	}
}

func TestDeployReadOnlyListReleasesNewestFirst(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath := writeDeployReadonlyFixture(t)
	runner := newDryDeployRunner(t, filepath.Join(deployPath, "releases"), currentPath)
	stubDeployHostWithRunner(t, runner, deployPath, currentPath)

	svc := desktop.NewDeployService()
	entries, err := svc.ListReleases(root, "fixture")
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 releases, got %#v", entries)
	}
	if entries[0].Release != "3" || entries[1].Release != "2" || entries[2].Release != "1" {
		t.Fatalf("releases must be newest first: %#v", entries)
	}
	if !entries[0].Live {
		t.Fatal("release 3 must be marked live")
	}
	if entries[0].Foreign {
		t.Fatal("govard-recorded releases must not be marked foreign")
	}
	if entries[0].Revision == "" || entries[0].Branch == "" || entries[0].Status == "" {
		t.Fatalf("release rows must carry revision/branch/status: %#v", entries[0])
	}
	for _, cmd := range runner.recorded() {
		for _, banned := range mutatingCommandFragments {
			if strings.Contains(cmd, banned) {
				t.Fatalf("ListReleases issued mutating command %q", cmd)
			}
		}
	}
}

// TestDeployReadOnlySkippedProbeReadsFalse proves the probe flag is observed,
// not inferred: the strategy reports symlink while the preflight itself is
// stubbed out, so no probe command can execute and disclosure must stay false.
// (The previous strategy==symlink inference read true here.)
func TestDeployReadOnlySkippedProbeReadsFalse(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath := writeDeployReadonlyFixture(t)
	runner := newDryDeployRunner(t, filepath.Join(deployPath, "releases"), currentPath)
	stubDeployHostWithRunner(t, runner, deployPath, currentPath)
	restoreStrategy := desktop.SetDeployPublishStrategyForTest(func(deploy.Host, deploy.Options) (string, error) {
		return deploy.PublishSymlink, nil
	})
	t.Cleanup(restoreStrategy)
	restoreCheck := desktop.SetDeployCoreCheckForTest(func(context.Context, deploy.Host, deploy.Options, string) error {
		return nil
	})
	t.Cleanup(restoreCheck)

	check, err := desktop.NewDeployService().DeployCheck(root, "fixture")
	if err != nil {
		t.Fatalf("DeployCheck: %v", err)
	}
	if check.PublishStrategy != deploy.PublishSymlink {
		t.Fatalf("want stubbed symlink strategy, got %#v", check)
	}
	if check.ProbePerformed {
		t.Fatal("a stubbed preflight issued no probe command, so ProbePerformed must be false")
	}
	if len(runner.recorded()) != 0 {
		t.Fatalf("a stubbed preflight must touch no transport, saw %q", runner.recorded())
	}
}

// stubDeployRunner is a minimal transport for probe-observer unit tests.
type stubDeployRunner struct {
	run func(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error)
}

func (s stubDeployRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	return s.run(ctx, command, opts)
}

// TestProbeMarkerIsSharedBetweenEngineAndObserver pins the probe-marker
// decoupling: the engine builds its probe filenames from deploy.ProbeMarker
// and the desktop observer matches on the same const, so neither side can
// drift to a literal the other no longer recognises.
func TestProbeMarkerIsSharedBetweenEngineAndObserver(t *testing.T) {
	if strings.TrimSpace(deploy.ProbeMarker) == "" {
		t.Fatal("deploy.ProbeMarker must name the atomic-rename probe marker")
	}
	src, err := os.ReadFile("../internal/deploy/tasks_prepare.go")
	if err != nil {
		t.Fatalf("read tasks_prepare.go: %v", err)
	}
	if !strings.Contains(string(src), "ProbeMarker") {
		t.Error("tasks_prepare.go must build its probe paths from the shared ProbeMarker const, not a literal")
	}
	inner := stubDeployRunner{run: func(context.Context, string, deploy.RunOptions) (deploy.Result, error) {
		return deploy.Result{}, nil
	}}
	wrapped, observed := desktop.WrapRunnerWithProbeObserverForTest(inner)
	probe := `ln -sfn "x/.dep" "x/.dep/` + deploy.ProbeMarker + `.link"`
	if _, err := wrapped.Run(context.Background(), probe, deploy.RunOptions{}); err != nil {
		t.Fatalf("probe command: %v", err)
	}
	if !observed() {
		t.Fatal("the observer must match the shared deploy.ProbeMarker")
	}
}

// TestProbeObserverReadsFalseWhenTransportNeverStarted pins the flip-after
// semantics: a failing non-probe command never reached the probe, so the flag
// stays false.
func TestProbeObserverReadsFalseWhenTransportNeverStarted(t *testing.T) {
	inner := stubDeployRunner{run: func(context.Context, string, deploy.RunOptions) (deploy.Result, error) {
		return deploy.Result{}, errors.New("transport never started")
	}}
	wrapped, observed := desktop.WrapRunnerWithProbeObserverForTest(inner)
	if _, err := wrapped.Run(context.Background(), "true", deploy.RunOptions{}); err == nil {
		t.Fatal("want the inner failure propagated")
	}
	if observed() {
		t.Fatal("a failed non-probe command must not read performed")
	}
}

// TestProbeObserverReadsTrueWhenExecutedProbeFails pins the other half: the
// probe ran and the transport answered with failure, which still counts as
// performed.
func TestProbeObserverReadsTrueWhenExecutedProbeFails(t *testing.T) {
	inner := stubDeployRunner{run: func(context.Context, string, deploy.RunOptions) (deploy.Result, error) {
		return deploy.Result{}, errors.New("mv: missing destination")
	}}
	wrapped, observed := desktop.WrapRunnerWithProbeObserverForTest(inner)
	probe := `mkdir -p "x/.dep" && ln -sfn "x/.dep" "x/.dep/.govard-mvprobe.link"`
	if _, err := wrapped.Run(context.Background(), probe, deploy.RunOptions{}); err == nil {
		t.Fatal("want the inner failure propagated")
	}
	if !observed() {
		t.Fatal("an executed probe that failed still counts as performed")
	}
}

// TestDeployPlanToleratesSeamChurnDuringFlight guards the await-closure
// capture pattern under the race detector: in-flight plan calls share no
// mutable state with concurrent seam swaps, so churning every seam while
// plans build must stay race-clean and successful.
func TestDeployPlanToleratesSeamChurnDuringFlight(t *testing.T) {
	desktop.ResetStateForTest()
	root, _, _ := writeDeployReadonlyFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if _, err := desktop.NewDeployService().DeployPlan(root, "fixture"); err != nil {
					t.Errorf("DeployPlan during churn: %v", err)
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 25; j++ {
			// Every switchable seam except the ceilings: DeployPlan reads the
			// ceiling var in the calling goroutine, so churning it here would
			// be a same-variable race by construction rather than a guard.
			desktop.SetDeployHostResolverForTest(nil)()
			desktop.SetDeployListReleasesForTest(nil)()
			desktop.SetDeployLiveReleaseForTest(nil)()
			desktop.SetDeployPublishStrategyForTest(nil)()
			desktop.SetDeployCoreCheckForTest(nil)()
		}
	}()
	wg.Wait()
}

// TestDeployHungCallYieldsTypedTimeout proves the ceiling is enforced even
// when the engine seam ignores its context: the hung stub never returns, yet
// the call fails typed within the bound instead of hanging the suite.
func TestDeployHungCallYieldsTypedTimeout(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath := writeDeployReadonlyFixture(t)
	runner := newDryDeployRunner(t, filepath.Join(deployPath, "releases"), currentPath)
	stubDeployHostWithRunner(t, runner, deployPath, currentPath)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	restoreList := desktop.SetDeployListReleasesForTest(func(context.Context, deploy.Host) ([]deploy.ReleaseEntry, error) {
		<-release
		return nil, nil
	})
	t.Cleanup(restoreList)
	restoreTimeouts := desktop.SetDeployTimeoutsForTest(30*time.Second, 100*time.Millisecond)
	t.Cleanup(restoreTimeouts)

	started := time.Now()
	_, err := desktop.NewDeployService().ListReleases(root, "fixture")
	elapsed := time.Since(started)
	if !assertDeployRemoteError(t, err, "fixture", "ListReleases") {
		return
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Errorf("timeout error must say it timed out: %v", err)
	}
	if elapsed > 10*time.Second {
		t.Errorf("hung call returned after %v, want it bounded by the ceiling", elapsed)
	}
}

func TestDeployReadOnlyUnreachableYieldsTypedErrorWithRetryHint(t *testing.T) {
	desktop.ResetStateForTest()
	root, _, _ := writeDeployReadonlyFixture(t)
	restore := desktop.SetDeployHostResolverForTest(func(engine.Config, string, deploy.Options) (deploy.Host, error) {
		return deploy.Host{}, errors.New("dial tcp 127.0.0.1:22: connect: connection refused")
	})
	t.Cleanup(restore)

	svc := desktop.NewDeployService()

	if _, err := svc.DeployCheck(root, "fixture"); !assertDeployRemoteError(t, err, "fixture", "DeployCheck") {
		return
	}
	status, err := svc.DeployStatus(root, "fixture")
	if !assertDeployRemoteError(t, err, "fixture", "DeployStatus") {
		return
	}
	// Wails discards the payload when a call returns an error, so an
	// unreachable target must come back as the empty row plus the typed
	// error — never a populated row the panel can never receive.
	if status != (desktop.DeployStatusResult{}) {
		t.Errorf("unreachable status must return the empty row with the typed error, got %#v", status)
	}
	if _, err := svc.ListReleases(root, "fixture"); !assertDeployRemoteError(t, err, "fixture", "ListReleases") {
		return
	}
}

// TestDeployReadOnlyStatusLiveFailureReturnsEmptyRow covers the second
// unreachable shape: the host resolved but the live read failed. Same
// contract — empty row plus the typed error, no hintless payload.
func TestDeployReadOnlyStatusLiveFailureReturnsEmptyRow(t *testing.T) {
	desktop.ResetStateForTest()
	root, deployPath, currentPath := writeDeployReadonlyFixture(t)
	runner := newDryDeployRunner(t, filepath.Join(deployPath, "releases"), currentPath)
	stubDeployHostWithRunner(t, runner, deployPath, currentPath)
	restore := desktop.SetDeployLiveReleaseForTest(func(context.Context, deploy.Host) (*deploy.Release, error) {
		return nil, errors.New("connection reset by target")
	})
	t.Cleanup(restore)

	status, err := desktop.NewDeployService().DeployStatus(root, "fixture")
	if !assertDeployRemoteError(t, err, "fixture", "DeployStatus") {
		return
	}
	if status != (desktop.DeployStatusResult{}) {
		t.Errorf("failed live read must return the empty row with the typed error, got %#v", status)
	}
}

func assertDeployRemoteError(t *testing.T, err error, remote, operation string) bool {
	t.Helper()
	if err == nil {
		t.Errorf("%s: want typed error for unreachable remote, got nil", operation)
		return false
	}
	var typed *desktop.DeployRemoteError
	if !errors.As(err, &typed) {
		t.Errorf("%s: want *DeployRemoteError, got %T (%v)", operation, err, err)
		return false
	}
	if !strings.Contains(typed.Error(), remote) {
		t.Errorf("%s: typed error must name remote %q: %v", operation, remote, err)
		return false
	}
	if !strings.Contains(strings.ToLower(typed.RetryHint), "retry") {
		t.Errorf("%s: typed error must carry a retry hint: %#v", operation, typed)
		return false
	}
	return true
}

func TestDeployReadOnlyUnknownRemoteFails(t *testing.T) {
	desktop.ResetStateForTest()
	root, _, _ := writeDeployReadonlyFixture(t)

	svc := desktop.NewDeployService()
	if _, err := svc.DeployPlan(root, "ghost"); err == nil {
		t.Fatal("DeployPlan with an unknown remote must fail")
	} else if !strings.Contains(strings.ToLower(err.Error()), "ghost") {
		t.Fatalf("unknown-remote error must name the remote: %v", err)
	}
}

// TestDeployMutatingCallsStayBehindPreviewTokens pins the M2 boundary. M1
// forbade every mutating reference; M2 legitimately adds the wrapped engine
// entry points (SelectRollbackTarget, RunStep, SandboxUp/Down, CoreLock and
// friends), so this test replaces that ban with the milestone rule: the
// transport-level calls no desktop path may ever reimplement stay banned
// outright, and the preview-token gate must exist in the source. The
// behavioral proof (apply without a token is refused, stale and foreign
// tokens are refused) lives in desktop_deploy_mutating_test.go.
func TestDeployMutatingCallsStayBehindPreviewTokens(t *testing.T) {
	src, err := os.ReadFile("../internal/desktop/deploy_service.go")
	if err != nil {
		t.Fatalf("read deploy_service.go: %v", err)
	}
	banned := []string{
		"CoreActivate", "AcquireLock", "ReleaseLock", "RefreshSandboxMirror",
		"sandbox up", "unlock", "\"push\"", "Push(",
	}
	for _, fragment := range banned {
		if strings.Contains(string(src), fragment) {
			t.Errorf("deploy_service.go must not contain %q: wrap the engine path instead of reimplementing transport", fragment)
		}
	}
	for _, want := range []string{
		"spendDeployPreviewToken", "mintDeployPreviewToken",
		"PreviewRollback", "ApplyRollback",
		"PreviewSandboxUp", "PreviewSandboxDown",
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("deploy_service.go must contain the preview-token gate %q", want)
		}
	}
}
