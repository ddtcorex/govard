package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/frameworks"
)

// Default timeout ceilings for the read-only deploy surface. The plan is
// local only and gets the short ceiling; every target-reading call gets the
// SSH ceiling so a slow target fails with a typed error instead of hanging
// the panel.
const (
	defaultDeployPlanTimeout = 30 * time.Second
	defaultDeployReadTimeout = 2 * time.Minute
)

// Live ceilings, overridable in tests via SetDeployTimeoutsForTest.
var (
	deployPlanTimeout = defaultDeployPlanTimeout
	deployReadTimeout = defaultDeployReadTimeout
)

// SetDeployTimeoutsForTest overrides the plan/read ceilings and returns a
// restore func, so timeout tests run in milliseconds instead of minutes.
func SetDeployTimeoutsForTest(plan, read time.Duration) func() {
	previousPlan, previousRead := deployPlanTimeout, deployReadTimeout
	deployPlanTimeout, deployReadTimeout = plan, read
	return func() { deployPlanTimeout, deployReadTimeout = previousPlan, previousRead }
}

// UI text caps for deploy DTO strings.
const (
	deployNameMax    = 128
	deployGeneralMax = 512
	deployCommandMax = 2000
)

// deployRetryHint is the actionable close of every unreachable-target error.
const deployRetryHint = "Verify the target is reachable and its SSH identity is loaded, then retry."

// DeployService exposes the read-only deploy surface (plan, check, status,
// releases) to the desktop panel. It only reads; the M2 follow-up adds
// preview-gated actions on this same service.
type DeployService struct {
	serviceBase
}

