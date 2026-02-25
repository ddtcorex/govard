package cmd

import (
	"fmt"
	"govard/internal/engine"
	"govard/internal/proxy"
	"govard/internal/updater"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up [flags]",
	Short: "Start the development environment",
	Long: `Starts all Docker containers required for the current project.
It automatically handles framework detection, configuration validation,
Docker Compose blueprint rendering, host mapping, and proxy registration.

The startup process follows these stages:
1. Detect: Identifies framework context from project files.
2. Validate: Checks Docker status, port conflicts, and layered config.
3. Render: Generates the specialized Docker Compose file.
4. Start: Runs 'docker compose up' in detached mode.
5. Verify: Maps the .test domain to 127.0.0.1 and registers it with the Govard Proxy.

Case Studies:
- Standard Startup: Simply run 'govard up' to get your full stack running.
- Low Resource Mode: Use --quickstart if you have limited RAM or only need PHP/Web server.
- Fresh Install Recovery: If containers are broken, 'govard up' re-renders and restarts them.`,
	Example: `  # Start the environment normally
  govard up

  # Fast startup: skip heavy services like Elasticsearch, Varnish, Redis
  govard up --quickstart`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		updater.CheckForUpdates(Version)
		pterm.DefaultHeader.Println("Govard Environment Liftoff")
		startedAt := time.Now()

		quickstart, _ := cmd.Flags().GetBool("quickstart")
		cwd, _ := os.Getwd()
		context := upRuntimeContext{
			Cwd:        cwd,
			Quickstart: quickstart,
			Out:        cmd.OutOrStdout(),
			Err:        cmd.ErrOrStderr(),
		}
		defer func() {
			status := engine.OperationStatusSuccess
			message := "up completed"
			if err != nil {
				status = engine.OperationStatusFailure
				message = err.Error()
			}
			writeOperationEventBestEffort(
				"up.run",
				status,
				context.Config,
				"",
				"",
				message,
				"",
				time.Since(startedAt),
			)
			if err == nil {
				trackProjectRegistryBestEffort(context.Config, cwd, "up")
			}
		}()
		stages := buildUpPipelineStages(cmd, &context)
		if err = runUpPipeline(stages); err != nil {
			return err
		}
		pterm.Success.Printf("Environment is up and running at https://%s\n", context.Config.Domain)
		return nil
	},
}

type upPipelineStage struct {
	Name         string
	OnFailureTip string
	Run          func() error
}

type upRuntimeContext struct {
	Cwd        string
	Config     engine.Config
	Compose    string
	Loaded     []string
	Metadata   engine.ProjectMetadata
	Quickstart bool
	Out        interface{ Write([]byte) (int, error) }
	Err        interface{ Write([]byte) (int, error) }
}

