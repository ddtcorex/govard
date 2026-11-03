package try

import (
	"bufio"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"govard/internal/cli"
	"govard/internal/conventions"
	"govard/internal/engine"
)

// Options is everything a trial needs from the command line.
type Options struct {
	Framework string
	Dir       string
	Keep      bool
	Sample    bool
	Setup     bool
	Timeout   time.Duration
	Open      bool
}

// Dependencies are the side effects a trial performs. Every field is optional:
// a nil field keeps the production implementation, so a test overrides only the
// one it is about.
type Dependencies struct {
	RunSelf          func(ctx context.Context, dir string, args ...string) error
	Probe            func(ctx context.Context, origin Origin, timeout time.Duration) error
	DeleteProject    func(ctx context.Context, dir string) error
	CollectArtifacts func(name, root string, profiles []string) engine.ProjectArtifacts
	ProxyUp          func() error
	FreeDisk         func(path string) (uint64, error)
	Now              func() time.Time
	Signal           <-chan os.Signal
	Stdin            io.Reader
	OpenURL          func(url string) error
}

// runner carries the resolved dependencies through one trial.
type runner struct {
	deps Dependencies
}

// Run creates a throwaway workspace, runs the real bootstrap and env up inside
// it, waits for the storefront, then keeps or removes it.
func Run(ctx context.Context, o Options, d Dependencies) error {
	r := &runner{deps: withDefaults(d)}

	return r.run(ctx, o)
}

func withDefaults(d Dependencies) Dependencies {
	if d.RunSelf == nil {
		d.RunSelf = runSelfCommand
	}
	if d.Probe == nil {
		d.Probe = probeOrigin
	}
	if d.DeleteProject == nil {
		d.DeleteProject = deleteProject
	}
	if d.CollectArtifacts == nil {
		d.CollectArtifacts = engine.CollectProjectArtifacts
	}
	if d.ProxyUp == nil {
		d.ProxyUp = checkProxyReachable
	}
	if d.FreeDisk == nil {
		d.FreeDisk = freeBytes
	}
	if d.Now == nil {
		d.Now = time.Now
	}

	return d
}

const (
	// minTryFreeBytes is the floor a trial needs before it is allowed to pull
	// images and download a storefront. Failing here beats failing halfway.
	minTryFreeBytes uint64 = 8 << 30

	// proxyHTTPSAddr is where the Govard proxy terminates TLS. Readiness is
	// measured against it directly and verified against the Govard local CA
	// alone, so neither the local resolver nor the system CA store can turn a
	// healthy app into a reported failure.
	proxyHTTPSAddr = "127.0.0.1:443"

	tryProbeInterval    = 2 * time.Second
	tryProxyDialTimeout = 3 * time.Second

	// tryCleanupTimeout bounds the teardown, which runs on a context detached
	// from the one Ctrl-C cancels.
	tryCleanupTimeout = 2 * time.Minute
)

// workspace is where a trial builds, and whether Govard created it.
// A workspace the user named with --dir is never deleted: Govard did not make
// it, so it is not Govard's to remove.
type workspace struct {
	path      string
	generated bool
}

// runTry creates a throwaway workspace, runs the real bootstrap and env up
// inside it, waits for the storefront, then keeps or removes it.
func (r *runner) run(ctx context.Context, o Options) error {
	ws, err := r.prepareWorkspace(o)
	if err != nil {
		return err
	}

	if err := r.ensureTryFreeSpace(ws.path); err != nil {
		return err
	}

	// The proxy, its CA and the local domain all come from one `svc up`. Check
	// before downloading a storefront, not after: the decisive failure on a
	// machine that has never run Govard used to arrive minutes later.
	if err := r.deps.ProxyUp(); err != nil {
		return fmt.Errorf("the Govard proxy is not answering on %s: %w\n\n"+
			"Start the proxy, its local CA and the local domains with:\n"+
			"  govard try --setup\n"+
			"(the same step is available as: govard svc up)", proxyHTTPSAddr, err)
	}

	if err := r.steps(ctx, o, ws); err != nil {
		return fmt.Errorf("%w\n\nworkspace kept at %s\nremove it with: govard project delete %s", err, ws.path, ws.path)
	}

	return nil
}

