package types

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/tunnel"
)

// BootstrapFactory builds a framework's bootstrapper for one invocation.
// A factory (not a pre-built instance) because bootstrap.Options carries
// per-invocation state (target version, DB creds, etc.) that must not be
// baked into a long-lived registry entry.
type BootstrapFactory func(bootstrap.Options) bootstrap.FrameworkBootstrap

// FrameworkDefinition is the single source of truth for one framework's
// identity, runtime defaults, sync/manifest data, and dispatch (bootstrap,
// base-URL rewriting, bootstrap-command support, fresh-install
// orchestration, clone-workflow hooks). Every registered framework now
// has FreshInstall populated except Magento 1 (fresh install unsupported
// by design - CreateProject just returns an error telling the user to
// use --clone) and PrestaShop (fresh install never supported, no
// FreshInstall field at all). Clone-workflow orchestration in
// internal/cmd/bootstrap_remote.go dispatches through the registry too -
// the generic FrameworkBootstrap.PostClone(projectDir) interface method
// for most frameworks, plus the two optional hook fields below
// (PreConfigureHook/PostCloneHook) for frameworks whose clone-workflow
// needs step timing or cmd-package capabilities (running `govard tool
// <x>`) that plain interface method can't express - Magento 2/Mage-OS
// are the first consumers, not the only intended ones.
type FrameworkDefinition struct {
	// Name is the canonical framework key, e.g. "magento2", "laravel".
	Name string
	// Aliases are additional strings that should resolve to Name (e.g.
	// "magento" -> "magento2").
	Aliases []string
	// DisplayName is a human-readable label, e.g. "Magento 2".
	DisplayName string

	// Config carries runtime/compose defaults (PHP version, includes list,
	// nginx template, etc.), currently sourced from engine.GetFrameworkConfig.
	Config engine.FrameworkConfig
	// Manifest carries sync/media exclude and sensitive-table data,
	// currently sourced from engine.GetFrameworkManifestConfig.
	Manifest engine.FrameworkManifestConfig

	// Detect describes how to auto-detect this framework from a project
	// directory (composer.json/package.json/auth.json/file-path matches).
	// Populated by each framework's Definition() and pushed into
	// engine's detection registry by Registry.Register.
	Detect engine.DetectionSpec

	// Bootstrap builds this framework's fresh-install/clone bootstrapper.
	// Populated for all 14 frameworks; frameworks.RunBootstrap uses it to
	// dispatch without a per-framework switch.
	Bootstrap BootstrapFactory

	// BaseURLManager builds this framework's tunnel base-URL rewriter (for
	// `govard tunnel`). Nil for frameworks that don't need specialized
	// rewriting; frameworks.NewBaseURLManager falls back to
	// tunnel.NoopManager in that case.
	BaseURLManager func() tunnel.BaseURLManager

	// SupportsBootstrap allows `govard bootstrap` (remote/clone workflow)
	// for this framework.
	SupportsBootstrap bool
	// SupportsFreshInstall allows `govard bootstrap --fresh` for this
	// framework.
	SupportsFreshInstall bool

	// FreshInstall runs this framework's fresh-install orchestration
	// (CreateProject/Install/Configure sequencing, env-up timing, etc.),
	// replacing a per-framework case in
	// internal/cmd/bootstrap_fresh_install.go's switch. nil for
	// frameworks not yet migrated to this field - they keep dispatching
	// through that switch.
	FreshInstall func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error
	// FreshInstallNeedsDB/FreshInstallNeedsDomain tell the caller which
	// bootstrap.Options fields (DB credentials, domain) to populate
	// before invoking FreshInstall. Only meaningful when FreshInstall is
	// non-nil.
	FreshInstallNeedsDB     bool
	FreshInstallNeedsDomain bool

	// PreConfigureHook runs framework-specific setup that must happen
	// during the remote/clone bootstrap workflow, before `govard config
	// auto` - for frameworks whose configure step depends on a generated
	// file existing first (Magento 2/Mage-OS's app/etc/env.php, generated
	// from a template plus any probed remote crypt key/table prefix).
	// Optional; nil for frameworks that don't need it. Runs unconditionally
	// (not gated on opts.ComposerInstall) whenever set, since env.php
	// generation is a prerequisite for config auto rather than a
	// consequence of composer install.
	PreConfigureHook func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error
	// PostCloneHook runs additional framework-specific steps during the
	// remote/clone bootstrap workflow, after the generic
	// def.Bootstrap(opts).PostClone(projectDir) dispatch (which some
	// frameworks - e.g. Magento 2/Mage-OS - don't implement at all,
	// reporting it as unsupported since their real post-clone setup is
	// this hook instead). Optional; nil for frameworks that don't need
	// it. The caller gates this on opts.ComposerInstall alone, NOT on
	// shouldRunFrameworkPostClone/FrameworkSupportsPostClone - those also
	// check engine.FrameworkSupportsPostClone, which is deliberately false
	// for frameworks that only use this hook (their plain PostClone really
	// is unsupported), so gating on that combined condition would make
	// this hook permanently unreachable for exactly the frameworks that set it.
	PostCloneHook func(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error
}