func buildUpPipelineStages(cmd *cobra.Command, context *upRuntimeContext) []upPipelineStage {
	return []upPipelineStage{
		{
			Name:         "Detect",
			OnFailureTip: "govard init",
			Run: func() error {
				context.Metadata = engine.DetectFramework(context.Cwd)
				if strings.TrimSpace(context.Metadata.Version) == "" {
					pterm.Info.Printf("Detected framework: %s\n", context.Metadata.Framework)
				} else {
					pterm.Info.Printf("Detected framework: %s (%s)\n", context.Metadata.Framework, context.Metadata.Version)
				}
				return nil
			},
		},
		{
			Name:         "Validate",
			OnFailureTip: "govard doctor",
			Run: func() error {
				var err error
				context.Config, context.Loaded, err = engine.LoadConfigFromDir(context.Cwd, true)
				if err != nil {
					return fmt.Errorf("load layered config: %w", err)
				}
				if notes := AutoTuneMagentoRuntime(&context.Config, context.Metadata); len(notes) > 0 {
					for _, note := range notes {
						pterm.Info.Println(note)
					}
				}
				if context.Quickstart {
					ApplyQuickstartProfile(&context.Config)
					pterm.Info.Println("Quickstart profile enabled: optional services reduced for faster first run.")
				}
				context.Compose = engine.ComposeFilePath(context.Cwd, context.Config.ProjectName)
				pterm.Info.Printf("Loaded config layers: %d\n", len(context.Loaded))
				lockWarnings, lockErr := evaluateUpLockPolicy(context.Cwd, context.Config)
				for _, warning := range lockWarnings {
					pterm.Warning.Println(warning)
				}
				if len(lockWarnings) > 0 {
					pterm.Warning.Println("Run `govard lock check` for full lockfile compliance details.")
				}
				if lockErr != nil {
					return lockErr
				}

				if err := engine.CheckDockerStatus(); err != nil {
					return fmt.Errorf("docker daemon is not ready: %w", err)
				}
				if err := engine.CheckDockerComposePlugin(); err != nil {
					return err
				}

				if !engine.CheckPortForGovardProxy("80") {
					pterm.Warning.Println("Port 80 is currently in use. Proxy routing may fail.")
				}
				if !engine.CheckPortForGovardProxy("443") {
					pterm.Warning.Println("Port 443 is currently in use. HTTPS proxy routing may fail.")
				}
				if err := engine.CheckDiskScratchWrite(); err != nil {
					return fmt.Errorf("disk scratch check failed: %w", err)
				}
				if err := engine.CheckNetworkConnectivity(); err != nil {
					pterm.Warning.Printf("Network outbound probe failed: %v\n", err)
				}
				return nil
			},
		},
		{
			Name:         "Render",
			OnFailureTip: "govard doctor --fix",
			Run: func() error {
				if err := engine.RunHooks(context.Config, engine.HookPreUp, context.Out, context.Err); err != nil {
					return fmt.Errorf("pre-up hooks failed: %w", err)
				}

				if err := engine.EnsureGlobalProxy(); err != nil {
					pterm.Warning.Printf("Govard Proxy not ready: %v\n", err)
				}

				if err := engine.RenderBlueprint(context.Cwd, context.Config); err != nil {
					return fmt.Errorf("render blueprint: %w", err)
				}
				pterm.Success.Printf("Rendered compose file: %s\n", context.Compose)
				return nil
			},
		},
		{
			Name:         "Start",
			OnFailureTip: "govard doctor fix-deps",
			Run: func() error {
				command := exec.Command(
					"docker",
					"compose",
					"--project-directory",
					context.Cwd,
					"-p",
					context.Config.ProjectName,
					"-f",
					context.Compose,
					"up",
					"-d",
				)
				command.Stdout, command.Stderr = context.Out, context.Err
				if err := command.Run(); err != nil {
					return fmt.Errorf("docker compose up failed: %w", err)
				}
				return nil
			},
		},
		{
			Name:         "Verify",
			OnFailureTip: "govard doctor",
			Run: func() error {
				if err := engine.AddHostsEntry(context.Config.Domain); err != nil {
					pterm.Warning.Printf("Could not update hosts file: %v\n", err)
					pterm.Info.Printf("Please manually add '127.0.0.1 %s' to your hosts file.\n", context.Config.Domain)
				} else {
					pterm.Success.Printf("Domain %s mapped to 127.0.0.1\n", context.Config.Domain)
				}

				target := ResolveUpProxyTarget(context.Config)
				if err := proxy.RegisterDomain(context.Config.Domain, target); err != nil {
					pterm.Warning.Printf("Could not register domain with Govard Proxy: %v\n", err)
				} else {
					pterm.Success.Printf("Domain %s registered with Govard Proxy -> %s\n", context.Config.Domain, target)
				}

				if err := engine.RunHooks(context.Config, engine.HookPostUp, context.Out, context.Err); err != nil {
					return fmt.Errorf("post-up hooks failed: %w", err)
				}
				return nil
			},
		},
	}
}

func runUpPipeline(stages []upPipelineStage) error {
	total := len(stages)
	for index, stage := range stages {
		step := fmt.Sprintf("[%d/%d] %s", index+1, total, stage.Name)
		pterm.Info.Printf("%s...\n", step)

		started := time.Now()
		if err := stage.Run(); err != nil {
			pterm.Error.Printf("%s failed (%s): %v\n", step, time.Since(started).Round(time.Millisecond), err)
			if stage.OnFailureTip != "" {
				pterm.Info.Printf("Suggested next command: %s\n", stage.OnFailureTip)
			}
			return err
		}
		pterm.Success.Printf("%s completed (%s)\n", step, time.Since(started).Round(time.Millisecond))
	}
	return nil
}

