package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/tunnel"
	"govard/internal/frameworks"
	"govard/internal/proxy"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

type tunnelCommandDependencies struct {
	NewProvider     func(ref engine.ProviderRef) (tunnel.Provider, error)
	RunCommand      func(command *exec.Cmd) error
	ReadProcessArgv func(pid int) (string, bool)
	ProcessAlive    func(pid int) bool
	SignalProcess   func(pid int, sig os.Signal) error
	Now             func() time.Time
	// Sleep is the wait between liveness polls while a tunnel shuts down. It is
	// a dependency so a test can control *when* the grace period expires (with
	// Now) without also paying for it in real time. Those are two separate
	// decisions: a test that can only move the clock still has to sit through
	// every sleep that clock would have skipped.
	Sleep func(d time.Duration)
}

var tunnelDeps = tunnelCommandDependencies{
	NewProvider: tunnel.NewProvider,
	RunCommand: func(command *exec.Cmd) error {
		return command.Run()
	},
	ReadProcessArgv: readProcessArgv,
	ProcessAlive:    processAlive,
	SignalProcess:   signalProcess,
	Now:             time.Now,
	Sleep:           time.Sleep,
}

// TunnelDependenciesForTest allows tests to swap tunnel command dependencies.
type TunnelDependenciesForTest struct {
	NewProvider     func(ref engine.ProviderRef) (tunnel.Provider, error)
	RunCommand      func(command *exec.Cmd) error
	ReadProcessArgv func(pid int) (string, bool)
	ProcessAlive    func(pid int) bool
	SignalProcess   func(pid int, sig os.Signal) error
	Now             func() time.Time
	Sleep           func(d time.Duration)
}

var tunnelCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapCloudflared),
	},
	Use:   "tunnel",
	Short: "Manage local project tunnels",
	Long: `Manage local project tunnels. Tunnels allow you to securely
expose your local environment to the internet via Cloudflare Tunnels.

Note: This command requires the 'cloudflared' binary to be installed on your host.
You can install it via the official Cloudflare repository or by downloading it
from: https://github.com/cloudflare/cloudflared/releases`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var tunnelStartCmd = &cobra.Command{
	Use:   "start [url]",
	Short: "Start a public tunnel to the local project",
	Long: `Start a new public tunnel session. This command will launch 'cloudflared'
and automatically update your project's base URL (e.g. for Magento or Laravel)
to match the temporary tunnel URL. When you stop the tunnel (Ctrl+C), the
original base URL will be restored.
Give the target either as the positional url or as --url, not both.

Prerequisite: You must have 'cloudflared' installed and available in your PATH.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		startedAt := time.Now()
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		cwd, _ := os.Getwd()
		operationStatus := engine.OperationStatusFailure
		operationMessage := ""
		operationCategory := ""

		defer func() {
			if err != nil && operationMessage == "" {
				operationMessage = err.Error()
			}
			if err == nil && operationStatus == engine.OperationStatusFailure {
				operationStatus = engine.OperationStatusSuccess
			}
			if err != nil && operationCategory == "" {
				operationCategory = classifyTunnelError(err)
			}
			writeOperationEventBestEffort(
				"tunnel.start",
				operationStatus,
				config,
				"",
				"",
				operationMessage,
				operationCategory,
				time.Since(startedAt),
			)
			if err == nil {
				trackProjectRegistryBestEffort(config, cwd, "tunnel-start")
			}
		}()

		providerName, _ := cmd.Flags().GetString("provider")
		targetFlag, _ := cmd.Flags().GetString("url")
		noTLSVerify, _ := cmd.Flags().GetBool("no-tls-verify")
		planOnly, _ := cmd.Flags().GetBool("plan")

		targetURL, err := resolveTunnelTarget(config, targetFlag, args)
		if err != nil {
			return err
		}

		provider, err := tunnelDeps.NewProvider(engine.ProviderRef{
			Kind: engine.ProviderKindTunnel,
			Name: providerName,
		})
		if err != nil {
			return err
		}

		// We will NOT override the Host header for tunnels by default.
		// Instead, we will register the tunnel domain in Caddy as an alias.
		// This keeps the Host header intact so applications (like Magento) don't get confused.
		var hostHeader string

		plan, err := provider.BuildStartPlan(tunnel.StartOptions{
			TargetURL:   targetURL,
			NoTLSVerify: noTLSVerify,
			HostHeader:  hostHeader,
		})
		if err != nil {
			return err
		}

		if planOnly {
			fmt.Fprintln(cmd.OutOrStdout(), "Tunnel Plan")
			fmt.Fprintf(cmd.OutOrStdout(), "provider: %s\n", provider.Name())
			fmt.Fprintf(cmd.OutOrStdout(), "target: %s\n", targetURL)
			fmt.Fprintf(cmd.OutOrStdout(), "command: %s\n", plan.CommandString())
			operationStatus = engine.OperationStatusPlan
			operationMessage = "tunnel plan generated"
			return nil
		}

		pterm.Info.Printf("Starting tunnel provider '%s' to %s. Press Ctrl+C to stop.\n", provider.Name(), targetURL)

		// #469: one process per record. A tunnel govard already started is not
		// replaced by a second one behind the operator's back.
		record, recorded, err := readTunnelPIDRecord(config.ProjectName)
		if err != nil {
			return err
		}
		if recorded {
			if recordIsLiveAndOwned(record, tunnelDeps.ProcessAlive, tunnelDeps.ReadProcessArgv) {
				return fmt.Errorf(
					"govard already started a tunnel for %q (pid %d): run 'govard tunnel stop' first",
					config.ProjectName, record.PID)
			}
			// The recorded process is gone, or its pid was recycled; either way
			// this record points at nothing govard owns.
			clearTunnelPID(config.ProjectName)
		}

		mgr := frameworks.NewBaseURLManager(config.Framework)
		if err := mgr.Backup(cwd, config); err != nil {
			pterm.Warning.Printf("Failed to backup base URL: %v\n", err)
		}

		process := exec.Command(plan.Binary, plan.Args...)
		process.Env = append(os.Environ(), plan.Env...)
		process.Stdin = cmd.InOrStdin()

		// Capture stderr to find the tunnel URL
		stderr, _ := process.StderrPipe()
		process.Stdout = cmd.OutOrStdout()

		if err := process.Start(); err != nil {
			return fmt.Errorf("failed to start tunnel provider %s: %w", provider.Name(), err)
		}

		// Handle Revert on exit
		var tunnelHost string
		defer func() {
			// The record lives exactly as long as the process it names, so a
			// crash, a Ctrl+C or a `tunnel stop` all leave none behind.
			clearTunnelPID(config.ProjectName)
			if tunnelHost != "" {
				pterm.Info.Printf("Cleaning up tunnel alias for %s...\n", tunnelHost)
				_ = proxy.UnregisterDomain(tunnelHost)
			}
			pterm.Info.Println("Reverting base URL...")
			if rerr := mgr.Revert(cwd, config); rerr != nil {
				pterm.Warning.Printf("Failed to revert base URL: %v\n", rerr)
			}
		}()

		// Record the pid and the argv it was started with, so `tunnel stop` can
		// reach exactly this process instead of matching a name on the host.
		argv := append([]string{plan.Binary}, plan.Args...)
		if resolved, lookErr := exec.LookPath(plan.Binary); lookErr == nil {
			argv[0] = resolved
		}
		if err := recordTunnelPID(config.ProjectName, process.Process.Pid, argv); err != nil {
			// A tunnel govard cannot stop is not a tunnel worth leaving running.
			_ = process.Process.Kill()
			_ = process.Wait()
			return err
		}

		// Monitor stderr for the tunnel URL
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				fmt.Fprintln(cmd.ErrOrStderr(), line)

				if strings.Contains(line, ".trycloudflare.com") {
					// Extract URL
					parts := strings.Fields(line)
					for _, p := range parts {
						if strings.HasPrefix(p, "https://") && strings.Contains(p, ".trycloudflare.com") {
							pterm.Success.Printf("Tunnel URL detected: %s\n", p)
							if parsed, perr := url.Parse(p); perr == nil {
								tunnelHost = parsed.Host
								webContainer := fmt.Sprintf("%s%s", config.ProjectName, conventions.WebSuffix)
								pterm.Info.Printf("Registering tunnel alias %s -> %s...\n", tunnelHost, webContainer)
								_ = proxy.RegisterDomain(tunnelHost, webContainer)
							}

							pterm.Info.Println("Updating application base URL...")
							if uerr := mgr.Update(cwd, config, p); uerr != nil {
								pterm.Warning.Printf("Failed to update base URL: %v\n", uerr)
							}
							break
						}
					}
				}
			}
		}()

		if err := process.Wait(); err != nil {
			// Ctrl+C (SIGINT) and this repo's own `tunnel stop` (SIGTERM) both
			// end the session on purpose; neither is a failure.
			if isInterruptExit(err) || isTerminationExit(err) {
				return nil
			}
			return fmt.Errorf("tunnel provider %s failed: %w", provider.Name(), err)
		}

		operationStatus = engine.OperationStatusSuccess
		operationMessage = "tunnel session completed"
		pterm.Success.Println("Tunnel session completed.")
		return nil
	},
}

var tunnelStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running tunnel provider",
	Long: `Stop the running tunnel provider and restore the project's base URL.

Only the tunnel govard started for this project is stopped: it is the process
recorded under $GOVARD_HOME_DIR/tunnels/<project>.pid, and govard signals it only
while the process still carries the argv it was started with. A pid govard did
not start — or one whose argv cannot be read — is refused with an error instead
of being matched by name, so no other cloudflared on this host is touched.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// The record is keyed by project_name, so the config has to be readable
		// before anything can be signalled. cloudflared does not usually run in
		// the background unless told to, but the pid govard started is still
		// worth reaching: it is the one process this project owns.
		config, err := loadFullConfig()
		if err != nil {
			return fmt.Errorf("cannot tell which tunnel to stop without the project config: %w", err)
		}

		record, recorded, err := readTunnelPIDRecord(config.ProjectName)
		if err != nil {
			return err
		}

		if recorded {
			pterm.Info.Println("Stopping tunnel provider...")
			if err := signalRecordedTunnel(
				config.ProjectName,
				tunnelDeps.ReadProcessArgv,
				tunnelDeps.ProcessAlive,
				tunnelDeps.Now,
			); err != nil {
				// Refused: nothing was signalled, so the base URL must keep
				// pointing at a tunnel that is still up.
				return err
			}
		} else {
			pterm.Info.Println("No tunnel started by govard for this project.")
		}

		cwd, _ := os.Getwd()
		mgr := frameworks.NewBaseURLManager(config.Framework)
		pterm.Info.Println("Reverting base URL...")
		_ = mgr.Revert(cwd, config)

		if recorded {
			pterm.Success.Printf("Tunnel stopped (pid %d).\n", record.PID)
		}
		return nil
	},
}

var tunnelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check tunnel status",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {
			return fmt.Errorf("cannot tell which tunnel to report on without the project config: %w", err)
		}

		record, recorded, err := readTunnelPIDRecord(config.ProjectName)
		if err != nil {
			return err
		}
		if !recorded {
			pterm.Info.Println("Tunnel is INACTIVE.")
			return nil
		}
		if !tunnelDeps.ProcessAlive(record.PID) {
			// Nothing is running under that pid: the record is what is stale.
			clearTunnelPID(config.ProjectName)
			pterm.Info.Println("Tunnel is INACTIVE (stale record removed).")
			return nil
		}
		observed, readable := tunnelDeps.ReadProcessArgv(record.PID)
		if !readable || !tunnelArgvMatches(record.Argv, observed) {
			// "Govard has no tunnel here" is true even when something else owns
			// that pid, so this reports INACTIVE rather than erroring.
			pterm.Info.Println("Tunnel is INACTIVE.")
			return nil
		}
		pterm.Success.Printf("Tunnel is ACTIVE (pid %d).\n", record.PID)
		return nil
	},
}

func init() {
	tunnelStartCmd.Flags().String("provider", "cloudflare", "Tunnel provider (cloudflare)")
	tunnelStartCmd.Flags().String("url", "", "Target URL to expose (defaults to https://<domain> from config)")
	tunnelStartCmd.Flags().Bool("no-tls-verify", true, "Disable TLS verification against the target URL (verification stays on with --no-tls-verify=false)")
	tunnelStartCmd.Flags().Bool("plan", false, "Print tunnel execution plan and exit")
	tunnelCmd.AddCommand(tunnelStartCmd)
	tunnelCmd.AddCommand(tunnelStopCmd)
	tunnelCmd.AddCommand(tunnelStatusCmd)
}

