package engine

import "strings"

// FrameworkCapabilities records optional behaviors supplied by a registered
// framework without making engine depend on the framework registry package.
type FrameworkCapabilities struct {
	FrontendSync  bool
	AuditProfiler bool
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
func FrameworkSupportsAuditProfiler(name string) bool {
	capabilities, ok := registeredFrameworkCapabilities[NormalizeFrameworkAlias(name)]
	return ok && capabilities.AuditProfiler
}