func (r *runner) steps(ctx context.Context, o Options, ws workspace) error {
	if o.Setup {
		fmt.Println("Running the one-time setup:")
		fmt.Println("  starts the Govard proxy, exports its root CA and trusts it in the system store (sudo),")
		fmt.Println("  and configures local .test resolution.")
		if err := r.deps.RunSelf(ctx, ws.path, "svc", "up"); err != nil {
			return fmt.Errorf("run the one-time setup: %w", err)
		}
	}

	fmt.Println(tryETALine(o.Sample))

	bootstrapArgs := []string{"bootstrap", "--fresh", "--framework", o.Framework, "--yes"}
	if o.Sample {
		bootstrapArgs = append(bootstrapArgs, "--include-sample")
	}
	if err := r.deps.RunSelf(ctx, ws.path, bootstrapArgs...); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	if err := r.deps.RunSelf(ctx, ws.path, "env", "up"); err != nil {
		return fmt.Errorf("start the environment: %w", err)
	}

	domain := tryDomain(ws.path)
	origin := ResolveOrigin(domain, engine.IsDomainResolvableLocally, caFilePresent)

	fmt.Printf("Waiting for %s to answer ...\n", domain)
	if err := r.deps.Probe(ctx, origin, o.Timeout); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			fmt.Println("\nInterrupted while waiting for the storefront.")

			return r.finishTrial(ctx, o, ws)
		}
		if errors.Is(err, ErrTLSVerification) {
			return fmt.Errorf("%w\n\nre-trust the Govard CA and retry: govard doctor trust", err)
		}

		return fmt.Errorf("the storefront did not answer within %s (is the Govard proxy running? start it with: govard svc up): %w",
			o.Timeout, err)
	}

	printTryOrigins(origin)

	if o.Open && origin.Resolvable && r.deps.OpenURL != nil {
		if err := r.deps.OpenURL(origin.URL); err != nil {
			fmt.Printf("Could not open a browser: %v\n", err)
		}
	}

	if interrupted := r.waitForExit(ctx, o.Timeout); interrupted {
		fmt.Println("\nInterrupted.")
	} else {
		fmt.Printf("\nTrial timeout (%s) reached.\n", o.Timeout)
	}

	return r.finishTrial(ctx, o, ws)
}

// finishTrial asks whether to keep the workspace and, unless told to keep it,
// tears the trial down. It runs on every exit path including Ctrl-C, because a
// trial the user interrupted still owns containers and a workspace.
func (r *runner) finishTrial(ctx context.Context, o Options, ws workspace) error {
	if r.keepTrial(o) {
		fmt.Printf("\nKept at %s\nresume with: cd %s && govard env up\n", ws.path, ws.path)
		return nil
	}

	if err := r.removeTrial(ctx, ws); err != nil {
		return err
	}
	fmt.Println("\nTrial removed.")
	if !ws.generated {
		fmt.Printf("Workspace kept at %s (you named this directory, so Govard leaves it in place).\n", ws.path)
	}

	return nil
}

// tryDomain reads the project domain the bootstrap wrote, falling back to a
// name derived from the workspace so a half-built trial still reports something.
func tryDomain(workspace string) string {
	if cfg, _, err := engine.LoadConfigFromDirWithProfile(workspace, true, ""); err == nil {
		if domain := strings.TrimSpace(cfg.Domain); domain != "" {
			return domain
		}
	}

	return filepath.Base(workspace) + ".test"
}

