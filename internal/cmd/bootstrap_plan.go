package cmd

import (
	"fmt"
	"strings"

	"govard/internal/engine"

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

	// 6. Framework specific post-steps
	switch {
	case engine.IsMagento2Family(framework):
		frameworkName := engine.Magento2FamilyDisplayName(framework)
		plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Configuring %s environment (env.php)...", frameworkName))
		plan.Commands = append(plan.Commands, "govard config auto")

		if opts.AdminCreate {
			plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Creating %s admin user...", frameworkName))
			plan.Commands = append(plan.Commands, "govard tool magento admin:user:create ...")
		}

		plan.Descriptions = append(plan.Descriptions, fmt.Sprintf("Reindexing %s data...", frameworkName))
		plan.Commands = append(plan.Commands, "govard tool magento indexer:reindex")
	case framework == "magento1" || framework == "openmage":
		plan.Descriptions = append(plan.Descriptions, "Configuring Magento 1 environment (base URLs and scoped website/store URLs)...")
		plan.Commands = append(plan.Commands, "govard config auto")
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