// ResolveUpProxyTarget resolves the upstream container for proxy registration.
func ResolveUpProxyTarget(config engine.Config) string {
	target := config.ProjectName + "-web-1"
	if config.Stack.Features.Varnish {
		target = config.ProjectName + "-varnish-1"
	}
	return target
}

// ApplyQuickstartProfile trims optional runtime services for faster first startup.
func ApplyQuickstartProfile(config *engine.Config) {
	if config == nil {
		return
	}

	config.Stack.Features.Xdebug = false
	config.Stack.Features.Varnish = false

	config.Stack.Services.Search = "none"
	config.Stack.SearchVersion = ""
	config.Stack.Features.Elasticsearch = false

	config.Stack.Services.Cache = "none"
	config.Stack.CacheVersion = ""
	config.Stack.Features.Redis = false

	config.Stack.Services.Queue = "none"
	config.Stack.QueueVersion = ""
}

// AutoTuneMagentoRuntime applies the resolved Magento profile for the detected
// framework version so service/runtime versions stay compatible per project.
func AutoTuneMagentoRuntime(config *engine.Config, metadata engine.ProjectMetadata) []string {
	if config == nil || config.Recipe != "magento2" {
		return nil
	}

	version := strings.TrimSpace(metadata.Version)
	if version == "" {
		version = strings.TrimSpace(config.FrameworkVersion)
	}

	profileResult, err := engine.ResolveRuntimeProfile("magento2", version)
	if err != nil {
		return []string{fmt.Sprintf("Magento runtime auto-tune skipped: %v", err)}
	}

	notes := []string{
		fmt.Sprintf("Magento runtime auto-tune applied (source: %s)", profileResult.Source),
	}

	existingDBType := strings.TrimSpace(config.Stack.DBType)
	existingDBVersion := strings.TrimSpace(config.Stack.DBVersion)
	existingWebServer := strings.TrimSpace(config.Stack.Services.WebServer)
	engine.ApplyRuntimeProfileToConfig(config, profileResult.Profile)

	if shouldPreserveConfiguredDB(existingDBType, existingDBVersion, config.Stack.DBType, config.Stack.DBVersion) {
		config.Stack.DBType = existingDBType
		config.Stack.DBVersion = existingDBVersion
		notes = append(notes, fmt.Sprintf(
			"Magento runtime auto-tune kept existing DB version %s:%s to avoid incompatible downgrade from %s:%s",
			existingDBType,
			existingDBVersion,
			profileResult.Profile.DBType,
			profileResult.Profile.DBVersion,
		))
	}
	if shouldPreserveConfiguredWebServer(existingWebServer, config.Stack.Services.WebServer) {
		config.Stack.Services.WebServer = existingWebServer
		notes = append(notes, fmt.Sprintf(
			"Magento runtime auto-tune kept configured web server %s over tuned %s",
			existingWebServer,
			profileResult.Profile.WebServer,
		))
	}

	engine.NormalizeConfig(config)
	return notes
}

func shouldPreserveConfiguredDB(existingType, existingVersion, tunedType, tunedVersion string) bool {
	existingType = strings.ToLower(strings.TrimSpace(existingType))
	tunedType = strings.ToLower(strings.TrimSpace(tunedType))
	existingVersion = strings.TrimSpace(existingVersion)
	tunedVersion = strings.TrimSpace(tunedVersion)

	if existingType == "" || existingVersion == "" || tunedType == "" || tunedVersion == "" {
		return false
	}
	if existingType != tunedType {
		return false
	}

	comparison, comparable := compareNumericDotVersions(existingVersion, tunedVersion)
	return comparable && comparison > 0
}

func shouldPreserveConfiguredWebServer(existingWebServer, tunedWebServer string) bool {
	existing := strings.ToLower(strings.TrimSpace(existingWebServer))
	tuned := strings.ToLower(strings.TrimSpace(tunedWebServer))
	if existing == "" || existing == tuned {
		return false
	}
	switch existing {
	case "nginx", "apache", "hybrid":
		return true
	default:
		return false
	}
}

func compareNumericDotVersions(left, right string) (int, bool) {
	return engine.CompareNumericDotVersions(left, right)
}

func init() {
	upCmd.Flags().Bool("quickstart", false, "Use a minimal runtime profile for faster first run")
}
