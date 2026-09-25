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
// lint-audit capability. Frameworks without AuditLint return false so config
// normalization can omit the default audit.lint.provider that would otherwise
// promise a non-existent gate.
func FrameworkSupportsAuditLint(name string) bool {
	capabilities, ok := registeredFrameworkCapabilities[NormalizeFrameworkAlias(name)]
	return ok && capabilities.AuditLint
}

// SandboxSeedRewriter rewrites one env file's content for a derived sandbox
// (base_url, local hosts). It returns the rewritten content plus the mapped
// keys it did not find: absent keys are skipped loudly, never invented —
// inserting new structure into a grammar the project may not use corrupts
// files. Frameworks own their file's grammar; the deploy core only dispatches
// the function the framework registered.
type SandboxSeedRewriter func(content []byte, mapping map[string]string) (rewritten []byte, skipped []string, err error)

// SandboxSeedDBRewrite builds the SQL statements that point a seeded database
// at the sandbox, from the origin env file's content and the sandbox web URL.
// It returns nil (or nothing) when the framework has nothing to rewrite there;
// the deploy core runs what comes back through the sandbox mysql client, so a
// statement must never carry a secret. Frameworks own their schema's grammar;
// the core only dispatches the function the framework registered.
type SandboxSeedDBRewrite func(envContent []byte, baseURL string) []string

// SandboxSeedDefinition is what a framework contributes to sandbox seeding:
// the env/media paths relative to the app workdir, the rewriter, and the
// optional post-import database rewrite. Absent means the framework has
// nothing to seed beyond the database.
type SandboxSeedDefinition struct {
	EnvPath   string
	MediaPath string
	Rewrite   SandboxSeedRewriter
	DBRewrite SandboxSeedDBRewrite
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
