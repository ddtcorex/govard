package frameworks

import (
	"strings"

	"govard/internal/frameworks/types"
)

// Registry holds a set of FrameworkDefinitions indexed by canonical name,
// with alias resolution. The zero value is not usable - construct with
// NewRegistry.
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
	r.byName[def.Name] = def
	for _, alias := range def.Aliases {
		r.aliases[strings.ToLower(strings.TrimSpace(alias))] = def.Name
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
func (r *Registry) Get(name string) (types.FrameworkDefinition, bool) {
	def, ok := r.byName[r.Normalize(name)]
	return def, ok
}

// All returns every registered definition, in no particular order.
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
// not call this directly.
func Register(def types.FrameworkDefinition) { defaultRegistry.Register(def) }

// Normalize resolves raw against the package-level default registry.
func Normalize(raw string) string { return defaultRegistry.Normalize(raw) }

// Get looks up name in the package-level default registry.
func Get(name string) (types.FrameworkDefinition, bool) { return defaultRegistry.Get(name) }

// All returns every definition in the package-level default registry.
func All() []types.FrameworkDefinition { return defaultRegistry.All() }
