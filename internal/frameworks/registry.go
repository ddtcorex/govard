package frameworks

import (
	"fmt"
	"sort"
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
	parents map[string]string
}

// NewRegistry returns an empty, ready-to-use Registry. Tests construct
// their own instance to stay isolated from the package-level default
// registry that all_generated.go populates for production use.
func NewRegistry() *Registry {
	return &Registry{
		byName:  make(map[string]types.FrameworkDefinition),
		aliases: make(map[string]string),
		parents: make(map[string]string),
	}
}

// NewRegistryFromSpecs resolves a parent/child framework graph into a
// read-only Registry. Specs may be supplied in any order.
func NewRegistryFromSpecs(specs []types.FrameworkSpec) (*Registry, error) {
	byName := make(map[string]types.FrameworkSpec, len(specs))
	for _, spec := range specs {
		name := normalizeName(spec.Definition.Name)
		if name == "" {
			return nil, fmt.Errorf("framework spec has an empty name")
		}
		if _, exists := byName[name]; exists {
			return nil, fmt.Errorf("duplicate framework spec %q", name)
		}
		spec.Definition.Name = name
		spec.Parent = normalizeName(spec.Parent)
		byName[name] = spec
	}

	registry := NewRegistry()
	states := make(map[string]uint8, len(byName))
	var resolve func(string) (types.FrameworkDefinition, error)
	resolve = func(name string) (types.FrameworkDefinition, error) {
		switch states[name] {
		case 1:
			return types.FrameworkDefinition{}, fmt.Errorf("framework inheritance cycle includes %q", name)
		case 2:
			definition, _ := registry.Get(name)
			return definition, nil
		}

		spec := byName[name]
		states[name] = 1
		parent := types.FrameworkDefinition{}
		if spec.Parent != "" {
			if spec.Parent == name {
				return types.FrameworkDefinition{}, fmt.Errorf("framework %q cannot be its own parent", name)
			}
			if _, exists := byName[spec.Parent]; !exists {
				return types.FrameworkDefinition{}, fmt.Errorf("framework %q has unknown parent %q", name, spec.Parent)
			}
			var err error
			parent, err = resolve(spec.Parent)
			if err != nil {
				return types.FrameworkDefinition{}, err
			}
		}

		definition := spec.Resolve(parent)
		definition.Name = name
		registry.byName[name] = definition
		registry.parents[name] = spec.Parent
		states[name] = 2
		return definition, nil
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := resolve(name); err != nil {
			return nil, err
		}
	}

	for _, name := range names {
		definition := registry.byName[name]
		for _, rawAlias := range definition.Aliases {
			alias := normalizeName(rawAlias)
			if alias == "" {
				return nil, fmt.Errorf("framework %q has an empty alias", name)
			}
			if owner, exists := registry.byName[alias]; exists && owner.Name != name {
				return nil, fmt.Errorf("framework alias %q conflicts with framework %q", alias, owner.Name)
			}
			if owner, exists := registry.aliases[alias]; exists && owner != name {
				return nil, fmt.Errorf("framework alias %q conflicts with framework %q", alias, owner)
			}
			registry.aliases[alias] = name
		}
	}

	return registry, nil
}

// Register adds def to the registry, indexing its aliases for Normalize.
func (r *Registry) Register(def types.FrameworkDefinition) {
	name := normalizeName(def.Name)
	def.Name = name
	r.byName[name] = types.CloneDefinition(def)
	r.parents[name] = ""
	for _, alias := range def.Aliases {
		r.aliases[normalizeName(alias)] = name
	}
}

func normalizeName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// Normalize resolves a raw framework name (possibly an alias) to its
// canonical registered Name. Unknown names are returned lowercased/trimmed
// but otherwise unchanged, matching the tolerant behavior of the existing
// per-package alias checks this registry will eventually replace.
func (r *Registry) Normalize(raw string) string {
	normalized := normalizeName(raw)
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
	return types.CloneDefinition(def), ok
}