// NewDeployService builds a DeployService bound to the default platform.
func NewDeployService() *DeployService {
	return &DeployService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// DeployPlanStep is one sanitized row of a resolved deploy plan.
type DeployPlanStep struct {
	Index          int    `json:"index"`
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Stage          string `json:"stage"`
	Title          string `json:"title"`
	Command        string `json:"command"`
	Source         string `json:"source"`
	Implementation string `json:"implementation"`
	Skipped        bool   `json:"skipped"`
	SkipReason     string `json:"skipReason"`
}

// DeployPlanResult is the sanitized, structured deploy plan for one remote.
type DeployPlanResult struct {
	Remote          string           `json:"remote"`
	Branch          string           `json:"branch"`
	Revision        string           `json:"revision"`
	BuildMode       string           `json:"buildMode"`
	PublishStrategy string           `json:"publishStrategy"`
	Steps           []DeployPlanStep `json:"steps"`
}

// DeployCheckResult is the sanitized preflight report for one remote.
type DeployCheckResult struct {
	Remote          string `json:"remote"`
	Host            string `json:"host"`
	DeployPath      string `json:"deployPath"`
	CurrentPath     string `json:"currentPath"`
	PublishStrategy string `json:"publishStrategy"`
	Layout          string `json:"layout"`
	Deployable      bool   `json:"deployable"`
	// ProbePerformed reports whether the preflight ran its transient
	// atomic-rename probe (symlink strategy only), so the panel can disclose
	// the probe instead of presenting the check as purely observational.
	ProbePerformed bool     `json:"probePerformed"`
	Warnings       []string `json:"warnings"`
}

// DeployStatusResult is the sanitized live-release row for one remote.
type DeployStatusResult struct {
	Remote   string `json:"remote"`
	Release  string `json:"release"`
	Revision string `json:"revision"`
	Branch   string `json:"branch"`
	Status   string `json:"status"`
	// Reachable is false when the target answered nothing usable.
	Reachable bool `json:"reachable"`
	// Error carries the sanitized reason when Reachable is false.
	Error string `json:"error"`
}

// DeployReleaseEntry is one sanitized release row from the target.
type DeployReleaseEntry struct {
	Release   string `json:"release"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy"`
	Live      bool   `json:"live"`
	Foreign   bool   `json:"foreign"`
	Number    int    `json:"number"`
}

// DeployRemoteError is the typed failure for an unreachable target. It always
// carries a retry hint so the panel can offer a retry instead of a dead end.
type DeployRemoteError struct {
	Remote    string
	Operation string
	Err       error
	RetryHint string
}

// Error renders the sanitized failure with its retry hint.
func (e *DeployRemoteError) Error() string {
	remote := sanitizeUIText(strings.TrimSpace(e.Remote), deployNameMax)
	op := sanitizeUIText(strings.TrimSpace(e.Operation), deployNameMax)
	hint := sanitizeUIText(strings.TrimSpace(e.RetryHint), deployGeneralMax)
	detail := ""
	if e.Err != nil {
		detail = ": " + sanitizeUIText(e.Err.Error(), deployGeneralMax)
	}
	if op != "" {
		return fmt.Sprintf("%s for remote %q failed%s. %s", op, remote, detail, hint)
	}
	return fmt.Sprintf("remote %q failed%s. %s", remote, detail, hint)
}

// Unwrap exposes the underlying cause for errors.As / errors.Is.
func (e *DeployRemoteError) Unwrap() error { return e.Err }

// newDeployRemoteError builds the typed error with the shared retry hint, so
// no caller can produce a hintless failure.
func newDeployRemoteError(remote, operation string, cause error) *DeployRemoteError {
	return &DeployRemoteError{
		Remote:    strings.TrimSpace(remote),
		Operation: strings.TrimSpace(operation),
		Err:       cause,
		RetryHint: deployRetryHint,
	}
}

var (
	deployLoadConfigForDesktop = func(projectRoot string) (engine.Config, error) {
		cfg, _, err := engine.LoadConfigFromDir(projectRoot, true)
		return cfg, err
	}
	deployHostForDesktop = func(cfg engine.Config, remote string, opts deploy.Options) (deploy.Host, error) {
		return deploy.HostForConfig(cfg, remote, opts)
	}
	deployListReleasesForDesktop = func(ctx context.Context, host deploy.Host) ([]deploy.ReleaseEntry, error) {
		return deploy.ListReleases(ctx, host)
	}
	deployLiveReleaseForDesktop = func(ctx context.Context, host deploy.Host) (*deploy.Release, error) {
		return deploy.LiveRelease(ctx, host)
	}
	deployPublishStrategyForDesktop = func(host deploy.Host, opts deploy.Options) (string, error) {
		return deploy.ResolvePublishStrategy(host, opts)
	}
	deployCoreCheckForDesktop = defaultDeployCoreCheck
)

// defaultDeployCoreCheck runs the real preflight with its notes discarded, so
// a desktop check never spills engine progress text into the host's stderr.
func defaultDeployCoreCheck(ctx context.Context, host deploy.Host, opts deploy.Options, workDir string) error {
	sc := deploy.StepContextForTest(host, opts)
	sc.WorkDir = workDir
	sc.Out = io.Discard
	return deploy.CoreCheck(ctx, sc)
}

// RunDeployCoreCheckForTest runs the current preflight seam, so tests can pin
// the nil-restored behavior without reaching through the service.
func RunDeployCoreCheckForTest(ctx context.Context, host deploy.Host, opts deploy.Options, workDir string) error {
	return deployCoreCheckForDesktop(ctx, host, opts, workDir)
}

// SetDeployHostResolverForTest swaps the host seam and returns its restore func.
func SetDeployHostResolverForTest(fn func(engine.Config, string, deploy.Options) (deploy.Host, error)) func() {
	previous := deployHostForDesktop
	if fn == nil {
		deployHostForDesktop = func(cfg engine.Config, remote string, opts deploy.Options) (deploy.Host, error) {
			return deploy.HostForConfig(cfg, remote, opts)
		}
	} else {
		deployHostForDesktop = fn
	}
	return func() { deployHostForDesktop = previous }
}

// SetDeployListReleasesForTest swaps the releases seam and returns its restore func.
func SetDeployListReleasesForTest(fn func(context.Context, deploy.Host) ([]deploy.ReleaseEntry, error)) func() {
	previous := deployListReleasesForDesktop
	if fn == nil {
		deployListReleasesForDesktop = func(ctx context.Context, host deploy.Host) ([]deploy.ReleaseEntry, error) {
			return deploy.ListReleases(ctx, host)
		}
	} else {
		deployListReleasesForDesktop = fn
	}
	return func() { deployListReleasesForDesktop = previous }
}

// SetDeployLiveReleaseForTest swaps the live-release seam and returns its restore func.
func SetDeployLiveReleaseForTest(fn func(context.Context, deploy.Host) (*deploy.Release, error)) func() {
	previous := deployLiveReleaseForDesktop
	if fn == nil {
		deployLiveReleaseForDesktop = func(ctx context.Context, host deploy.Host) (*deploy.Release, error) {
			return deploy.LiveRelease(ctx, host)
		}
	} else {
		deployLiveReleaseForDesktop = fn
	}
	return func() { deployLiveReleaseForDesktop = previous }
}

// SetDeployPublishStrategyForTest swaps the strategy seam and returns its restore func.
func SetDeployPublishStrategyForTest(fn func(deploy.Host, deploy.Options) (string, error)) func() {
	previous := deployPublishStrategyForDesktop
	if fn == nil {
		deployPublishStrategyForDesktop = func(host deploy.Host, opts deploy.Options) (string, error) {
			return deploy.ResolvePublishStrategy(host, opts)
		}
	} else {
		deployPublishStrategyForDesktop = fn
	}
	return func() { deployPublishStrategyForDesktop = previous }
}

// SetDeployCoreCheckForTest swaps the preflight seam and returns its restore func.
func SetDeployCoreCheckForTest(fn func(context.Context, deploy.Host, deploy.Options, string) error) func() {
	previous := deployCoreCheckForDesktop
	if fn == nil {
		deployCoreCheckForDesktop = defaultDeployCoreCheck
	} else {
		deployCoreCheckForDesktop = fn
	}
	return func() { deployCoreCheckForDesktop = previous }
}

// deployRecipeForDesktop resolves the framework recipe the same way the run
// does: the registry owns framework recipes and anything without one gets the
// neutral default pipeline.
func deployRecipeForDesktop(cfg engine.Config) deploy.Recipe {
	if recipe, ok := frameworks.DeployRecipe(cfg.Framework); ok {
		return recipe
	}
	return deploy.DefaultRecipe()
}

// deployPlanRevisionLabel mirrors the run's revision display without resolving
// git, so the plan stays local-only.
func deployPlanRevisionLabel(opts deploy.Options) string {
	switch {
	case strings.TrimSpace(opts.Revision) != "":
		return strings.TrimSpace(opts.Revision)
	case strings.TrimSpace(opts.Tag) != "":
		return "tag " + strings.TrimSpace(opts.Tag)
	default:
		return "HEAD (resolved at deploy time)"
	}
}

// deployLayoutLabel names the served shape a publish strategy implies.
func deployLayoutLabel(strategy string) string {
	switch strategy {
	case deploy.PublishInPlace:
		return "current path is a real directory: releases are copied into it"
	case deploy.PublishSymlink:
		return "current path is absent or a symlink: releases are swapped atomically"
	default:
		return ""
	}
}

// resolveDeployContext validates the names and loads the project config the
// read paths share.
func resolveDeployContext(project, remote string) (root string, cfg engine.Config, name string, err error) {
	name = strings.TrimSpace(remote)
	if name == "" {
		return "", engine.Config{}, "", fmt.Errorf("remote name is required")
	}
	root, _, err = resolveProjectRootForRemotes(project)
	if err != nil {
		return "", engine.Config{}, "", err
	}
	cfg, err = deployLoadConfigForDesktop(root)
	if err != nil {
		return "", engine.Config{}, "", fmt.Errorf("load config for deploy: %w", err)
	}
	return root, cfg, name, nil
}

// resolveDeployHost maps a host-resolution failure to its class: an unknown
// name is a config error, anything else is the target not answering.
func resolveDeployHost(cfg engine.Config, remote string, opts deploy.Options, operation string) (deploy.Host, error) {
	host, err := deployHostForDesktop(cfg, remote, opts)
	if err != nil {
		if errors.Is(err, deploy.ErrUnknownRemote) {
			return deploy.Host{}, err
		}
		return deploy.Host{}, newDeployRemoteError(remote, operation, err)
	}
	return host, nil
}

// awaitDeployCall runs call in a goroutine and returns its outcome, or a
// typed timeout error when ctx lapses first. It exists because several engine
// entry points take no context: without the select a hung call would hang the
// panel past every ceiling above. The timeout cause is wrapped in
// DeployRemoteError, never returned raw, so the panel always gets the retry
// hint shape it renders.
//
// The losing goroutine is abandoned, not stopped: the result channel is
// buffered so the late call never blocks on send, and the engine's own
// runner timeouts still bound the underlying command. Abandonment only
// outlives the call itself, never the process.
func awaitDeployCall[T any](ctx context.Context, remote, operation string, call func() (T, error)) (T, error) {
	type outcome struct {
		value T
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		value, err := call()
		done <- outcome{value: value, err: err}
	}()
	select {
	case out := <-done:
		return out.value, out.err
	case <-ctx.Done():
		var zero T
		cause := ctx.Err()
		if errors.Is(cause, context.DeadlineExceeded) {
			cause = fmt.Errorf("timed out: %w", cause)
		}
		return zero, newDeployRemoteError(remote, operation, cause)
	}
}

// wrapDeployError keeps an already-typed timeout error as-is and wraps any
// other failure in the typed shape, so await call sites stay short and a
// timeout never comes back double-wrapped.
func wrapDeployError(remote, operation string, err error) *DeployRemoteError {
	var typed *DeployRemoteError
	if errors.As(err, &typed) {
		return typed
	}
	return newDeployRemoteError(remote, operation, err)
}

// awaitDeployErr is awaitDeployCall for calls that return only an error.
func awaitDeployErr(ctx context.Context, remote, operation string, call func() error) error {
	_, err := awaitDeployCall(ctx, remote, operation, func() (struct{}, error) {
		return struct{}{}, call()
	})
	return err
}

// probeObservingRunner delegates to an engine Runner and records whether the
// atomic-rename probe executed, identified by its probe-file marker. It lets
// DeployCheck report an observed ProbePerformed instead of inferring it from
// the strategy, so a stubbed or skipped preflight honestly reads false.
type probeObservingRunner struct {
	inner deploy.Runner
	ran   *atomic.Bool
}

func (r *probeObservingRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	if !strings.Contains(command, deploy.ProbeMarker) {
		return r.inner.Run(ctx, command, opts)
	}
	// The flag flips only after the transport returns, so a command that
	// never reached the transport never reads true; a probe that ran and
	// failed still counts as performed.
	result, err := r.inner.Run(ctx, command, opts)
	r.ran.Store(true)
	return result, err
}

// WrapRunnerWithProbeObserverForTest exposes the probe observer for tests: it
// returns the wrapped runner and a reader for the observed flag.
func WrapRunnerWithProbeObserverForTest(inner deploy.Runner) (deploy.Runner, func() bool) {
	var ran atomic.Bool
	return &probeObservingRunner{inner: inner, ran: &ran}, ran.Load
}

// DeployPlan returns the structured deploy plan for one remote. It reads the
// local config and recipe only and opens no connection. The timeout bounds
// the whole local computation: none of the plan entry points take a context,
// so the build runs under the select instead and a plan that outlives its
// ceiling is refused rather than returned late to a panel that moved on.
func (s *DeployService) DeployPlan(project string, remote string) (res DeployPlanResult, err error) {
	defer RecoverPanic(&err, "DeployPlan")
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployPlanTimeout)
	defer cancel()
	_, cfg, name, err := resolveDeployContext(project, remote)
	if err != nil {
		return DeployPlanResult{}, err
	}
	// Plan failures are configuration errors and pass through unwrapped; the
	// timeout already arrives typed from the await.
	// The recipe lookup is captured like the other seam funcs: the await
	// closure must not read shared state that a late start could race.
	recipeFn := deployRecipeForDesktop
	result, err := awaitDeployCall(ctx, name, "deploy plan", func() (DeployPlanResult, error) {
		opts, err := deploy.ResolveOptions(cfg, name, deploy.Overrides{})
		if err != nil {
			return DeployPlanResult{}, err
		}
		recipe := recipeFn(cfg)
		if err := deploy.ValidateSettings(recipe, opts.Settings); err != nil {
			return DeployPlanResult{}, err
		}
		if err := deploy.ValidateDBBackup(recipe, opts); err != nil {
			return DeployPlanResult{}, err
		}
		opts = deploy.WithRecipeDefaults(recipe, opts)
		hooks := make([]deploy.Hook, 0, len(opts.Hooks))
		for _, hookCfg := range opts.Hooks {
			hook, err := deploy.HookFromConfig(hookCfg)
			if err != nil {
				return DeployPlanResult{}, fmt.Errorf("invalid deploy hook: %w", err)
			}
			hooks = append(hooks, hook)
		}
		plan, err := deploy.BuildPlan(recipe, hooks)
		if err != nil {
			return DeployPlanResult{}, err
		}
		plan = plan.ForBuildMode(opts.Build).ForPublishStrategy(opts.Publish)

		steps := make([]DeployPlanStep, 0, len(plan.Steps))
		for idx, step := range plan.Steps {
			implementation := string(step.Implementation())
			command := ""
			if step.Implementation() == deploy.ImplementationCommand {
				command = step.Command
			}
			skipReason := step.SkipReason
			if step.Skipped && strings.TrimSpace(skipReason) == "" {
				skipReason = "no implementation for this framework"
			}
			steps = append(steps, DeployPlanStep{
				Index:          idx + 1,
				ID:             sanitizeUIText(step.ID, deployNameMax),
				Kind:           sanitizeUIText(string(step.Kind), deployNameMax),
				Stage:          sanitizeUIText(string(step.Stage), deployNameMax),
				Title:          sanitizeUIText(step.Title, deployGeneralMax),
				Command:        sanitizeUIText(command, deployCommandMax),
				Source:         sanitizeUIText(step.Source, deployNameMax),
				Implementation: sanitizeUIText(implementation, deployNameMax),
				Skipped:        step.Skipped,
				SkipReason:     sanitizeUIText(skipReason, deployGeneralMax),
			})
		}
		return DeployPlanResult{
			Remote:          sanitizeUIText(name, deployNameMax),
			Branch:          sanitizeUIText(opts.Branch, deployNameMax),
			Revision:        sanitizeUIText(deployPlanRevisionLabel(opts), deployNameMax),
			BuildMode:       sanitizeUIText(opts.Build, deployNameMax),
			PublishStrategy: sanitizeUIText(opts.Publish, deployNameMax),
			Steps:           steps,
		}, nil
	})
	if err != nil {
		return DeployPlanResult{}, err
	}
	return result, nil
}

