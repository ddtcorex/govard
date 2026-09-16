package engine

import "strings"

// FrameworkCapabilities records optional behaviors supplied by a registered
// framework without making engine depend on the framework registry package.
type FrameworkCapabilities struct {
	FrontendSync   bool
	AuditProfiler  bool
	AuditLint      bool
	AuditIntegrity bool
}

var registeredFrameworkCapabilities = map[string]FrameworkCapabilities{}

// RegisterFrameworkCapabilities projects framework-owned capability metadata
// into engine during framework registration.
func RegisterFrameworkCapabilities(name string, capabilities FrameworkCapabilities) {
	registeredFrameworkCapabilities[strings.ToLower(strings.TrimSpace(name))] = capabilities
}

// FrameworkSupportsFrontendSync reports whether framework registered a
// frontend-sync provider capability.
func FrameworkSupportsFrontendSync(name string) bool {
	capabilities, ok := registeredFrameworkCapabilities[NormalizeFrameworkAlias(name)]
	return ok && capabilities.FrontendSync
}

// FrameworkSupportsAuditProfiler reports whether the framework registered a
// stock runtime-profiler capability.
// FrameworkSupportsAuditIntegrity reports whether the framework declares a
// container-free integrity analyzer profile.
func FrameworkSupportsAuditIntegrity(name string) bool {
	capabilities, ok := registeredFrameworkCapabilities[NormalizeFrameworkAlias(name)]
	return ok && capabilities.AuditIntegrity
}

func FrameworkSupportsAuditProfiler(name string) bool {
	capabilities, ok := registeredFrameworkCapabilities[NormalizeFrameworkAlias(name)]
	return ok && capabilities.AuditProfiler
}

// FrameworkSupportsAuditLint reports whether the framework registered a
// lint-audit capability. Frameworks without AuditLint (e.g. Symfony,
// Laravel) return false so config normalization can omit the default
// audit.lint.provider that would otherwise promise a non-existent gate.
func FrameworkSupportsAuditLint(name string) bool {
	capabilities, ok := registeredFrameworkCapabilities[NormalizeFrameworkAlias(name)]
	return ok && capabilities.AuditLint
}

// SandboxSeedRewriter rewrites one env file's content for a derived sandbox
// (base_url, local hosts). Frameworks own their file's grammar; the deploy
// core only dispatches the function the framework registered.
type SandboxSeedRewriter func(content []byte, mapping map[string]string) ([]byte, error)

// SandboxSeedDefinition is what a framework contributes to sandbox seeding:
// the env/media paths relative to the app workdir, and the rewriter. Absent
// means the framework has nothing to seed beyond the database.
type SandboxSeedDefinition struct {
	EnvPath   string
	MediaPath string
	Rewrite   SandboxSeedRewriter
}

var registeredSandboxSeeds = map[string]SandboxSeedDefinition{}

// RegisterSandboxSeedDefinition projects one framework's seed definition into
// engine during framework registration.
func RegisterSandboxSeedDefinition(name string, definition SandboxSeedDefinition) {
	registeredSandboxSeeds[strings.ToLower(strings.TrimSpace(name))] = definition
}

// SandboxSeedFor returns the seed definition a framework registered, or false
// when it registered none.
func SandboxSeedFor(name string) (SandboxSeedDefinition, bool) {
	definition, ok := registeredSandboxSeeds[NormalizeFrameworkAlias(name)]
	return definition, ok
}
