package cmd

import (
	"fmt"
	"strings"

	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"

	"github.com/pterm/pterm"
)

type bootstrapExecutionPlan struct {
	Descriptions []string
	Commands     []string
}

func buildBootstrapRemotePlan(config engine.Config, opts BootstrapRuntimeOptions) (bootstrapExecutionPlan, error) {
	plan := bootstrapExecutionPlan{}
	framework := strings.ToLower(strings.TrimSpace(config.Framework))

	// 1. Env Up
	if !opts.SkipUp {
		plan.Descriptions = append(plan.Descriptions, "Starting local development environment (containers)...")
		plan.Commands = append(plan.Commands, "govard env up --remove-orphans")
	}

	// 2. File Sync (Clone)
	if opts.Clone {
		syncArgs := bootstrapFileSyncArgs(opts)
		plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Cloning source files from remote '%s'...", opts.Source))
		plan.Commands = append(plan.Commands, "govard "+strings.Join(syncArgs, " "))
	}

	// 3. Composer Install
	if opts.ComposerInstall {
		plan.Descriptions = append(plan.Descriptions, "Installing PHP dependencies (composer install)...")
		plan.Commands = append(plan.Commands, "govard tool composer install -n")
	}

	// 4. DB Sync
	if opts.DBImport {
		if opts.DBDump != "" {
			plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Importing database from local file '%s'...", opts.DBDump))
			plan.Commands = append(plan.Commands, fmt.Sprintf("govard db import --file %s", opts.DBDump))
		} else if opts.StreamDB {
			plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Streaming database import from remote '%s'...", opts.Source))
			cmdLine := fmt.Sprintf("govard db import --stream-db --environment %s", opts.Source)
			if opts.NoNoise {
				cmdLine += " --no-noise"
			}
			if opts.NoPII {
				cmdLine += " --no-pii"
			}
			plan.Commands = append(plan.Commands, cmdLine)
		} else {
			plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Synchronizing database from remote '%s'...", opts.Source))
			cmdLine := fmt.Sprintf("govard sync --source %s --db", opts.Source)
			if opts.NoNoise {
				cmdLine += " --no-noise"
			}
			if opts.NoPII {
				cmdLine += " --no-pii"
			}
			plan.Commands = append(plan.Commands, cmdLine)
		}
	}

	// 5. Media Sync
	if opts.MediaSync != "" {
		plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Synchronizing media files from remote '%s'...", opts.Source))
		cmdLine := fmt.Sprintf("govard sync --source %s --media %s", opts.Source, opts.MediaSync)
		plan.Commands = append(plan.Commands, cmdLine)
	}

	if definition, ok := frameworks.Get(framework); ok && definition.BootstrapPlanSteps != nil {
		for _, step := range definition.BootstrapPlanSteps(opts.AdminCreate) {
			plan.Descriptions = append(plan.Descriptions, step.Description)
			plan.Commands = append(plan.Commands, step.Command)
		}
	}

	return plan, nil
}

func BuildBootstrapRemotePlanForTest(config engine.Config, opts BootstrapRuntimeOptions) (bootstrapExecutionPlan, error) {
	return buildBootstrapRemotePlan(config, opts)
}

func buildBootstrapPlanSummary(config engine.Config, source string, execution bootstrapExecutionPlan) []string {
	var lines []string

	header := pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Sprint(" Bootstrap Plan Review ")
	lines = append(lines, "", header, "")

	// Project Info
	lines = append(lines, fmt.Sprintf("  Source:      %s", pterm.LightCyan(source)))
	lines = append(lines, fmt.Sprintf("  Destination: local (local project: %s)", pterm.Gray(config.ProjectName)))
	lines = append(lines, fmt.Sprintf("  Framework:   %s", pterm.LightMagenta(config.Framework)))
	lines = append(lines, "")

	// Planned Actions
	lines = append(lines, "", pterm.Bold.Sprint("Planned Actions:"), "")
	for i, description := range execution.Descriptions {
		lines = append(lines, fmt.Sprintf(" %d. %s", i+1, description))
		if i < len(execution.Commands) {
			lines = append(lines, pterm.NewStyle(pterm.FgCyan, pterm.Bold).Sprintf("    ↳ sh: %s", execution.Commands[i]))
		}
	}

	return lines
}