// DeployCheck runs the read-only preflight for one remote and reports the
// target layout it found.
func (s *DeployService) DeployCheck(project string, remote string) (res DeployCheckResult, err error) {
	defer RecoverPanic(&err, "DeployCheck")
	root, cfg, name, err := resolveDeployContext(project, remote)
	if err != nil {
		return DeployCheckResult{}, err
	}
	opts, err := deploy.ResolveReadOptions(cfg, name, deploy.Overrides{})
	if err != nil {
		return DeployCheckResult{}, err
	}
	host, err := resolveDeployHost(cfg, name, opts, "deploy check")
	if err != nil {
		return DeployCheckResult{}, err
	}
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployReadTimeout)
	defer cancel()
	// The host the preflight runs against carries the probe observer, so the
	// flag below reports an executed probe rather than an assumed one: a
	// stubbed or skipped preflight issues no probe command and reads false.
	var probeRan atomic.Bool
	probedHost := host.WithRunner(&probeObservingRunner{inner: host.Runner(), ran: &probeRan})
	// Seam funcs are captured before the await: the abandoned loser must not
	// read the switchable package vars after a test restore replaces them.
	// A late-starting loser would otherwise race the restore, which the race
	// detector rightly flags.
	checkFn := deployCoreCheckForDesktop
	strategyFn := deployPublishStrategyForDesktop
	if err := awaitDeployErr(ctx, name, "deploy check", func() error {
		return checkFn(ctx, probedHost, opts, root)
	}); err != nil {
		return DeployCheckResult{}, wrapDeployError(name, "deploy check", err)
	}
	strategy, err := awaitDeployCall(ctx, name, "deploy check", func() (string, error) {
		return strategyFn(host, opts)
	})
	if err != nil {
		return DeployCheckResult{}, wrapDeployError(name, "deploy check", err)
	}
	var warnings []string
	if strings.TrimSpace(opts.Repository) == "" {
		warnings = append(warnings, "no repository configured for this remote")
	}
	return DeployCheckResult{
		Remote:          sanitizeUIText(name, deployNameMax),
		Host:            sanitizeUIText(host.Name, deployNameMax),
		DeployPath:      sanitizeUIText(host.DeployPath, deployGeneralMax),
		CurrentPath:     sanitizeUIText(host.CurrentPath, deployGeneralMax),
		PublishStrategy: sanitizeUIText(strategy, deployNameMax),
		Layout:          sanitizeUIText(deployLayoutLabel(strategy), deployGeneralMax),
		Deployable:      true,
		ProbePerformed:  probeRan.Load(),
		Warnings:        warnings,
	}, nil
}

// DeployStatus reports the live release of one remote. An unreachable target
// yields a typed error carrying a retry hint.
func (s *DeployService) DeployStatus(project string, remote string) (res DeployStatusResult, err error) {
	defer RecoverPanic(&err, "DeployStatus")
	_, cfg, name, err := resolveDeployContext(project, remote)
	if err != nil {
		return DeployStatusResult{}, err
	}
	opts, err := deploy.ResolveReadOptions(cfg, name, deploy.Overrides{
		Verify: engine.BoolPtr(false),
		Lock:   engine.BoolPtr(false),
	})
	if err != nil {
		return DeployStatusResult{}, err
	}
	host, err := resolveDeployHost(cfg, name, opts, "deploy status")
	if err != nil {
		// Wails discards the payload when a call returns an error, so the
		// row must stay empty here: the typed error alone carries the
		// retry hint to the panel.
		return DeployStatusResult{}, err
	}
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployReadTimeout)
	defer cancel()
	// The seam takes the context but a hung transport may ignore it, so the
	// await enforces the ceiling even then; the row still reports the outage.
	liveFn := deployLiveReleaseForDesktop
	live, err := awaitDeployCall(ctx, name, "deploy status", func() (*deploy.Release, error) {
		return liveFn(ctx, host)
	})
	if err != nil {
		return DeployStatusResult{}, wrapDeployError(name, "deploy status", err)
	}
	if live == nil {
		return DeployStatusResult{
			Remote:    sanitizeUIText(name, deployNameMax),
			Status:    sanitizeUIText("no live release", deployNameMax),
			Reachable: true,
		}, nil
	}
	return DeployStatusResult{
		Remote:    sanitizeUIText(name, deployNameMax),
		Release:   sanitizeUIText(live.Release, deployNameMax),
		Revision:  sanitizeUIText(live.Revision, deployGeneralMax),
		Branch:    sanitizeUIText(live.Branch, deployNameMax),
		Status:    sanitizeUIText(live.Status, deployNameMax),
		Reachable: true,
	}, nil
}

// ---------------------------------------------------------------------------
// M2 mutating surface: rollback and sandbox rehearse behind preview tokens.
//
// Every mutating entry requires a single-use preview token minted by its
// preview call plus an explicit confirm flag. The token binds the exact
// action and parameters the preview described; anything else (missing,
// foreign, spent, or mismatched) is refused before any engine call runs.
// ---------------------------------------------------------------------------

// RollbackTargetInfo is the sanitized release a preview resolves to.
type RollbackTargetInfo struct {
	Release  string `json:"release"`
	Revision string `json:"revision"`
	Branch   string `json:"branch"`
	Status   string `json:"status"`
}

// RollbackPreview describes the swap an apply would perform, plus the
// single-use token that authorises exactly that swap.
type RollbackPreview struct {
	Token    string             `json:"token"`
	Remote   string             `json:"remote"`
	Target   RollbackTargetInfo `json:"target"`
	Previous string             `json:"previous"`
	Strategy string             `json:"strategy"`
}

// RollbackResult reports what an apply did. Cancelled reads true when the
// operator stopped it mid-flight; Message then carries the honest cancel
// wording instead of a failure.
type RollbackResult struct {
	Remote    string `json:"remote"`
	Release   string `json:"release"`
	Revision  string `json:"revision"`
	Previous  string `json:"previous"`
	Message   string `json:"message"`
	Cancelled bool   `json:"cancelled"`
}

// SandboxUpPreview describes the rehearse target an apply would create,
// plus the single-use token that authorises exactly that creation.
type SandboxUpPreview struct {
	Token          string `json:"token"`
	Project        string `json:"project"`
	Profile        string `json:"profile"`
	Container      string `json:"container"`
	CurrentProfile string `json:"currentProfile"`
	Recreate       bool   `json:"recreate"`
	Exists         bool   `json:"exists"`
}

// SandboxUpResult is the sanitized state of the rehearsed sandbox.
// Cancelled reads true when the operator stopped the creation mid-flight;
// Message then carries the honest cancel wording instead of a failure.
type SandboxUpResult struct {
	Container  string `json:"container"`
	Project    string `json:"project"`
	Profile    string `json:"profile"`
	PHP        string `json:"php"`
	RemoteName string `json:"remoteName"`
	Message    string `json:"message"`
	Port       int    `json:"port"`
	WebPort    int    `json:"webPort"`
	Running    bool   `json:"running"`
	Exists     bool   `json:"exists"`
	Cancelled  bool   `json:"cancelled"`
}

// SandboxDownPreview names the container an apply would remove, plus the
// single-use token that authorises exactly that removal.
type SandboxDownPreview struct {
	Token     string `json:"token"`
	Project   string `json:"project"`
	Container string `json:"container"`
	Profile   string `json:"profile"`
	Purge     bool   `json:"purge"`
	Volumes   bool   `json:"volumes"`
	Exists    bool   `json:"exists"`
}

// SandboxDownResult reports the removal. Cancelled reads true when the
// operator stopped it mid-flight.
type SandboxDownResult struct {
	Container string `json:"container"`
	Message   string `json:"message"`
	Exists    bool   `json:"exists"`
	Running   bool   `json:"running"`
	Cancelled bool   `json:"cancelled"`
}

// SandboxSSHInfo carries the connection facts for the existing terminal
// path. It holds the key path, never key material.
type SandboxSSHInfo struct {
	User    string `json:"user"`
	Host    string `json:"host"`
	KeyPath string `json:"keyPath"`
	Command string `json:"command"`
	Port    int    `json:"port"`
	Exists  bool   `json:"exists"`
	Running bool   `json:"running"`
}

// defaultDeployMutateTimeout ceilings a confirmed mutation. Sandbox image
// builds and docroot rewrites take minutes, so the ceiling is generous; the
// op registry still lets the operator stop earlier, and stopping is honest.
const defaultDeployMutateTimeout = 15 * time.Minute

// deployMutateTimeout is the live ceiling, overridable in tests.
var deployMutateTimeout = defaultDeployMutateTimeout

// SetDeployMutateTimeoutForTest overrides the mutation ceiling and returns a
// restore func.
func SetDeployMutateTimeoutForTest(timeout time.Duration) func() {
	previous := deployMutateTimeout
	if timeout > 0 {
		deployMutateTimeout = timeout
	} else {
		deployMutateTimeout = defaultDeployMutateTimeout
	}
	return func() { deployMutateTimeout = previous }
}

