package cmd

import (
	"errors"
	"fmt"

	"govard/internal/audit"

	"github.com/spf13/cobra"
)

// Operator guidance for the toolchain commands. It names only Govard commands
// and Docker itself: a credential path, an agent socket, or any environment
// value would end up in terminals, CI logs, and pasted issue reports.
const (
	auditToolchainStatusRepair = `run "govard audit toolchain pull" to fetch the pinned official lint image, or "govard audit toolchain build" to build the embedded one locally`
	auditToolchainPullRepair   = `run "govard audit toolchain build" to build the embedded lint toolchain image locally instead`
	auditToolchainBuildRepair  = `check that the Docker daemon is reachable, then run "govard audit toolchain status" to see what is present`
)

// auditToolchainIdentity is the exact identity of one resolved lint toolchain
// image. Every field is content- or digest-derived, so it is safe to print.
type auditToolchainIdentity struct {
	Provider      string `json:"provider"`
	Image         string `json:"image"`
	ImageDigest   string `json:"image_digest"`
	ContextDigest string `json:"context_digest"`
	LocalBuild    bool   `json:"local_build"`
}

// auditToolchainStatusReport is the read-only view of this host's lint
// toolchain: what is present, what the pinned official reference is, and what
// to run when nothing usable exists yet.
//
// Toolchain is deliberately a value, not a pointer: the default output format is
// text, and fmt's "%+v" only dereferences a top-level pointer, so a nested
// pointer field would render as a hex address instead of the image identity.
// Present is the authority on whether Toolchain means anything; it stays the
// zero value when nothing usable exists.
type auditToolchainStatusReport struct {
	Provider          string                 `json:"provider"`
	Present           bool                   `json:"present"`
	ContextDigest     string                 `json:"context_digest"`
	OfficialImage     string                 `json:"official_image"`
	OfficialUsable    bool                   `json:"official_usable"`
	LocalBuildImage   string                 `json:"local_build_image"`
	LocalBuildPresent bool                   `json:"local_build_present"`
	Toolchain         auditToolchainIdentity `json:"toolchain"`
	Repair            string                 `json:"repair,omitempty"`
}

// newAuditToolchainCommand groups the project-independent lint image commands.
// They manage a machine-wide image, so unlike every other audit subcommand they
// never resolve an audit target and work outside a Govard project.
func newAuditToolchainCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "toolchain",
		Short: "Inspect, pull, or build the Govard lint toolchain image",
		Long: "Manage the Govard-owned Magento lint image.\n\n" +
			"These commands act on a machine-wide image and do not need to run inside a\n" +
			"Govard project. They never run an externally configured lint provider.",
	}
	command.AddCommand(
		newAuditToolchainStatusCommand(options, dependencies),
		newAuditToolchainPullCommand(options, dependencies),
		newAuditToolchainBuildCommand(options, dependencies),
	)
	return command
}

func newAuditToolchainStatusCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report the lint toolchain image already present on this host",
		Long:  "Inspect local images only. Status never pulls and never builds, so it reports\nwhat this host already has rather than what a run would create.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateAuditOutputOptions(options); err != nil {
				return err
			}
			manager, err := auditToolchainManager(dependencies)
			if err != nil {
				return err
			}
			status, err := manager.Status(cmd.Context())
			if err != nil {
				return fmt.Errorf("inspect the lint toolchain image: %w; %s", err, auditToolchainStatusRepair)
			}
			return writeAuditValue(cmd, options.Format, auditToolchainStatusView(status))
		},
	}
}

func newAuditToolchainPullCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull the pinned official lint toolchain image",
		Long:  "Resolve only the official image pinned by this build's release digest. Pull\nnever builds anything: when the official path is unusable it reports that\ninstead of quietly producing a local image.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateAuditOutputOptions(options); err != nil {
				return err
			}
			manager, err := auditToolchainManager(dependencies)
			if err != nil {
				return err
			}
			resolved, err := manager.Pull(cmd.Context())
			if err != nil {
				return fmt.Errorf("pull the official lint toolchain image: %w; %s", err, auditToolchainPullRepair)
			}
			return writeAuditValue(cmd, options.Format, auditToolchainIdentityView(resolved))
		},
	}
}

func newAuditToolchainBuildCommand(options *auditCommandOptions, dependencies auditCommandDependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Build the embedded lint toolchain image locally",
		Long:  "Build the lint image from the build context embedded in this Govard binary.\nBuild never pulls, and reuses an existing image for the same context digest\nbecause that image is content addressed.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateAuditOutputOptions(options); err != nil {
				return err
			}
			manager, err := auditToolchainManager(dependencies)
			if err != nil {
				return err
			}
			resolved, err := manager.Build(cmd.Context())
			if err != nil {
				return fmt.Errorf("build the embedded lint toolchain image: %w; %s", err, auditToolchainBuildRepair)
			}
			return writeAuditValue(cmd, options.Format, auditToolchainIdentityView(resolved))
		},
	}
}

func auditToolchainManager(dependencies auditCommandDependencies) (*audit.ToolchainManager, error) {
	resolved := currentAuditDependencies(dependencies)
	if resolved.toolchainFactory == nil {
		return nil, errors.New("audit lint toolchain manager is not configured")
	}
	manager, err := resolved.toolchainFactory()
	if err != nil {
		return nil, fmt.Errorf("resolve the lint toolchain manager: %w", err)
	}
	if manager == nil {
		return nil, errors.New("audit lint toolchain manager is not configured")
	}
	return manager, nil
}

func auditToolchainIdentityView(resolved audit.ResolvedToolchain) auditToolchainIdentity {
	return auditToolchainIdentity{
		Provider:      audit.GovardLintProvider,
		Image:         resolved.Image,
		ImageDigest:   resolved.ImageDigest,
		ContextDigest: resolved.ContextDigest,
		LocalBuild:    resolved.LocalBuild,
	}
}

func auditToolchainStatusView(status audit.ToolchainStatus) auditToolchainStatusReport {
	report := auditToolchainStatusReport{
		Provider:          audit.GovardLintProvider,
		Present:           status.Present,
		ContextDigest:     status.ContextDigest,
		OfficialImage:     status.OfficialImage,
		OfficialUsable:    status.OfficialUsable,
		LocalBuildImage:   status.LocalBuildImage,
		LocalBuildPresent: status.LocalBuildPresent,
	}
	if status.Present {
		report.Toolchain = auditToolchainIdentityView(status.Toolchain)
		return report
	}
	report.Repair = auditToolchainStatusRepair
	return report
}