func printTryOrigins(origin Origin) {
	if !origin.Resolvable {
		fmt.Printf("\nThe storefront is running, but %s does not resolve on this machine.\n", origin.HostHeader)
		fmt.Println(tryHostsHint(origin.HostHeader))
	} else {
		fmt.Printf("\nStorefront: %s\n", origin.URL)
		if !origin.Trusted {
			fmt.Println("Your browser will warn about the certificate until you trust it: govard doctor trust")
		}
		fmt.Printf("Admin:      %s/admin\n", origin.URL)
	}
	fmt.Printf("Login:      %s / %s\n", conventions.DefaultAdminUser, conventions.DefaultAdminPassword)
}

// tryETALine states the time the two paths actually take. Measured runs vary
// with Composer download speed (6m25s to 9m11s without sample data, 10m47s to
// 10m54s with it), so the estimate is a range rather than a promise.
func tryETALine(sample bool) string {
	if sample {
		return "This takes about 10 minutes on a warm machine, sometimes 15 when Composer is slow. --no-sample skips the sample data and saves a few minutes."
	}

	return "This takes about 6 to 10 minutes on a warm machine; Composer download speed decides most of it."
}

// tryHostsHint is the exact command that makes a trial reachable when the
// domain is missing from the hosts file.
func tryHostsHint(domain string) string {
	return fmt.Sprintf("Add it with: echo '127.0.0.1 %s' | sudo tee -a /etc/hosts", domain)
}

// caFilePresent reports whether the Govard root CA has been generated. It is a
// best-effort signal: the file existing means the setup ran at least once, not
// that every browser trusts it.
func caFilePresent(string) bool {
	_, err := os.Stat(filepath.Join(engine.GovardHomeDir(), "ssl", "root.crt"))
	return err == nil
}

// waitForExit blocks until the user interrupts, the command context is
// cancelled, or the trial timeout expires. It reports whether the exit was an
// interruption; a timeout is a normal end of trial.
func (r *runner) waitForExit(ctx context.Context, timeout time.Duration) bool {
	sig := r.deps.Signal
	if sig == nil {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(ch)
		sig = ch
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-sig:
		return true
	case <-timer.C:
		return false
	}
}

// keepTrial asks whether to keep the workspace. An empty answer removes it:
// the default for a trial is leaving nothing behind.
func (r *runner) keepTrial(o Options) bool {
	if o.Keep {
		return true
	}

	stdin := r.deps.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	fmt.Print("\nKeep this trial? [y/N] ")
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// ensureTryFreeSpace measures the filesystem that will hold the workspace,
// resolving the nearest directory that exists: on a machine that has never run
// Govard, the workspace parent does not exist yet and statfs refuses it.
func (r *runner) ensureTryFreeSpace(path string) error {
	existing, err := nearestExistingDir(path)
	if err != nil {
		return fmt.Errorf("check free space under %s: %w", path, err)
	}

	free, err := r.deps.FreeDisk(existing)
	if err != nil {
		return fmt.Errorf("check free space under %s: %w", path, err)
	}
	if free < minTryFreeBytes {
		return fmt.Errorf("try needs at least 8 GiB free under %s, found %.1f GiB", path, float64(free)/float64(1<<30))
	}

	return nil
}

func nearestExistingDir(path string) (string, error) {
	dir := filepath.Clean(path)
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if info.IsDir() {
				return dir, nil
			}
			return filepath.Dir(dir), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return dir, nil
		}
		dir = parent
	}
}

func (r *runner) prepareWorkspace(o Options) (workspace, error) {
	if o.Dir != "" {
		abs, err := filepath.Abs(o.Dir)
		if err != nil {
			return workspace{}, fmt.Errorf("resolve try workspace: %w", err)
		}
		// Checked before touching the directory: a registered project must be
		// reported as such, not as "not empty".
		if err := assertNotRegistered(abs); err != nil {
			return workspace{}, err
		}
		if err := PrepareDir(abs); err != nil {
			return workspace{}, &cli.UsageError{Err: err}
		}

		return workspace{path: abs, generated: false}, nil
	}

	parent := filepath.Join(engine.GovardHomeDir(), "try")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return workspace{}, fmt.Errorf("create try workspace root: %w", err)
	}

	path, err := CreateWorkspace(parent, o.Framework, r.deps.Now())
	if err != nil {
		return workspace{}, err
	}

	return workspace{path: path, generated: true}, nil
}

