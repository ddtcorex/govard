package engine

// VerifySupport records the framework-owned discovery a `govard verify` item
// needs. internal/verify must not name a framework package — the module layout
// a Magento checklist row audits is framework knowledge — so the framework
// registers the lookup here and the checklist asks the engine for it, exactly
// the way SandboxSeedDefinition and VerifyToolItem work.
type VerifySupport struct {
	// AuditModuleDir returns the directory of a module inside projectRoot that
	// the audit's module-scoped target modes require, or false when the project
	// has none. The path is what an item passes to `audit run --path`; a
	// framework with no module concept leaves the field nil.
	//
	// An unreadable directory reports false the same way a missing one does: the
	// item's only alternatives are a skip or an argv pointing somewhere the
	// target resolver will reject, and a skip says so without inventing a path.
	AuditModuleDir func(projectRoot string) (string, bool)
}

var registeredVerifySupport = map[string]VerifySupport{}

// RegisterVerifySupport projects one framework's verify support into engine
// during framework registration. Called from frameworks.Register alongside
// RegisterSandboxSeedDefinition; not safe for concurrent calls, and intended to
// run during package init() before the verify registry is read.
func RegisterVerifySupport(name string, support VerifySupport) {
	registeredVerifySupport[NormalizeFrameworkAlias(name)] = support
}

// VerifySupportFor returns the support a framework registered, or false when it
// registered none. A framework that registered an empty VerifySupport reports
// true with a nil AuditModuleDir, which reads the same way at the call site: no
// module can be discovered.
func VerifySupportFor(name string) (VerifySupport, bool) {
	support, ok := registeredVerifySupport[NormalizeFrameworkAlias(name)]
	return support, ok
}