// All returns every registered definition, in no particular order. Each
// returned value shares backing arrays with the stored entry - callers
// must treat every definition as read-only and must not mutate any of
// its slice fields.
func (r *Registry) All() []types.FrameworkDefinition {
	all := make([]types.FrameworkDefinition, 0, len(r.byName))
	for _, def := range r.byName {
		all = append(all, types.CloneDefinition(def))
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}

// Lineage returns the resolved ancestry from root to name. Unknown names return
// nil so callers can distinguish them from root frameworks.
func (r *Registry) Lineage(name string) []string {
	canonical := r.Normalize(name)
	if _, ok := r.byName[canonical]; !ok {
		return nil
	}

	lineage := []string{canonical}
	for parent := r.parents[canonical]; parent != ""; parent = r.parents[parent] {
		lineage = append(lineage, parent)
	}
	for left, right := 0, len(lineage)-1; left < right; left, right = left+1, right-1 {
		lineage[left], lineage[right] = lineage[right], lineage[left]
	}
	return lineage
}

// IsA reports whether name is ancestor itself or descends from ancestor.
func (r *Registry) IsA(name, ancestor string) bool {
	want := r.Normalize(ancestor)
	for _, current := range r.Lineage(name) {
		if current == want {
			return true
		}
	}
	return false
}

// ResolveAuditTarget asks every framework with an audit-target capability to
// classify request. A descendant framework wins when multiple matches share a
// lineage; matches from unrelated framework lineages are rejected as ambiguous.
func (r *Registry) ResolveAuditTarget(request types.AuditTargetResolveRequest) (types.FrameworkDefinition, types.AuditTarget, error) {
	var selectedDefinition types.FrameworkDefinition
	var selectedTarget types.AuditTarget
	selected := false

	for _, definition := range r.All() {
		if definition.AuditTargetResolver == nil {
			continue
		}
		target, recognized, err := definition.AuditTargetResolver(request)
		if err != nil {
			return definition, target, fmt.Errorf("resolve audit target with framework %q: %w", definition.Name, err)
		}
		if !recognized {
			continue
		}

		if !selected {
			selectedDefinition = definition
			selectedTarget = target
			selected = true
			continue
		}
		if r.IsA(definition.Name, selectedDefinition.Name) {
			selectedDefinition = definition
			selectedTarget = target
			continue
		}
		if r.IsA(selectedDefinition.Name, definition.Name) {
			continue
		}
		return types.FrameworkDefinition{}, types.AuditTarget{}, fmt.Errorf("ambiguous audit target: frameworks %q and %q both match %q", selectedDefinition.Name, definition.Name, request.StartPath)
	}

	if !selected {
		return types.FrameworkDefinition{}, types.AuditTarget{}, fmt.Errorf("no framework can resolve audit target %q", request.StartPath)
	}
	selectedTarget.Framework = selectedDefinition.Name
	return selectedDefinition, selectedTarget, nil
}

var defaultRegistry = NewRegistry()

// RegisterSpecs resolves specs and installs them as the package-level registry.
// The supplied order is retained when projecting detection data into engine,
// because detection priority is intentionally defined by generated registration
// order even though inheritance resolution itself is order-independent.
func RegisterSpecs(specs []types.FrameworkSpec) {
	registry, err := NewRegistryFromSpecs(specs)
	if err != nil {
		panic(fmt.Sprintf("build framework registry: %v", err))
	}
	defaultRegistry = registry
	for _, spec := range specs {
		definition, ok := registry.Get(spec.Definition.Name)
		if !ok {
			panic(fmt.Sprintf("resolved framework %q is missing", spec.Definition.Name))
		}
		registerEngineDefinition(definition)
	}
}

// Register adds def to the package-level default registry. Called from
// all_generated.go's init() for each of the 14 frameworks; production code should
// not call this directly. Also registers def's detection data with
// engine - unlike (*Registry).Register, this package-level function is
// only ever called on the real 14 frameworks (from all_generated.go), never on a
// throwaway test Registry, so it's safe for it alone to touch engine's
// global detection registry.
func Register(def types.FrameworkDefinition) {
	defaultRegistry.Register(def)
	registerEngineDefinition(def)
}

func registerEngineDefinition(def types.FrameworkDefinition) {
	engine.RegisterDetection(strings.ToLower(strings.TrimSpace(def.Name)), def.Detect)
	engine.RegisterFrameworkConfig(def.Name, def.Config)
	engine.RegisterFrameworkManifest(def.Name, def.Manifest)
	engine.RegisterPHPImageVariant(def.Name, def.PHPImageVariant)
	engine.RegisterNodeImageFlavor(def.Name, def.NodeImageFlavor)
	engine.RegisterVarnishTemplateFramework(def.Name, def.VarnishTemplateFramework)
	engine.RegisterDBDriverCategory(def.Name, def.DBDriverCategory)
	engine.RegisterDefaultChownDirectories(def.Name, def.DefaultChownDirectories)
	engine.RegisterFrameworkCapabilities(def.Name, engine.FrameworkCapabilities{
		FrontendSync:  def.FrontendSyncRenderer != nil,
		AuditProfiler: def.AuditProfiler != nil,
	})
	for _, migrationType := range def.MigrationTypes.DDEV {
		engine.RegisterMigrationFramework("ddev", migrationType, def.Name)
	}
	for _, migrationType := range def.MigrationTypes.Warden {
		engine.RegisterMigrationFramework("warden", migrationType, def.Name)
	}
	for _, alias := range def.Aliases {
		engine.RegisterFrameworkAlias(alias, def.Name)
	}
	if def.Upgrade != nil {
		engine.RegisterUpgrader(def.Name, def.Upgrade)
	}
	if def.RunMappingAssetPreparer != nil {
		engine.RegisterRunMappingAssetPreparer(def.Name, def.RunMappingAssetPreparer)
	}
	if def.TablePrefixDetector != nil {
		engine.RegisterTablePrefixDetector(def.Name, def.TablePrefixDetector)
	}
	if def.VersionProfileResolver != nil {
		engine.RegisterVersionProfileResolver(def.Name, def.VersionProfileResolver)
	}
	for name, fn := range def.TemplateFuncs {
		engine.RegisterTemplateFunc(name, fn)
	}
}

// Normalize resolves raw against the package-level default registry.
func Normalize(raw string) string { return defaultRegistry.Normalize(raw) }

// Get looks up name in the package-level default registry.
func Get(name string) (types.FrameworkDefinition, bool) { return defaultRegistry.Get(name) }

// FrontendSyncDiscoverer resolves a framework's optional frontend-sync
// discovery function without exposing framework-specific checks to command
// code.
func FrontendSyncDiscoverer(name string) (types.FrontendSyncDiscoverer, bool) {
	definition, ok := Get(name)
	if !ok || definition.FrontendSyncDiscoverer == nil {
		return nil, false
	}
	return definition.FrontendSyncDiscoverer, true
}

// FrontendSyncRenderer resolves a framework's optional frontend-sync
// blueprint-rendering function without exposing framework-specific checks
// to command code.
func FrontendSyncRenderer(name string) (types.FrontendSyncRenderer, bool) {
	definition, ok := Get(name)
	if !ok || definition.FrontendSyncRenderer == nil {
		return nil, false
	}
	return definition.FrontendSyncRenderer, true
}

// All returns every definition in the package-level default registry.
func All() []types.FrameworkDefinition { return defaultRegistry.All() }

// Lineage returns a framework's resolved ancestry from root to child.
func Lineage(name string) []string { return defaultRegistry.Lineage(name) }

// IsA reports whether framework is ancestor itself or descends from ancestor.
func IsA(framework, ancestor string) bool { return defaultRegistry.IsA(framework, ancestor) }

// ResolveAuditTarget resolves request through the package-level framework
// registry.
func ResolveAuditTarget(request types.AuditTargetResolveRequest) (types.FrameworkDefinition, types.AuditTarget, error) {
	return defaultRegistry.ResolveAuditTarget(request)
}
