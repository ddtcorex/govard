package types

import (
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
)

// BootstrapFactory builds a framework's bootstrapper for one invocation.
// A factory (not a pre-built instance) because bootstrap.Options carries
// per-invocation state (target version, DB creds, etc.) that must not be
// baked into a long-lived registry entry.
type BootstrapFactory func(bootstrap.Options) bootstrap.FrameworkBootstrap

// FrameworkDefinition is the single source of truth for one framework's
// identity, runtime defaults, and sync/manifest data. It is intentionally
// minimal today - fields are added incrementally as later migration steps
// move each scattered per-framework switch onto this registry (see
// docs/superpowers/specs/2026-07-20-framework-registry-consolidation-design.md).
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

	// Bootstrap builds this framework's fresh-install/clone bootstrapper.
	// Nil for frameworks that don't yet implement bootstrap.FrameworkBootstrap
	// (currently only magento2, which uses the free function
	// bootstrap.Magento2FreshCommands instead - resolved when the bootstrap
	// dispatchers are unified in a later step).
	Bootstrap BootstrapFactory
}
