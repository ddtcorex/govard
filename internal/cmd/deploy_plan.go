package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"govard/internal/deploy"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

var deployPlanCmd = &cobra.Command{
	Annotations: map[string]string{
		// Planning reads .govard.yml and the recipe only: no ssh, no network,
		// no git. That is what makes it usable in CI before a deploy and on a
		// host that has nothing installed.
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "plan [remote]",
	Short: "Print the deploy plan without connecting to the target",
	Long: `Print the resolved execution plan for one remote: every task in order, the
hooks the project and the recipe contributed, and which layer each came from.

Nothing is executed and no connection is made, so this is safe to run anywhere
and useful for reviewing a hook's placement before a deploy.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployPlan,
}

// DeployPlanCommand exposes the plan subcommand for tests.
func DeployPlanCommand() *cobra.Command { return deployPlanCmd }

// DeployCheckCommand exposes the check subcommand for tests.
func DeployCheckCommand() *cobra.Command { return deployCheckCmd }

func runDeployPlan(cmd *cobra.Command, args []string) error {
	remote, err := deployRemoteName(cmd, args)
	if err != nil {
		return err
	}
	config, options, err := resolveDeployOptions(cmd, remote)
	if err != nil {
		return err
	}
	hooks, err := hooksFromConfig(options.Hooks)
	if err != nil {
		return err
	}
	recipe, options := deployRecipe(config, options)
	plan, err := deploy.BuildPlan(recipe, hooks, remote)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Deploy plan for %s (%s @ %s)\n", remote, branchOrDetached(options), revisionOrSymbolic(options))
	fmt.Fprintf(out, "Publish strategy: %s\n\n", options.Publish)

	currentStage := deploy.Stage("")
	for idx, step := range plan.Steps {
		if step.Stage != currentStage {
			currentStage = step.Stage
			fmt.Fprintf(out, "%s\n", currentStage)
		}
		marker := "task"
		if step.Kind == deploy.StepHook {
			marker = "hook"
		}
		implementation := step.Command
		switch {
		case implementation != "":
		case step.Implemented():
			// A core step runs in the engine rather than as a shell command.
			implementation = "implemented in the engine"
		default:
			implementation = "skipped (no implementation for this framework)"
		}
		fmt.Fprintf(out, "  %2d. [%-4s] %-22s %s\n      %s\n", idx+1, marker, step.ID, step.Title, implementation)
		if step.Kind == deploy.StepHook {
			fmt.Fprintf(out, "      source: %s\n", step.Source)
		}
	}
	return nil
}

var deployCheckCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapSSH),
	},
	Use:   "check [remote]",
	Short: "Preflight a remote and report how it would be deployed",
	Long: `Check that a remote can be deployed to: connectivity, write access to the deploy
path, the existing release layout, and which publish strategy that layout implies.

Running this before a first deploy to an environment turns "it failed halfway
through" into "it cannot work, and here is why".`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployCheck,
}

func runDeployCheck(cmd *cobra.Command, args []string) error {
	remote, err := deployRemoteName(cmd, args)
	if err != nil {
		return err
	}
	config, options, err := resolveDeployOptions(cmd, remote)
	if err != nil {
		return err
	}
	host, err := deploy.HostForConfig(config, remote, options)
	if err != nil {
		return err
	}

	sc := deploy.StepContextForTest(host, options)
	if err := deploy.CoreCheck(cmd.Context(), sc); err != nil {
		return err
	}
	strategy, err := deploy.ResolvePublishStrategy(host, options)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Target %s is deployable\n", remote)
	for _, note := range sc.Notes {
		fmt.Fprintf(out, "  %s\n", note)
	}
	fmt.Fprintf(out, "  host:            %s\n", host.Name)
	fmt.Fprintf(out, "  deploy path:     %s\n", host.DeployPath)
	fmt.Fprintf(out, "  current path:    %s\n", host.CurrentPath)
	fmt.Fprintf(out, "  publish:         %s\n", strategy)
	if layout := describeLayout(strategy); layout != "" {
		fmt.Fprintf(out, "  layout:          %s\n", layout)
	}
	if strings.TrimSpace(options.Repository) == "" {
		fmt.Fprintln(out, "  warning:         no repository configured for this remote")
	}
	return nil
}

func describeLayout(strategy string) string {
	switch strategy {
	case deploy.PublishInPlace:
		return "current path is a real directory: releases are copied into it"
	case deploy.PublishSymlink:
		return "current path is absent or a symlink: releases are swapped atomically"
	default:
		return ""
	}
}

func branchOrDetached(options deploy.Options) string {
	if strings.TrimSpace(options.Branch) == "" {
		return "detached"
	}
	return options.Branch
}

// revisionOrSymbolic deliberately avoids resolving git: plan must stay
// requirement-free.
func revisionOrSymbolic(options deploy.Options) string {
	switch {
	case strings.TrimSpace(options.Revision) != "":
		return options.Revision
	case strings.TrimSpace(options.Tag) != "":
		return "tag " + options.Tag
	default:
		return "HEAD (resolved at deploy time)"
	}
}

// writeDeployJSON emits the machine-readable result CI consumes.
func writeDeployJSON(cmd *cobra.Command, remote string, options deploy.Options, release *deploy.Release, outcome deploy.Outcome) {
	type taskJSON struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		DurationMS int64  `json:"duration_ms"`
	}
	payload := struct {
		SchemaVersion int        `json:"schema_version"`
		Remote        string     `json:"remote"`
		Branch        string     `json:"branch"`
		Revision      string     `json:"revision"`
		Release       string     `json:"release"`
		Publish       string     `json:"publish"`
		Verify        string     `json:"verify"`
		Result        string     `json:"result"`
		DurationMS    int64      `json:"duration_ms"`
		Tasks         []taskJSON `json:"tasks"`
	}{
		SchemaVersion: 1,
		Remote:        remote,
		Branch:        options.Branch,
		Revision:      release.Revision,
		Release:       release.Release,
		Publish:       options.Publish,
		Verify:        release.Verify.Status,
		Result:        "ok",
		DurationMS:    outcome.Total.Milliseconds(),
	}
	for _, step := range outcome.Steps {
		payload.Tasks = append(payload.Tasks, taskJSON{ID: step.ID, Status: step.Status, DurationMS: step.Duration.Milliseconds()})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
}