// Mutating action names bound into preview tokens.
const (
	deployPreviewRollback    = "rollback"
	deployPreviewSandboxUp   = "sandbox-up"
	deployPreviewSandboxDown = "sandbox-down"
)

// deployPreview is one minted authorisation: the action, the project and
// remote it covers, the selector the preview was asked for (detail), and the
// resolved facts the apply must re-match (fingerprint). The token is
// single-use and short-lived; spending marks it, so a second click needs a
// fresh preview.
type deployPreview struct {
	action      string
	project     string
	remote      string
	detail      string
	fingerprint string
	created     time.Time
	used        bool
}

// deployPreviewTTL bounds how long a minted token stays spendable: a preview
// describes the world as it was, and an old one must not authorise a new one.
const defaultDeployPreviewTTL = 15 * time.Minute

// deployPreviewTTL is the live token lifetime, overridable in tests.
var deployPreviewTTL = defaultDeployPreviewTTL

// SetDeployPreviewTTLForTest overrides the token lifetime and returns its
// restore func. Non-positive values restore the default.
func SetDeployPreviewTTLForTest(ttl time.Duration) func() {
	previous := deployPreviewTTL
	if ttl > 0 {
		deployPreviewTTL = ttl
	} else {
		deployPreviewTTL = defaultDeployPreviewTTL
	}
	return func() { deployPreviewTTL = previous }
}

// deployPreviewTokenCap bounds the token map: previews are authorisations,
// not history, and a long-lived desktop must not accumulate them.
const deployPreviewTokenCap = 128

var deployPreviewTokens = struct {
	sync.Mutex
	byToken map[string]*deployPreview
}{byToken: map[string]*deployPreview{}}

// deployPreviewRandRead draws token randomness. It is a seam so tests can
// fail it without touching global randomness.
var deployPreviewRandRead = rand.Read

// SetDeployPreviewRandForTest swaps the token randomness seam and returns
// its restore func. Nil restores crypto/rand.
func SetDeployPreviewRandForTest(fn func([]byte) (int, error)) func() {
	previous := deployPreviewRandRead
	if fn == nil {
		deployPreviewRandRead = rand.Read
	} else {
		deployPreviewRandRead = fn
	}
	return func() { deployPreviewRandRead = previous }
}

// mintDeployPreviewToken stores a preview authorisation and returns its
// token. Tokens are random hex, never derived from parameters; a randomness
// failure is an error, never a weak fallback token.
func mintDeployPreviewToken(action, project, remote, detail, fingerprint string) (string, error) {
	var raw [16]byte
	if _, err := deployPreviewRandRead(raw[:]); err != nil {
		return "", fmt.Errorf("mint preview token: %w", err)
	}
	token := hex.EncodeToString(raw[:])
	deployPreviewTokens.Lock()
	defer deployPreviewTokens.Unlock()
	evictDeployPreviewTokensLocked()
	deployPreviewTokens.byToken[token] = &deployPreview{
		action:      action,
		project:     project,
		remote:      remote,
		detail:      detail,
		fingerprint: fingerprint,
		created:     time.Now(),
	}
	return token, nil
}

// evictDeployPreviewTokensLocked keeps the map bounded. Expired entries go
// first, then spent ones, then the oldest live entries until the cap holds.
// Callers hold the token mutex.
func evictDeployPreviewTokensLocked() {
	now := time.Now()
	for key, old := range deployPreviewTokens.byToken {
		if now.Sub(old.created) > deployPreviewTTL {
			delete(deployPreviewTokens.byToken, key)
		}
	}
	for key, old := range deployPreviewTokens.byToken {
		if old.used {
			delete(deployPreviewTokens.byToken, key)
		}
	}
	for len(deployPreviewTokens.byToken) >= deployPreviewTokenCap {
		oldest := ""
		var oldestAt time.Time
		first := true
		for key, old := range deployPreviewTokens.byToken {
			if first || old.created.Before(oldestAt) {
				oldest, oldestAt, first = key, old.created, false
			}
		}
		if oldest == "" {
			return
		}
		delete(deployPreviewTokens.byToken, oldest)
	}
}

// spendDeployPreviewToken spends the token for one attempt and returns the
// authorisation it carries, so the apply runs what the preview described
// without taking new parameters. Unknown, spent, or foreign tokens are
// refused before any engine call runs; spending happens first so one token
// can never authorise two attempts.
func spendDeployPreviewToken(token, action, project string) (*deployPreview, error) {
	return lookupDeployPreviewToken(token, action, project, true)
}

// peekDeployPreviewToken validates the token like a spend but leaves it
// unspent, so the caller can register the run before authorising it: a
// duplicate run is then refused without burning the token.
func peekDeployPreviewToken(token, action, project string) (*deployPreview, error) {
	return lookupDeployPreviewToken(token, action, project, false)
}

func lookupDeployPreviewToken(token, action, project string, markUsed bool) (*deployPreview, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, fmt.Errorf("a preview token is required: preview this action first, then apply with the token it minted")
	}
	deployPreviewTokens.Lock()
	defer deployPreviewTokens.Unlock()
	preview, ok := deployPreviewTokens.byToken[trimmed]
	if !ok {
		return nil, fmt.Errorf("the preview token is unknown: request a fresh preview before applying")
	}
	if time.Since(preview.created) > deployPreviewTTL {
		delete(deployPreviewTokens.byToken, trimmed)
		return nil, fmt.Errorf("the preview token has expired: request a fresh preview before applying")
	}
	if preview.used {
		return nil, fmt.Errorf("the preview token was already spent: request a fresh preview before applying again")
	}
	if preview.action != action || preview.project != project {
		return nil, fmt.Errorf("this preview token was minted for another action: request a fresh preview for this one")
	}
	if markUsed {
		preview.used = true
	}
	return preview, nil
}

// DeployPreviewTokenCountForTest reports how many tokens the map holds.
func DeployPreviewTokenCountForTest() int {
	deployPreviewTokens.Lock()
	defer deployPreviewTokens.Unlock()
	return len(deployPreviewTokens.byToken)
}

// resetDeployPreviewTokens clears minted tokens. Tests only; ResetStateForTest
// calls it so previews never leak between tests.
func resetDeployPreviewTokens() {
	deployPreviewTokens.Lock()
	defer deployPreviewTokens.Unlock()
	deployPreviewTokens.byToken = map[string]*deployPreview{}
}

var (
	deploySelectRollbackTargetForDesktop = deploy.SelectRollbackTarget
	deployRunStepForDesktop              = deploy.RunStep
	deployExecutorRunForDesktop          = func(ctx context.Context, host deploy.Host, opts deploy.Options, out io.Writer, plan deploy.Plan, vars deploy.Vars, release *deploy.Release) (deploy.Outcome, error) {
		return deploy.NewExecutor(host, opts, out).Run(ctx, plan, vars, release)
	}
	deploySandboxRuntimeForDesktop = func() deploy.SandboxRuntime { return deploy.NewDockerCLI() }
	// deploySandboxProbeForDesktop overrides the sandbox SSH probe. Nil keeps
	// the engine default (a real login); tests stub it.
	deploySandboxProbeForDesktop deploy.SandboxPortProbe
	launchInTerminalForDesktop   = LaunchInTerminal
)

// SetDeploySelectRollbackTargetForTest swaps the rollback-target seam and
// returns its restore func.
func SetDeploySelectRollbackTargetForTest(fn func(context.Context, deploy.Host, string) (*deploy.Release, error)) func() {
	previous := deploySelectRollbackTargetForDesktop
	if fn == nil {
		deploySelectRollbackTargetForDesktop = deploy.SelectRollbackTarget
	} else {
		deploySelectRollbackTargetForDesktop = fn
	}
	return func() { deploySelectRollbackTargetForDesktop = previous }
}

// SetDeployRunStepForTest swaps the rollback step seam and returns its
// restore func.
func SetDeployRunStepForTest(fn func(context.Context, deploy.Host, deploy.Options, deploy.Vars, deploy.Step, *deploy.Release, io.Writer) error) func() {
	previous := deployRunStepForDesktop
	if fn == nil {
		deployRunStepForDesktop = deploy.RunStep
	} else {
		deployRunStepForDesktop = fn
	}
	return func() { deployRunStepForDesktop = previous }
}

// SetDeployExecutorRunForTest swaps the in-place tail seam and returns its
// restore func.
func SetDeployExecutorRunForTest(fn func(context.Context, deploy.Host, deploy.Options, io.Writer, deploy.Plan, deploy.Vars, *deploy.Release) (deploy.Outcome, error)) func() {
	previous := deployExecutorRunForDesktop
	if fn == nil {
		deployExecutorRunForDesktop = func(ctx context.Context, host deploy.Host, opts deploy.Options, out io.Writer, plan deploy.Plan, vars deploy.Vars, release *deploy.Release) (deploy.Outcome, error) {
			return deploy.NewExecutor(host, opts, out).Run(ctx, plan, vars, release)
		}
	} else {
		deployExecutorRunForDesktop = fn
	}
	return func() { deployExecutorRunForDesktop = previous }
}

// SetDeploySandboxRuntimeForTest swaps the sandbox runtime seam and returns
// its restore func.
func SetDeploySandboxRuntimeForTest(fn func() deploy.SandboxRuntime) func() {
	previous := deploySandboxRuntimeForDesktop
	if fn == nil {
		deploySandboxRuntimeForDesktop = func() deploy.SandboxRuntime { return deploy.NewDockerCLI() }
	} else {
		deploySandboxRuntimeForDesktop = fn
	}
	return func() { deploySandboxRuntimeForDesktop = previous }
}