func resolveTunnelTarget(config engine.Config, targetFlag string, args []string) (string, error) {
	targetArg := ""
	if len(args) > 0 {
		targetArg = strings.TrimSpace(args[0])
	}
	trimmedFlag := strings.TrimSpace(targetFlag)
	if trimmedFlag != "" && targetArg != "" {
		return "", fmt.Errorf("specify either positional [url] or --url, not both")
	}

	switch {
	case trimmedFlag != "":
		return trimmedFlag, nil
	case targetArg != "":
		return targetArg, nil
	}

	domain := strings.TrimSpace(config.Domain)
	if domain == "" {
		return "", fmt.Errorf("domain is required to infer tunnel URL; set domain in .govard.yml or pass --url")
	}
	if strings.HasPrefix(strings.ToLower(domain), "http://") || strings.HasPrefix(strings.ToLower(domain), "https://") {
		return domain, nil
	}
	return "https://" + domain, nil
}

func classifyTunnelError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "specify either positional"),
		strings.Contains(message, "target url"),
		strings.Contains(message, "domain is required"),
		strings.Contains(message, "unsupported"),
		strings.Contains(message, "provider"):
		return "validation"
	default:
		return "runtime"
	}
}

// SetTunnelDependenciesForTest swaps tunnel command dependencies and returns a restore callback.
func SetTunnelDependenciesForTest(deps TunnelDependenciesForTest) func() {
	previous := tunnelDeps
	if deps.NewProvider != nil {
		tunnelDeps.NewProvider = deps.NewProvider
	} else {
		tunnelDeps.NewProvider = tunnel.NewProvider
	}
	if deps.RunCommand != nil {
		tunnelDeps.RunCommand = deps.RunCommand
	} else {
		tunnelDeps.RunCommand = func(command *exec.Cmd) error { return command.Run() }
	}
	if deps.ReadProcessArgv != nil {
		tunnelDeps.ReadProcessArgv = deps.ReadProcessArgv
	} else {
		tunnelDeps.ReadProcessArgv = readProcessArgv
	}
	if deps.ProcessAlive != nil {
		tunnelDeps.ProcessAlive = deps.ProcessAlive
	} else {
		tunnelDeps.ProcessAlive = processAlive
	}
	if deps.SignalProcess != nil {
		tunnelDeps.SignalProcess = deps.SignalProcess
	} else {
		tunnelDeps.SignalProcess = signalProcess
	}
	if deps.Now != nil {
		tunnelDeps.Now = deps.Now
	} else {
		tunnelDeps.Now = time.Now
	}
	if deps.Sleep != nil {
		tunnelDeps.Sleep = deps.Sleep
	} else {
		tunnelDeps.Sleep = time.Sleep
	}
	return func() {
		tunnelDeps = previous
	}
}
