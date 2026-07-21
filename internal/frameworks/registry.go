package frameworks

import (
	"strings"

	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

// Registry holds a set of FrameworkDefinitions indexed by canonical name,
// with alias resolution. The zero value is not usable - construct with
// NewRegistry. Not safe for concurrent Register calls; intended usage is
// to populate a Registry once (e.g. from an init() function) and only
// read from it afterward.
type Registry struct {
	byName  map[string]types.FrameworkDefinition
	aliases map[string]string
}

// NewRegistry returns an empty, ready-to-use Registry. Tests construct
// their own instance to stay isolated from the package-level default
// registry that all.go populates for production use.
func NewRegistry() *Registry {
	return &Registry{
		byName:  make(map[string]types.FrameworkDefinition),
		aliases: make(map[string]string),
	}
}

// Register adds def to the registry, indexing its aliases for Normalize.
func (r *Registry) Register(def types.FrameworkDefinition) {
	name := strings.ToLower(strings.TrimSpace(def.Name))
	r.byName[name] = def
	for _, alias := range def.Aliases {
		r.aliases[strings.ToLower(strings.TrimSpace(alias))] = name
	}
}

// Normalize resolves a raw framework name (possibly an alias) to its
// canonical registered Name. Unknown names are returned lowercased/trimmed
// but otherwise unchanged, matching the tolerant behavior of the existing
// per-package alias checks this registry will eventually replace.
func (r *Registry) Normalize(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if canonical, ok := r.aliases[normalized]; ok {
		return canonical
	}
	return normalized
}

// Get returns the registered definition for name (resolving aliases first).
// The returned value shares backing arrays (e.g. Config.Includes,
// Manifest.Ignored/Sensitive) with the stored entry - callers must treat it
// as read-only and must not mutate any of its slice fields.
func (r *Registry) Get(name string) (types.FrameworkDefinition, bool) {
	def, ok := r.byName[r.Normalize(name)]
	return def, ok
}

// All returns every registered definition, in no particular order. Each
// returned value shares backing arrays with the stored entry - callers
// must treat every definition as read-only and must not mutate any of
// its slice fields.
func (r *Registry) All() []types.FrameworkDefinition {
	all := make([]types.FrameworkDefinition, 0, len(r.byName))
	for _, def := range r.byName {
		all = append(all, def)
	}
	return all
}

var defaultRegistry = NewRegistry()

// Register adds def to the package-level default registry. Called from
// all.go's init() for each of the 12 frameworks; production code should
// not call this directly. Also registers def's detection data with
// engine - unlike (*Registry).Register, this package-level function is
// only ever called on the real 12 frameworks (from all.go), never on a
// throwaway test Registry, so it's safe for it alone to touch engine's
// global detection registry.
func Register(def types.FrameworkDefinition) {
	defaultRegistry.Register(def)
	engine.RegisterDetection(strings.ToLower(strings.TrimSpace(def.Name)), def.Detect)
}

// Normalize resolves raw against the package-level default registry.
func Normalize(raw string) string { return defaultRegistry.Normalize(raw) }

// Get looks up name in the package-level default registry.
func Get(name string) (types.FrameworkDefinition, bool) { return defaultRegistry.Get(name) }

// All returns every definition in the package-level default registry.
func All() []types.FrameworkDefinition { return defaultRegistry.All() }