// SetDeploySandboxProbeForTest swaps the sandbox SSH probe seam and returns
// its restore func. Nil restores the engine default.
func SetDeploySandboxProbeForTest(fn deploy.SandboxPortProbe) func() {
	previous := deploySandboxProbeForDesktop
	deploySandboxProbeForDesktop = fn
	return func() { deploySandboxProbeForDesktop = previous }
}

// SetLaunchInTerminalForTest swaps the OS terminal launcher seam and returns
// its restore func.
func SetLaunchInTerminalForTest(fn func(string, string) error) func() {
	previous := launchInTerminalForDesktop
	if fn == nil {
		launchInTerminalForDesktop = LaunchInTerminal
	} else {
		launchInTerminalForDesktop = fn
	}
	return func() { launchInTerminalForDesktop = previous }
}

// rollbackFingerprint binds a token to the exact swap it authorises: the
// release, its revision, and the strategy the swap runs with.
func rollbackFingerprint(target *deploy.Release, strategy string) string {
	return strings.Join([]string{target.Release, target.Revision, strategy}, "\x00")
}

// PreviewRollback resolves the swap an apply would perform and mints the
// single-use token that authorises exactly that swap. It reads only.
func (s *DeployService) PreviewRollback(project string, remote string, to string) (res RollbackPreview, err error) {
	defer RecoverPanic(&err, "PreviewRollback")
	root, cfg, name, err := resolveDeployContext(project, remote)
	if err != nil {
		return RollbackPreview{}, err
	}
	opts, err := deploy.ResolveOptions(cfg, name, deploy.Overrides{})
	if err != nil {
		return RollbackPreview{}, err
	}
	host, err := resolveDeployHost(cfg, name, opts, "rollback preview")
	if err != nil {
		return RollbackPreview{}, err
	}
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployReadTimeout)
	defer cancel()
	// Seam funcs are captured before the await, like the read paths: an
	// abandoned loser must not read the switchable package vars after a
	// test restore replaces them.
	selectFn := deploySelectRollbackTargetForDesktop
	strategyFn := deployPublishStrategyForDesktop
	liveFn := deployLiveReleaseForDesktop
	target, err := awaitDeployCall(ctx, name, "rollback preview", func() (*deploy.Release, error) {
		return selectFn(ctx, host, to)
	})
	if err != nil {
		// A timeout already arrives typed from the await; a domain refusal
		// ("no release older than ...") passes through unwrapped so it never
		// gains a retry hint a retry could not satisfy.
		return RollbackPreview{}, err
	}
	strategy, err := awaitDeployCall(ctx, name, "rollback preview", func() (string, error) {
		return strategyFn(host, opts)
	})
	if err != nil {
		return RollbackPreview{}, err
	}
	// The previous release is best effort: not knowing what is live must not
	// block a preview, and the apply re-reads it before it swaps.
	previous := ""
	if live, liveErr := awaitDeployCall(ctx, name, "rollback preview", func() (*deploy.Release, error) {
		return liveFn(ctx, host)
	}); liveErr == nil && live != nil {
		previous = live.Release
	}
	token, err := mintDeployPreviewToken(deployPreviewRollback, root, name, to, rollbackFingerprint(target, strategy))
	if err != nil {
		return RollbackPreview{}, err
	}
	return RollbackPreview{
		Token:  token,
		Remote: sanitizeUIText(name, deployNameMax),
		Target: RollbackTargetInfo{
			Release:  sanitizeUIText(target.Release, deployNameMax),
			Revision: sanitizeUIText(target.Revision, deployGeneralMax),
			Branch:   sanitizeUIText(target.Branch, deployNameMax),
			Status:   sanitizeUIText(target.Status, deployNameMax),
		},
		Previous: sanitizeUIText(previous, deployNameMax),
		Strategy: sanitizeUIText(strategy, deployNameMax),
	}, nil
}

// firstDeployPlanStep returns the named step of a plan, or the zero Step when
// the recipe does not implement it. It mirrors the lookup the CLI rollback
// runs, over the same plan shape.
func firstDeployPlanStep(plan deploy.Plan, id string) deploy.Step {
	steps := plan.Only(id).Steps
	if len(steps) == 0 {
		return deploy.Step{}
	}
	return steps[0]
}

// rollbackCancelResult reports a deliberate stop through the result instead
// of an error, following the shared cancel convention: a cancel is not a
// failure, so it carries no retry prompt. The note says exactly what remains,
// because a stop mid-swap can leave the target half-moved.
func rollbackCancelResult(remote, note string) RollbackResult {
	message := humanOpMessage(deployPreviewRollback, context.Canceled)
	text := message.Title + ". " + message.Hint
	if strings.TrimSpace(note) != "" {
		text += " " + strings.TrimSpace(note)
	}
	return RollbackResult{
		Remote:    sanitizeUIText(remote, deployNameMax),
		Cancelled: true,
		Message:   sanitizeUIText(text, deployGeneralMax),
	}
}

// restoreRollbackRecordNote attempts the record restore a failed tail owes
// the next attempt and reports what happened, so the message states it
// instead of implying it.
func restoreRollbackRecordNote(ctx context.Context, host deploy.Host, original *deploy.Release) string {
	if err := deploy.WriteRelease(ctx, host, original); err != nil {
		return "the release record could not be restored: " + err.Error()
	}
	return "the release record was restored"
}

// closeRollbackWindowNote attempts the maintenance close a stopped tail owes
// and reports what happened. Where the recipe offers no close, it says that
// plainly instead of claiming a state it cannot produce.
func closeRollbackWindowNote(ctx context.Context, runStep func(context.Context, deploy.Host, deploy.Options, deploy.Vars, deploy.Step, *deploy.Release, io.Writer) error, host deploy.Host, opts deploy.Options, vars deploy.Vars, plan deploy.Plan, target *deploy.Release) string {
	disable := firstDeployPlanStep(plan, deploy.TaskMaintenanceDisable)
	if disable.ID == "" {
		return "the recipe has no maintenance step, so the open window cannot be closed from here"
	}
	if err := runStep(ctx, host, opts, vars, disable, target, io.Discard); err != nil {
		return "the maintenance window is still open: " + err.Error()
	}
	return "the maintenance window was closed"
}

// applyRollbackRefusesWithoutConfirm guards the confirm half of the gate.
func applyRollbackRefusesWithoutConfirm(confirm bool) error {
	if confirm {
		return nil
	}
	return fmt.Errorf("confirm the rollback explicitly before applying: pass confirm with a fresh preview token")
}