// assertNotRegistered refuses a workspace that is already a Govard project, or
// whose name would take another project's domain.
func assertNotRegistered(path string) error {
	if entry, ok := engine.GetProjectRegistryEntry(path); ok && entry.Path != "" {
		return fmt.Errorf("%s is already a registered Govard project (%s); pick another path", path, entry.ProjectName)
	}

	name := filepath.Base(path)
	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.ProjectName == name && entry.Path != path {
			return fmt.Errorf("project name %q already belongs to %s; pick another path", name, entry.Path)
		}
	}

	return nil
}

// removeTrial stops the stack, drops the registration, verifies that no Docker
// resource survived, and deletes a workspace Govard created. Ctrl-C cancels the
// command context, so teardown runs detached from it: a cancelled context would
// make `docker compose down` a no-op and leave everything behind.
func (r *runner) removeTrial(ctx context.Context, ws workspace) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tryCleanupTimeout)
	defer cancel()

	if err := r.deps.DeleteProject(cleanupCtx, ws.path); err != nil {
		return fmt.Errorf("remove the trial: %w", err)
	}

	if _, still := engine.GetProjectRegistryEntry(ws.path); still {
		return fmt.Errorf("remove the trial: %s is still registered; run: govard project delete %s", ws.path, ws.path)
	}

	name := filepath.Base(ws.path)
	if entry, ok := engine.GetProjectRegistryEntry(ws.path); ok && entry.ProjectName != "" {
		name = entry.ProjectName
	}
	if artifacts := r.deps.CollectArtifacts(name, ws.path, nil); artifacts.Docker {
		return fmt.Errorf("remove the trial: Docker resources for %s survived teardown; run: govard project delete %s", name, ws.path)
	}

	if !ws.generated {
		return nil
	}
	if err := os.RemoveAll(ws.path); err != nil {
		return fmt.Errorf("remove the trial workspace %s: %w", ws.path, err)
	}

	return nil
}

// runSelfCommand re-invokes this binary, so a trial runs exactly the commands a
// user would run by hand, with their exit codes and their output. An interrupt
// is forwarded rather than killing the child outright, so the child can still
// finish its own cleanup.
func runSelfCommand(ctx context.Context, dir string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the govard binary: %w", err)
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = 10 * time.Second

	return cmd.Run()
}

func probeOrigin(ctx context.Context, origin Origin, timeout time.Duration) error {
	client, err := RefreshingProbeClient(proxyHTTPSAddr, origin.HostHeader, loadProxyRoots)
	if err != nil {
		return fmt.Errorf("cannot verify the Govard proxy certificate without its local CA (is the Govard proxy running? start it with: govard svc up): %w", err)
	}

	return WaitReady(ctx, client, origin, timeout, tryProbeInterval)
}

// loadProxyRoots trusts the Govard local CA only, re-extracting it from the
// proxy when asked to refresh a stale exported copy.
func loadProxyRoots(refresh bool) (*x509.CertPool, error) {
	if refresh {
		return engine.RefreshLocalRootCAPool()
	}
	return engine.LocalRootCAPool()
}

// checkProxyReachable reports whether anything answers where the Govard proxy
// terminates TLS.
func checkProxyReachable() error {
	conn, err := net.DialTimeout("tcp", proxyHTTPSAddr, tryProxyDialTimeout)
	if err != nil {
		return err
	}

	return conn.Close()
}

func deleteProject(ctx context.Context, dir string) error {
	return engine.DeleteProject(ctx, dir, os.Stdout, os.Stderr)
}
