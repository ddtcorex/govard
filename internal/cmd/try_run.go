package cmd

import (
	"bufio"
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/try"
)

const (
	// minTryFreeBytes is the floor a trial needs before it is allowed to pull
	// images and download a storefront. Failing here beats failing halfway.
	minTryFreeBytes uint64 = 8 << 30

	// proxyHTTPSAddr is where the Govard proxy terminates TLS. Readiness is
	// measured against it directly and verified against the Govard local CA
	// alone, so neither the local resolver nor the system CA store can turn a
	// healthy app into a reported failure.
	proxyHTTPSAddr = "127.0.0.1:443"

	tryProbeInterval = 2 * time.Second

	// tryCleanupTimeout bounds the teardown, which runs on a context detached
	// from the one Ctrl-C cancels.
	tryCleanupTimeout = 2 * time.Minute
)

// runTry creates a throwaway workspace, runs the real bootstrap and env up
// inside it, waits for the storefront, then keeps or removes it.
func runTry(ctx context.Context, o tryOptions) error {
	root := engine.GovardHomeDir()
	if o.Dir != "" {
		root = filepath.Dir(o.Dir)
	}
	if err := ensureTryFreeSpace(root); err != nil {
		return err
	}

	workspace, err := prepareWorkspace(o)
	if err != nil {
		return err
	}

	if err := runTrySteps(ctx, o, workspace); err != nil {
		return fmt.Errorf("%w\n\nworkspace kept at %s\nremove it with: govard project delete %s", err, workspace, workspace)
	}

	return nil
}

func runTrySteps(ctx context.Context, o tryOptions, workspace string) error {
	if o.Setup {
		fmt.Println("Running the one-time setup: local domains and the Govard root CA.")
		if err := tryDeps.RunSelf(ctx, workspace, "svc", "up"); err != nil {
			return fmt.Errorf("run the one-time setup: %w", err)
		}
	}

	bootstrapArgs := []string{"bootstrap", "--fresh", "--framework", o.Framework, "--yes"}
	if o.Sample {
		bootstrapArgs = append(bootstrapArgs, "--include-sample")
	}
	if err := tryDeps.RunSelf(ctx, workspace, bootstrapArgs...); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	if err := tryDeps.RunSelf(ctx, workspace, "env", "up"); err != nil {
		return fmt.Errorf("start the environment: %w", err)
	}

	domain := tryDomain(workspace)
	origin := try.ResolveOrigin(domain, engine.IsDomainResolvableLocally, caFilePresent)

	fmt.Printf("Waiting for %s to answer ...\n", domain)
	if err := tryDeps.Probe(ctx, origin, o.Timeout); err != nil {
		return fmt.Errorf("the storefront did not answer within %s (is the Govard proxy running? start it with: govard svc up): %w",
			o.Timeout, err)
	}

	printTryOrigins(origin)

	if o.Open && origin.Resolvable {
		if err := tryDeps.OpenURL(origin.URL); err != nil {
			fmt.Printf("Could not open a browser: %v\n", err)
		}
	}

	waitForExit(ctx)

	if keepTrial(o) {
		fmt.Printf("\nKept at %s\nresume with: cd %s && govard env up\n", workspace, workspace)
		return nil
	}

	if err := removeTrial(ctx, workspace); err != nil {
		return err
	}
	fmt.Println("\nTrial removed.")

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

func printTryOrigins(origin try.Origin) {
	if !origin.Resolvable {
		fmt.Printf("\n%s does not resolve on this machine.\n", origin.HostHeader)
		fmt.Println("Add it to your hosts file, or run: govard try --setup")
	} else {
		fmt.Printf("\nStorefront: %s\n", origin.URL)
		if !origin.Trusted {
			fmt.Println("Your browser will warn about the certificate until you run: govard try --setup")
		}
		fmt.Printf("Admin:      %s/admin\n", origin.URL)
	}
	fmt.Printf("Login:      %s / %s\n", conventions.DefaultAdminUser, conventions.DefaultAdminPassword)
}

// removeTrial stops the stack, drops the registration and deletes the
// workspace. Ctrl-C cancels the command context, so teardown runs detached from
// it: a cancelled context would make `docker compose down` a no-op and leave
// containers, volumes and a registry row behind.
func removeTrial(ctx context.Context, workspace string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tryCleanupTimeout)
	defer cancel()

	if err := tryDeps.DeleteProject(cleanupCtx, workspace); err != nil {
		return fmt.Errorf("remove the trial: %w", err)
	}
	if _, still := engine.GetProjectRegistryEntry(workspace); still {
		return fmt.Errorf("remove the trial: %s is still registered; run: govard project delete %s", workspace, workspace)
	}
	if err := os.RemoveAll(workspace); err != nil {
		return fmt.Errorf("remove the trial workspace %s: %w", workspace, err)
	}

	return nil
}