// ApplyRollback spends a preview token and performs the swap it authorised.
// The token binds the target release, its revision, and the strategy; a
// target that moved since the preview is refused as stale. The run holds the
// deploy lock like the CLI path and registers on the op registry, so
// CancelOp stops it and the lock is still released.
func (s *DeployService) ApplyRollback(project string, token string, confirm bool) (res RollbackResult, err error) {
	defer RecoverPanic(&err, "ApplyRollback")
	if err := applyRollbackRefusesWithoutConfirm(confirm); err != nil {
		return RollbackResult{}, err
	}
	// The project resolves first: a path that resolves nowhere is a usage
	// error, never a spend. The token carries the remote and the selector,
	// so the apply runs what the preview described and takes no parameters.
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return RollbackResult{}, err
	}
	cfg, err := deployLoadConfigForDesktop(root)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("load config for deploy: %w", err)
	}
	// The token is peeked first so the run registers before it authorises:
	// a duplicate run is refused without burning the token.
	peeked, err := peekDeployPreviewToken(token, deployPreviewRollback, root)
	if err != nil {
		return RollbackResult{}, err
	}
	name := strings.TrimSpace(peeked.remote)
	if name == "" {
		return RollbackResult{}, fmt.Errorf("the preview token names no remote: request a fresh preview before applying")
	}
	opts, err := deploy.ResolveOptions(cfg, name, deploy.Overrides{})
	if err != nil {
		return RollbackResult{}, err
	}
	host, err := resolveDeployHost(cfg, name, opts, "rollback")
	if err != nil {
		return RollbackResult{}, err
	}

	opID := "deploy-rollback:" + root + "\x00" + name
	opCtx, cancel := context.WithCancel(s.lifecycleContext())
	if !RegisterOp(opID, root, "deploy-rollback", cancel) {
		cancel()
		return RollbackResult{}, fmt.Errorf("a rollback for remote %q is already running", name)
	}
	defer cancel()
	defer FinishOp(opID)
	preview, err := spendDeployPreviewToken(token, deployPreviewRollback, root)
	if err != nil {
		return RollbackResult{}, err
	}
	ctx, stop := context.WithTimeout(opCtx, deployMutateTimeout)
	defer stop()

	cancelled := func(note string) (RollbackResult, bool) {
		if ctx.Err() == context.Canceled {
			return rollbackCancelResult(name, note), true
		}
		return RollbackResult{}, false
	}

	selectFn := deploySelectRollbackTargetForDesktop
	strategyFn := deployPublishStrategyForDesktop
	liveFn := deployLiveReleaseForDesktop
	runStepFn := deployRunStepForDesktop
	executorFn := deployExecutorRunForDesktop

	target, err := selectFn(ctx, host, preview.detail)
	if err != nil {
		if out, ok := cancelled("no changes were made"); ok {
			return out, nil
		}
		return RollbackResult{}, err
	}
	strategy, err := strategyFn(host, opts)
	if err != nil {
		if out, ok := cancelled("no changes were made"); ok {
			return out, nil
		}
		return RollbackResult{}, err
	}
	// The swap the preview authorised is the swap that runs: a target that
	// moved since (another deploy went live) is stale, not applied.
	if rollbackFingerprint(target, strategy) != preview.fingerprint {
		if out, ok := cancelled("no changes were made"); ok {
			return out, nil
		}
		return RollbackResult{}, fmt.Errorf("the target moved since the preview: request a fresh preview before applying")
	}
	// The apply publishes the existing release: the revision is the one that
	// release was built from, never the branch tip.
	opts.Publish = strategy
	opts.Revision = target.Revision
	opts.Tag = ""
	opts.Force = true

	recipe := deployRecipeForDesktop(cfg)
	if err := deploy.ValidateSettings(recipe, opts.Settings); err != nil {
		return RollbackResult{}, err
	}
	if err := deploy.ValidateDBBackup(recipe, opts); err != nil {
		return RollbackResult{}, err
	}
	opts = deploy.WithRecipeDefaults(recipe, opts)
	hooks := make([]deploy.Hook, 0, len(opts.Hooks))
	for _, hookCfg := range opts.Hooks {
		hook, err := deploy.HookFromConfig(hookCfg)
		if err != nil {
			return RollbackResult{}, fmt.Errorf("invalid deploy hook: %w", err)
		}
		hooks = append(hooks, hook)
	}
	plan, err := deploy.BuildPlan(recipe, hooks)
	if err != nil {
		return RollbackResult{}, err
	}
	plan = plan.ForPublishStrategy(strategy)
	vars := deploy.VarsFor(host, opts)

	// The previous release is read before anything swaps, like the CLI path.
	previous := ""
	if live, liveErr := liveFn(ctx, host); liveErr == nil && live != nil {
		previous = live.Release
	} else if liveErr != nil {
		if out, ok := cancelled("no changes were made"); ok {
			return out, nil
		}
		return RollbackResult{}, wrapDeployError(name, "rollback", liveErr)
	}

	// A confirmed apply changes what the target serves, so it holds the same
	// lock a deploy would: a deploy starting mid-apply would fight it for the
	// link and the release record.
	var lockSC *deploy.StepContext
	if !opts.SkipLock {
		lockSC = &deploy.StepContext{
			Host:    host,
			Runner:  host.Runner(),
			Vars:    deploy.NewVars(),
			Release: target,
			Opts:    opts,
			Out:     io.Discard,
		}
		if err := deploy.CoreLock(ctx, lockSC); err != nil {
			if out, ok := cancelled("no changes were made"); ok {
				return out, nil
			}
			return RollbackResult{}, err
		}
		// Stopping still releases the lock: cleanup runs on a context that
		// cannot be stopped, because the reason the apply failed may be the
		// very stop that would skip it.
		defer func() {
			if uerr := deploy.CoreUnlock(context.WithoutCancel(ctx), lockSC); uerr != nil && err == nil {
				note := sanitizeUIText("the deploy lock could not be released: "+uerr.Error(), deployGeneralMax)
				res.Message = strings.TrimSpace(res.Message + " " + note)
			}
		}()
	}

	if strategy == deploy.PublishSymlink {
		activate := firstDeployPlanStep(plan, deploy.TaskActivate)
		if activate.ID == "" {
			return RollbackResult{}, fmt.Errorf("the recipe declares no %s task, so nothing can be activated", deploy.TaskActivate)
		}
		if err := runStepFn(ctx, host, opts, vars, activate, target, io.Discard); err != nil {
			if out, ok := cancelled("no release was activated"); ok {
				return out, nil
			}
			return RollbackResult{}, fmt.Errorf("re-point %s at release %s: %w", host.CurrentPath, target.Release, err)
		}
		// The swap is atomic on disk, but the application the target serves
		// was built for the release that was live a moment ago: its compiled
		// configuration and caches belong to the code that just went away.
		// The CLI path runs this same flush after the swap and before the
		// verify, so the desktop path must too — otherwise the GUI reports
		// success while the target serves stale caches.
		if flush := firstDeployPlanStep(plan, deploy.TaskAppCacheFlush); flush.ID != "" {
			if err := runStepFn(ctx, host, opts, vars, flush, target, io.Discard); err != nil {
				if out, ok := cancelled(fmt.Sprintf("release %s was activated before the stop; check the target status to confirm what is live", target.Release)); ok {
					return out, nil
				}
				return RollbackResult{}, fmt.Errorf("flush the caches of release %s after the rollback: %w", target.Release, err)
			}
		}
		if opts.Verify {
			if verify := firstDeployPlanStep(plan, deploy.TaskVerify); verify.ID != "" {
				if err := runStepFn(ctx, host, opts, vars, verify, target, io.Discard); err != nil {
					if out, ok := cancelled(fmt.Sprintf("release %s was activated before the stop; check the target status to confirm what is live", target.Release)); ok {
						return out, nil
					}
					return RollbackResult{}, err
				}
			}
		}
	} else {
		// The tail runs the executor over the target's own record, which the
		// executor rewrites as it goes. A stopped or failed tail still owes
		// two cleanups: the record restore that keeps the release usable for
		// the next attempt, and the maintenance close. Both run on a context
		// that cannot be stopped, and the message reports exactly what each
		// left behind.
		windowOpen := false
		if enable := firstDeployPlanStep(plan, deploy.TaskMaintenanceEnable); enable.ID != "" && !enable.Skipped {
			if err := runStepFn(ctx, host, opts, vars, enable, target, io.Discard); err != nil {
				if out, ok := cancelled("no changes were made"); ok {
					return out, nil
				}
				return RollbackResult{}, fmt.Errorf("enable maintenance mode before rewriting %s: %w", host.CurrentPath, err)
			}
			windowOpen = true
		}
		tail, ok := plan.From(deploy.TaskActivate)
		if !ok {
			return RollbackResult{}, fmt.Errorf("the recipe declares no %s task, so an in-place apply cannot run", deploy.TaskActivate)
		}
		tail = tail.Excluding(deploy.TaskUnlock, deploy.TaskCleanup)
		original := *target
		if _, err := executorFn(ctx, host, opts, io.Discard, tail, vars, target); err != nil {
			cleanupCtx := context.WithoutCancel(ctx)
			restoreNote := restoreRollbackRecordNote(cleanupCtx, host, &original)
			windowNote := "no maintenance window was opened"
			if windowOpen {
				windowNote = closeRollbackWindowNote(cleanupCtx, runStepFn, host, opts, vars, plan, target)
			}
			if out, done := cancelled(restoreNote + "; " + windowNote); done {
				return out, nil
			}
			return RollbackResult{}, fmt.Errorf("%w (%s; %s)", err, restoreNote, windowNote)
		}
	}

	short := deploy.ShortRevision(target.Revision)
	return RollbackResult{
		Remote:   sanitizeUIText(name, deployNameMax),
		Release:  sanitizeUIText(target.Release, deployNameMax),
		Revision: sanitizeUIText(target.Revision, deployGeneralMax),
		Previous: sanitizeUIText(previous, deployNameMax),
		Message:  sanitizeUIText(fmt.Sprintf("%s now serves release %s (%s)", name, target.Release, short), deployGeneralMax),
	}, nil
}

// resolveSandboxContext validates the project and loads its config, the
// part of context resolution every sandbox call shares.
func resolveSandboxContext(project string) (root string, cfg engine.Config, err error) {
	root, _, err = resolveProjectRootForRemotes(project)
	if err != nil {
		return "", engine.Config{}, err
	}
	cfg, err = deployLoadConfigForDesktop(root)
	if err != nil {
		return "", engine.Config{}, fmt.Errorf("load config for deploy: %w", err)
	}
	return root, cfg, nil
}

// buildSandboxRequest shapes one rehearse invocation from the project
// configuration plus the previewed flags, the way the CLI shapes its own:
// the framework recipe owns the container requirements, the stack owns the
// series defaults, and only an explicitly named profile may conflict with
// the container that exists. The snapshot stays claimed by nobody: a
// rehearse starts empty, and seeding one stays a later task.
func buildSandboxRequest(cfg engine.Config, root, profile string, recreate, purge, volumes bool, out io.Writer) (deploy.SandboxRequest, error) {
	recipe := deployRecipeForDesktop(cfg)
	if err := deploy.ValidateSettings(recipe, cfg.Deploy.Settings); err != nil {
		return deploy.SandboxRequest{}, err
	}
	requirements := deploy.SandboxRequirementsWithSettings(recipe.Sandbox, cfg.Deploy.Settings)
	if err := deploy.ValidateSandboxTools(requirements.Tools); err != nil {
		return deploy.SandboxRequest{}, err
	}
	trimmed := strings.TrimSpace(profile)
	explicit := trimmed != ""
	if !explicit {
		trimmed = deploy.DefaultSandboxProfile
	}
	if _, err := deploy.ValidateSandboxProfile(trimmed); err != nil {
		return deploy.SandboxRequest{}, err
	}
	return deploy.SandboxRequest{
		ProjectRoot:     root,
		ProjectName:     cfg.ProjectName,
		Profile:         trimmed,
		ProfileExplicit: explicit,
		PHPDefault:      cfg.Stack.PHPVersion,
		DBEngine:        cfg.Stack.Services.DB,
		DBVersion:       cfg.Stack.DBVersion,
		WebRoot:         cfg.Stack.WebRoot,
		Requirements:    requirements,
		Recreate:        recreate,
		Purge:           purge,
		Volumes:         volumes,
		Out:             out,
		Probe:           deploySandboxProbeForDesktop,
	}, nil
}