// buildBootstrapFreshPlan describes a fresh install from metadata the framework
// already declares. It deliberately does not enumerate framework internals: a
// framework that needs more detail exposes it through the registry rather than
// a name switch here (see AGENTS.md on framework-name branching).
func buildBootstrapFreshPlan(config engine.Config, def types.FrameworkDefinition, opts BootstrapRuntimeOptions) bootstrapExecutionPlan {
	plan := bootstrapExecutionPlan{}
	display := strings.TrimSpace(def.DisplayName)
	if display == "" {
		display = config.Framework
	}

	if !opts.SkipUp {
		plan.Descriptions = append(plan.Descriptions, "Creating local project scaffolding and starting the environment...")
		plan.Commands = append(plan.Commands, "govard env up --remove-orphans")
	}

	// opts.MetaPackage arrives pre-filled with the Magento sentinel unless the
	// operator passed --meta-package, so the sentinel means "nobody chose this":
	// the framework's own default applies, and a framework that declares none
	// (Laravel, Symfony, WordPress) gets no package named rather than Magento's.
	meta := strings.TrimSpace(opts.MetaPackage)
	if meta == defaultBootstrapMetaPackage {
		meta = strings.TrimSpace(def.DefaultFreshMetaPackage)
	}
	install := fmt.Sprintf("Creating a fresh %s project", display)
	if meta != "" {
		install += fmt.Sprintf(" from package %s", meta)
	}
	install += " " + describeBootstrapMetaVersion(opts)
	plan.Descriptions = append(plan.Descriptions, install+"...")
	plan.Commands = append(plan.Commands, "govard tool composer create-project (framework fresh install)")

	if def.FreshInstallNeedsDB {
		plan.Descriptions = append(plan.Descriptions, "Configuring database credentials for the fresh install...")
		plan.Commands = append(plan.Commands, "govard config auto")
	}
	if opts.HyvaInstall {
		plan.Descriptions = append(plan.Descriptions, "Installing the Hyva theme...")
		plan.Commands = append(plan.Commands, "govard bootstrap hyva install")
	}
	if opts.IncludeSample {
		plan.Descriptions = append(plan.Descriptions, "Installing sample data...")
		plan.Commands = append(plan.Commands, "govard tool magento sampledata:deploy")
	}

	plan.Descriptions = append(plan.Descriptions, "Rewriting local application configuration...")
	plan.Commands = append(plan.Commands, "govard config auto")
	return plan
}

func buildBootstrapFreshPlanSummary(config engine.Config, def types.FrameworkDefinition, opts BootstrapRuntimeOptions, execution bootstrapExecutionPlan) []string {
	var lines []string

	header := pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Sprint(" Fresh Bootstrap Plan Review ")
	lines = append(lines, "", header, "")

	lines = append(lines, fmt.Sprintf("  Destination: local (local project: %s)", pterm.Gray(config.ProjectName)))
	lines = append(lines, fmt.Sprintf("  Framework:   %s", pterm.LightMagenta(config.Framework)))
	lines = append(lines, "")

	lines = append(lines, "", pterm.Bold.Sprint("Planned Actions:"), "")
	for i, description := range execution.Descriptions {
		lines = append(lines, fmt.Sprintf(" %d. %s", i+1, description))
		if i < len(execution.Commands) {
			lines = append(lines, pterm.NewStyle(pterm.FgCyan, pterm.Bold).Sprintf("    ↳ sh: %s", execution.Commands[i]))
		}
	}

	return lines
}

// BuildBootstrapFreshPlanForTest exposes the fresh plan for tests in /tests.
func BuildBootstrapFreshPlanForTest(config engine.Config, framework string, opts BootstrapRuntimeOptions) ([]string, error) {
	def, ok := frameworks.Get(strings.ToLower(strings.TrimSpace(framework)))
	if !ok {
		return nil, fmt.Errorf("fresh install not supported for framework: %s", framework)
	}
	return buildBootstrapFreshPlanSummary(config, def, opts, buildBootstrapFreshPlan(config, def, opts)), nil
}

// describeBootstrapMetaVersion says which version a fresh install uses and where
// it came from, so the default taken from .govard.yml is never silent.
func describeBootstrapMetaVersion(opts BootstrapRuntimeOptions) string {
	version := strings.TrimSpace(opts.MetaVersion)
	if version == "" {
		return "at the latest version (no --framework-version, no framework_version in .govard.yml)"
	}
	if opts.MetaVersionSource != "" {
		return fmt.Sprintf("at version %s (from %s)", version, opts.MetaVersionSource)
	}
	return fmt.Sprintf("at version %s", version)
}