// caFilePresent reports whether the Govard root CA has been generated. It is a
// best-effort signal: the file existing means the setup ran at least once, not
// that every browser trusts it.
func caFilePresent(string) bool {
	_, err := os.Stat(filepath.Join(engine.GovardHomeDir(), "ssl", "root.crt"))
	return err == nil
}

func waitForExit(ctx context.Context) {
	sig := tryDeps.Signal
	if sig == nil {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(ch)
		sig = ch
	}

	select {
	case <-ctx.Done():
	case <-sig:
	}
}

// keepTrial asks whether to keep the workspace. An empty answer removes it:
// the default for a trial is leaving nothing behind.
func keepTrial(o tryOptions) bool {
	if o.Keep {
		return true
	}

	stdin := tryDeps.Stdin
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

func ensureTryFreeSpace(path string) error {
	free, err := tryDeps.FreeDisk(path)
	if err != nil {
		return fmt.Errorf("check free space under %s: %w", path, err)
	}
	if free < minTryFreeBytes {
		return fmt.Errorf("try needs at least 8 GiB free under %s, found %.1f GiB", path, float64(free)/float64(1<<30))
	}

	return nil
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

func prepareWorkspace(o tryOptions) (string, error) {
	if o.Dir != "" {
		if err := try.PrepareDir(o.Dir); err != nil {
			return "", err
		}
		abs, err := filepath.Abs(o.Dir)
		if err != nil {
			return "", fmt.Errorf("resolve try workspace: %w", err)
		}
		if err := assertNotRegistered(abs); err != nil {
			return "", err
		}
		return abs, nil
	}

	parent := filepath.Join(engine.GovardHomeDir(), "try")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", fmt.Errorf("create try workspace root: %w", err)
	}

	return try.CreateWorkspace(parent, o.Framework, tryDeps.Now())
}

// runSelfCommand re-invokes this binary, so a trial runs exactly the commands a
// user would run by hand, with their exit codes and their output.
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

	return cmd.Run()
}

func probeOrigin(ctx context.Context, origin try.Origin, timeout time.Duration) error {
	client, err := try.RefreshingProbeClient(proxyHTTPSAddr, origin.HostHeader, loadProxyRoots)
	if err != nil {
		return fmt.Errorf("cannot verify the Govard proxy certificate without its local CA (is the Govard proxy running? start it with: govard svc up): %w", err)
	}

	return try.WaitReady(ctx, client, origin, timeout, tryProbeInterval)
}

// loadProxyRoots trusts the Govard local CA only, re-extracting it from the
// proxy when asked to refresh a stale exported copy.
func loadProxyRoots(refresh bool) (*x509.CertPool, error) {
	if refresh {
		return engine.RefreshLocalRootCAPool()
	}
	return engine.LocalRootCAPool()
}

func deleteProject(ctx context.Context, dir string) error {
	return engine.DeleteProject(ctx, dir, os.Stdout, os.Stderr)
}