// sandboxUpFingerprint binds an up token to the requested profile and shape.
func sandboxUpFingerprint(profile string, recreate bool) string {
	flag := "0"
	if recreate {
		flag = "1"
	}
	return strings.Join([]string{strings.TrimSpace(profile), flag}, "\x00")
}

// parseSandboxUpFingerprint reads the bound request back. It parses what the
// preview minted, so the shape is trusted by construction.
func parseSandboxUpFingerprint(fingerprint string) (profile string, recreate bool) {
	profile, flag, _ := strings.Cut(fingerprint, "\x00")
	return profile, strings.TrimSpace(flag) == "1"
}

// sandboxDownFingerprint binds a down token to the removal shape.
func sandboxDownFingerprint(purge, volumes bool) string {
	purgeFlag, volumesFlag := "0", "0"
	if purge {
		purgeFlag = "1"
	}
	if volumes {
		volumesFlag = "1"
	}
	return strings.Join([]string{purgeFlag, volumesFlag}, "\x00")
}

// parseSandboxDownFingerprint reads the bound removal shape back.
func parseSandboxDownFingerprint(fingerprint string) (purge, volumes bool) {
	purgeFlag, volumesFlag, _ := strings.Cut(fingerprint, "\x00")
	return strings.TrimSpace(purgeFlag) == "1", strings.TrimSpace(volumesFlag) == "1"
}

// sandboxCancelStateNote re-reads the container after a stop and reports
// what remains: the named container and its state, the absence of one, or
// the plain fact that the state could not be determined. It reads on a
// context detached from the stop, so a hung runtime cannot hold the cancel
// answer hostage.
func sandboxCancelStateNote(ctx context.Context, runtime deploy.SandboxRuntime, root, projectName string) string {
	readCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), deployReadTimeout)
	defer stop()
	state, err := deploy.SandboxStatus(readCtx, runtime, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: projectName,
	})
	if err != nil {
		return "the container state could not be determined"
	}
	if !state.Exists {
		return "no container was left behind"
	}
	container := strings.TrimSpace(state.Container)
	if container == "" {
		container = deploy.SandboxContainerName(projectName, root)
	}
	if state.Running {
		return "the container " + container + " remains running"
	}
	return "the container " + container + " remains but is not running"
}

// sandboxUpCancelResult reports a deliberate stop through the result, naming
// the half-created state the status read found.
func sandboxUpCancelResult(project, stateNote string) SandboxUpResult {
	message := humanOpMessage(deployPreviewSandboxUp, context.Canceled)
	text := message.Title + ". " + message.Hint
	if strings.TrimSpace(stateNote) != "" {
		text += " " + strings.TrimSpace(stateNote) + "."
	}
	return SandboxUpResult{
		Project:   sanitizeUIText(project, deployNameMax),
		Cancelled: true,
		Message:   sanitizeUIText(text, deployGeneralMax),
	}
}

// sandboxDownCancelResult reports a deliberate stop through the result,
// naming the state the status read found.
func sandboxDownCancelResult(container, stateNote string) SandboxDownResult {
	message := humanOpMessage(deployPreviewSandboxDown, context.Canceled)
	text := message.Title + ". " + message.Hint
	if strings.TrimSpace(stateNote) != "" {
		text += " " + strings.TrimSpace(stateNote) + "."
	}
	return SandboxDownResult{
		Container: sanitizeUIText(container, deployNameMax),
		Cancelled: true,
		Message:   sanitizeUIText(text, deployGeneralMax),
	}
}

// PreviewSandboxUp describes the rehearse target an apply would create and
// mints the single-use token that authorises exactly that creation. It
// changes nothing: a profile that disagrees with the existing container is
// refused here, with the recreate hint, before any token exists.
func (s *DeployService) PreviewSandboxUp(project string, profile string, recreate bool) (res SandboxUpPreview, err error) {
	defer RecoverPanic(&err, "PreviewSandboxUp")
	root, cfg, err := resolveSandboxContext(project)
	if err != nil {
		return SandboxUpPreview{}, err
	}
	request, err := buildSandboxRequest(cfg, root, profile, recreate, false, false, io.Discard)
	if err != nil {
		return SandboxUpPreview{}, err
	}
	runtime := deploySandboxRuntimeForDesktop()
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployReadTimeout)
	defer cancel()
	state, err := deploy.SandboxStatus(ctx, runtime, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: cfg.ProjectName,
	})
	if err != nil {
		return SandboxUpPreview{}, err
	}
	// A reused container is what it is: an explicit profile that disagrees
	// with it is refused rather than silently reshaped, and the refusal
	// names the recreate path, mirroring the engine's own backstop.
	if state.Exists && request.ProfileExplicit && !recreate && strings.TrimSpace(state.Profile) != "" &&
		!strings.EqualFold(strings.TrimSpace(state.Profile), strings.TrimSpace(request.Profile)) {
		return SandboxUpPreview{}, fmt.Errorf(
			"the sandbox container %s was built for the %s profile; changing it means a new image, so preview again with --recreate --profile %s",
			state.Container, state.Profile, strings.TrimSpace(request.Profile))
	}
	token, err := mintDeployPreviewToken(deployPreviewSandboxUp, root, "", "", sandboxUpFingerprint(request.Profile, recreate))
	if err != nil {
		return SandboxUpPreview{}, err
	}
	return SandboxUpPreview{
		Token:          token,
		Project:        sanitizeUIText(cfg.ProjectName, deployNameMax),
		Profile:        sanitizeUIText(request.Profile, deployNameMax),
		Container:      sanitizeUIText(deploy.SandboxContainerName(cfg.ProjectName, root), deployNameMax),
		CurrentProfile: sanitizeUIText(state.Profile, deployNameMax),
		Recreate:       recreate,
		Exists:         state.Exists,
	}, nil
}

// SandboxUp spends an up token and creates the rehearsed target it
// authorised. The creation is a long op on the registry with a real stop:
// CancelOp aborts the run and the result says cancelled.
func (s *DeployService) SandboxUp(project string, token string, confirm bool) (res SandboxUpResult, err error) {
	defer RecoverPanic(&err, "SandboxUp")
	if !confirm {
		return SandboxUpResult{}, fmt.Errorf("confirm the sandbox creation explicitly before applying: pass confirm with a fresh preview token")
	}
	root, cfg, err := resolveSandboxContext(project)
	if err != nil {
		return SandboxUpResult{}, err
	}
	opID := "sandbox-up:" + root
	opCtx, cancel := context.WithCancel(s.lifecycleContext())
	if !RegisterOp(opID, root, "sandbox-up", cancel) {
		cancel()
		return SandboxUpResult{}, fmt.Errorf("a sandbox creation for this project is already running")
	}
	defer cancel()
	defer FinishOp(opID)
	preview, err := spendDeployPreviewToken(token, deployPreviewSandboxUp, root)
	if err != nil {
		return SandboxUpResult{}, err
	}
	profile, recreate := parseSandboxUpFingerprint(preview.fingerprint)
	request, err := buildSandboxRequest(cfg, root, profile, recreate, false, false, io.Discard)
	if err != nil {
		return SandboxUpResult{}, err
	}
	ctx, stop := context.WithTimeout(opCtx, deployMutateTimeout)
	defer stop()

	runtime := deploySandboxRuntimeForDesktop()
	// The creation the preview authorised names one shape: re-read the
	// container first, because one built for another profile since the
	// preview must be refused with the recreate hint, not reshaped. This
	// re-runs the preview-time profile check at apply, mirroring the
	// identity re-check SandboxDown runs before it removes.
	current, err := deploy.SandboxStatus(ctx, runtime, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: cfg.ProjectName,
	})
	if err != nil {
		return SandboxUpResult{}, err
	}
	if current.Exists && request.ProfileExplicit && !recreate && strings.TrimSpace(current.Profile) != "" &&
		!strings.EqualFold(strings.TrimSpace(current.Profile), strings.TrimSpace(request.Profile)) {
		return SandboxUpResult{}, fmt.Errorf(
			"the sandbox container %s was built for the %s profile; changing it means a new image, so preview again with --recreate --profile %s",
			current.Container, current.Profile, strings.TrimSpace(request.Profile))
	}
	state, err := deploy.SandboxUp(ctx, runtime, deploy.LocalRunner{}, request)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return sandboxUpCancelResult(cfg.ProjectName, sandboxCancelStateNote(ctx, runtime, root, cfg.ProjectName)), nil
		}
		return SandboxUpResult{}, err
	}
	return SandboxUpResult{
		Container:  sanitizeUIText(state.Container, deployNameMax),
		Project:    sanitizeUIText(cfg.ProjectName, deployNameMax),
		Profile:    sanitizeUIText(state.Profile, deployNameMax),
		PHP:        sanitizeUIText(state.PHP, deployNameMax),
		RemoteName: sanitizeUIText(state.RemoteName, deployNameMax),
		Port:       state.Port,
		WebPort:    state.WebPort,
		Running:    state.Running,
		Exists:     state.Exists,
	}, nil
}

// PreviewSandboxDown names the container an apply would remove and mints
// the single-use token that authorises exactly that removal. It changes
// nothing.
func (s *DeployService) PreviewSandboxDown(project string, purge bool, volumes bool) (res SandboxDownPreview, err error) {
	defer RecoverPanic(&err, "PreviewSandboxDown")
	root, cfg, err := resolveSandboxContext(project)
	if err != nil {
		return SandboxDownPreview{}, err
	}
	runtime := deploySandboxRuntimeForDesktop()
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployReadTimeout)
	defer cancel()
	state, err := deploy.SandboxStatus(ctx, runtime, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: cfg.ProjectName,
	})
	if err != nil {
		return SandboxDownPreview{}, err
	}
	token, err := mintDeployPreviewToken(deployPreviewSandboxDown, root, "", state.Container, sandboxDownFingerprint(purge, volumes))
	if err != nil {
		return SandboxDownPreview{}, err
	}
	return SandboxDownPreview{
		Token:     token,
		Project:   sanitizeUIText(cfg.ProjectName, deployNameMax),
		Container: sanitizeUIText(state.Container, deployNameMax),
		Profile:   sanitizeUIText(state.Profile, deployNameMax),
		Purge:     purge,
		Volumes:   volumes,
		Exists:    state.Exists,
	}, nil
}

// SandboxDown spends a down token and removes the container it authorised,
// through the same gate and registry as the creation.
func (s *DeployService) SandboxDown(project string, token string, confirm bool) (res SandboxDownResult, err error) {
	defer RecoverPanic(&err, "SandboxDown")
	if !confirm {
		return SandboxDownResult{}, fmt.Errorf("confirm the sandbox removal explicitly before applying: pass confirm with a fresh preview token")
	}
	root, cfg, err := resolveSandboxContext(project)
	if err != nil {
		return SandboxDownResult{}, err
	}
	opID := "sandbox-down:" + root
	opCtx, cancel := context.WithCancel(s.lifecycleContext())
	if !RegisterOp(opID, root, "sandbox-down", cancel) {
		cancel()
		return SandboxDownResult{}, fmt.Errorf("a sandbox removal for this project is already running")
	}
	defer cancel()
	defer FinishOp(opID)
	preview, err := spendDeployPreviewToken(token, deployPreviewSandboxDown, root)
	if err != nil {
		return SandboxDownResult{}, err
	}
	purge, volumes := parseSandboxDownFingerprint(preview.fingerprint)
	request, err := buildSandboxRequest(cfg, root, "", false, purge, volumes, io.Discard)
	if err != nil {
		return SandboxDownResult{}, err
	}
	ctx, stop := context.WithTimeout(opCtx, deployMutateTimeout)
	defer stop()

	runtime := deploySandboxRuntimeForDesktop()
	// The removal the preview authorised names one container: re-resolve it
	// first, because a project renamed since the preview resolves another.
	// A preview that saw nothing removes nothing, so only a named container
	// is revalidated.
	if strings.TrimSpace(preview.detail) != "" {
		current, err := deploy.SandboxStatus(ctx, runtime, deploy.SandboxRequest{
			ProjectRoot: root,
			ProjectName: cfg.ProjectName,
		})
		if err != nil {
			return SandboxDownResult{}, err
		}
		if strings.TrimSpace(current.Container) != strings.TrimSpace(preview.detail) {
			return SandboxDownResult{}, fmt.Errorf("the sandbox changed since the preview: request a fresh preview before applying")
		}
	}
	state, err := deploy.SandboxDown(ctx, runtime, request)
	if err != nil {
		if ctx.Err() == context.Canceled {
			container := deploy.SandboxContainerName(cfg.ProjectName, root)
			return sandboxDownCancelResult(container, sandboxCancelStateNote(ctx, runtime, root, cfg.ProjectName)), nil
		}
		return SandboxDownResult{}, err
	}
	return SandboxDownResult{
		Container: sanitizeUIText(state.Container, deployNameMax),
		Exists:    state.Exists,
		Running:   state.Running,
	}, nil
}

// sandboxSSHSession resolves the live session facts for one project. It is
// the shared read behind the info DTO and the terminal launcher, so both
// describe the same session.
func (s *DeployService) sandboxSSHSession(project string) (root string, info SandboxSSHInfo, err error) {
	root, cfg, err := resolveSandboxContext(project)
	if err != nil {
		return "", SandboxSSHInfo{}, err
	}
	runtime := deploySandboxRuntimeForDesktop()
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployReadTimeout)
	defer cancel()
	request := deploy.SandboxRequest{ProjectRoot: root, ProjectName: cfg.ProjectName}
	state, err := deploy.SandboxStatus(ctx, runtime, request)
	if err != nil {
		return "", SandboxSSHInfo{}, err
	}
	if !state.Exists {
		return "", SandboxSSHInfo{}, fmt.Errorf("no sandbox exists for this project yet: preview a creation first, then open a session")
	}
	if !state.Running {
		return "", SandboxSSHInfo{}, fmt.Errorf("the sandbox is not running: start it first, then open a session")
	}
	// The key path is reused from the sandbox record, never its contents: the
	// command carries -i with the path, exactly like the CLI session.
	keyPath := strings.TrimSpace(state.KeyPath)
	if keyPath == "" {
		keyPath = filepath.Join(deploy.SandboxStateDir(root), deploy.SandboxKeyName)
	}
	argv := append([]string{"ssh"}, deploy.SandboxSSHArgs(request, state)...)
	quoted := make([]string, 0, len(argv))
	for _, word := range argv {
		quoted = append(quoted, deploy.Shell(word))
	}
	return root, SandboxSSHInfo{
		User:    sanitizeUIText(deploy.SandboxUser, deployNameMax),
		Host:    sanitizeUIText("127.0.0.1", deployNameMax),
		KeyPath: sanitizeUIText(keyPath, deployGeneralMax),
		Command: sanitizeUIText(strings.Join(quoted, " "), deployCommandMax),
		Port:    state.Port,
		Exists:  state.Exists,
		Running: state.Running,
	}, nil
}

// SandboxSSHinfo reports how to reach the rehearsed sandbox: the facts the
// existing terminal path opens with. It reads only, so it needs no token,
// and it carries the key path, never key material. There is no embedded
// terminal: the session opens in the operator's own terminal.
func (s *DeployService) SandboxSSHinfo(project string) (res SandboxSSHInfo, err error) {
	defer RecoverPanic(&err, "SandboxSSHinfo")
	_, info, err := s.sandboxSSHSession(project)
	if err != nil {
		return SandboxSSHInfo{}, err
	}
	return info, nil
}

// OpenSandboxSSHInTerminal opens the sandbox session through the existing OS
// terminal path, the same launcher the remote shell uses. It starts a local
// terminal only and changes no managed state, so it needs no token.
func (s *DeployService) OpenSandboxSSHInTerminal(project string) (res string, err error) {
	defer RecoverPanic(&err, "OpenSandboxSSHInTerminal")
	root, info, err := s.sandboxSSHSession(project)
	if err != nil {
		return "", err
	}
	if err := launchInTerminalForDesktop(root, info.Command); err != nil {
		return "", fmt.Errorf("open the sandbox session in a terminal: %w", err)
	}
	return "Opening the sandbox session in a terminal...", nil
}

// ListReleases lists the releases on one remote, newest first. An empty
// target is an empty slice, not an error. An unreachable target yields a
// typed error carrying a retry hint.
func (s *DeployService) ListReleases(project string, remote string) (res []DeployReleaseEntry, err error) {
	defer RecoverPanic(&err, "ListReleases")
	_, cfg, name, err := resolveDeployContext(project, remote)
	if err != nil {
		return nil, err
	}
	opts, err := deploy.ResolveReadOptions(cfg, name, deploy.Overrides{})
	if err != nil {
		return nil, err
	}
	host, err := resolveDeployHost(cfg, name, opts, "deploy releases")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), deployReadTimeout)
	defer cancel()
	listFn := deployListReleasesForDesktop
	entries, err := awaitDeployCall(ctx, name, "deploy releases", func() ([]deploy.ReleaseEntry, error) {
		return listFn(ctx, host)
	})
	if err != nil {
		return nil, wrapDeployError(name, "deploy releases", err)
	}
	out := make([]DeployReleaseEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, DeployReleaseEntry{
			Release:   sanitizeUIText(entry.Release, deployNameMax),
			Revision:  sanitizeUIText(entry.Revision, deployGeneralMax),
			Branch:    sanitizeUIText(entry.Branch, deployNameMax),
			Status:    sanitizeUIText(entry.Status, deployNameMax),
			CreatedAt: sanitizeUIText(entry.CreatedAt, deployNameMax),
			CreatedBy: sanitizeUIText(entry.CreatedBy, deployNameMax),
			Live:      entry.Live,
			Foreign:   entry.Foreign,
			Number:    entry.Number,
		})
	}
	return out, nil
}
