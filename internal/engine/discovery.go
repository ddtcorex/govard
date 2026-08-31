package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type ProjectMetadata struct {
	Framework string
	Version   string
}

// DetectionSpec describes how to auto-detect one framework: any-of matches
// against composer.json require keys, package.json dependency keys,
// auth.json http-basic hosts, and relative file-path existence checks.
// Populated via RegisterDetection, normally called once per framework from
// internal/frameworks's init() (see internal/frameworks/registry.go).
type DetectionSpec struct {
	ComposerPackages []string
	PackageJSONDeps  []string
	AuthJSONHosts    []string
	FilePaths        []string
}

var detectionRegistry = map[string]DetectionSpec{}

// detectionOrder preserves registration order so DetectFramework's
// ambiguous-match heuristics (auth.json hosts, file paths, and a
// composer.json/package.json requiring packages from more than one
// registered framework) resolve deterministically by priority, instead of
// Go's randomized map iteration order.
var detectionOrder []string

// RegisterDetection registers framework's detection data. Not safe for
// concurrent calls; intended usage is registration during package init(),
// before DetectFramework is ever called. Call order sets detection
// priority - see internal/frameworks/gen/generator/order.go's PriorityOverrides map,
// which is what now controls it.
func RegisterDetection(framework string, spec DetectionSpec) {
	if _, exists := detectionRegistry[framework]; !exists {
		detectionOrder = append(detectionOrder, framework)
	}
	detectionRegistry[framework] = spec
}

// GetRegisteredDetectionForTest exposes the detection registry for tests.
func GetRegisteredDetectionForTest(framework string) (DetectionSpec, bool) {
	spec, ok := detectionRegistry[framework]
	return spec, ok
}

func DetectWebRoot(root string, framework string) string {
	return DetectFrameworkWebRoot(root, framework)
}

func DetectFramework(root string) ProjectMetadata {
	metadata := ProjectMetadata{Framework: "generic"}

	// Check composer.json
	composerPath := filepath.Join(root, "composer.json")
	if _, err := os.Stat(composerPath); err == nil {
		if require, ok := readComposerRequirements(composerPath); ok {
			for _, framework := range detectionOrder {
				spec := detectionRegistry[framework]
				for _, pkg := range spec.ComposerPackages {
					if raw, exists := require[pkg]; exists {
						metadata.Framework = framework
						metadata.Version = dependencyVersionString(raw)
						return metadata
					}
				}
			}
		}
	}

	// Check package.json
	packagePath := filepath.Join(root, "package.json")
	if _, err := os.Stat(packagePath); err == nil {
		if deps, ok := readPackageDependencies(packagePath); ok {
			for _, framework := range detectionOrder {
				spec := detectionRegistry[framework]
				for _, dep := range spec.PackageJSONDeps {
					if raw, exists := deps[dep]; exists {
						metadata.Framework = framework
						metadata.Version = dependencyVersionString(raw)
						return metadata
					}
				}
			}
		}
	}

	// Heuristic: auth.json with a registered host present (e.g. Magento repo credentials)
	authPath := filepath.Join(root, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		data, _ := os.ReadFile(authPath)
		var auth map[string]interface{}
		if err := json.Unmarshal(data, &auth); err == nil {
			if basic, ok := auth["http-basic"].(map[string]interface{}); ok {
				for _, framework := range detectionOrder {
					spec := detectionRegistry[framework]
					for _, host := range spec.AuthJSONHosts {
						if _, ok := basic[host]; ok {
							metadata.Framework = framework
							return metadata
						}
					}
				}
			}
		}
	}

	// Heuristic: registered file-path existence checks
	for _, framework := range detectionOrder {
		spec := detectionRegistry[framework]
		for _, relPath := range spec.FilePaths {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relPath))); err == nil {
				metadata.Framework = framework
				return metadata
			}
		}
	}

	return metadata
}

func dependencyVersionString(raw interface{}) string {
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func readComposerRequirements(path string) (map[string]interface{}, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var composer struct {
		Require map[string]interface{} `json:"require"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return nil, false
	}
	if composer.Require == nil {
		return nil, false
	}
	return composer.Require, true
}

func readPackageDependencies(path string) (map[string]interface{}, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var pkg struct {
		Dependencies    map[string]interface{} `json:"dependencies"`
		DevDependencies map[string]interface{} `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, false
	}
	if pkg.Dependencies == nil && pkg.DevDependencies == nil {
		return nil, false
	}

	deps := make(map[string]interface{}, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for key, value := range pkg.Dependencies {
		deps[key] = value
	}
	for key, value := range pkg.DevDependencies {
		if _, exists := deps[key]; !exists {
			deps[key] = value
		}
	}
	return deps, true
}

// aliasRegistry maps a lowercased/trimmed alias to its canonical framework
// name (e.g. "magento" -> "magento2", "wp" -> "wordpress"). Populated via
// RegisterFrameworkAlias, normally called once per framework from
// internal/frameworks's package-level Register (see
// internal/frameworks/registry.go), looping over each FrameworkDefinition's
// Aliases field.
var aliasRegistry = map[string]string{}

// RegisterFrameworkAlias registers alias as resolving to canonical. Not
// safe for concurrent calls; intended usage is registration during package
// init(), before NormalizeFrameworkAlias is ever called.
func RegisterFrameworkAlias(alias string, canonical string) {
	alias = strings.ToLower(strings.TrimSpace(alias))
	canonical = strings.ToLower(strings.TrimSpace(canonical))
	if alias == "" || canonical == "" {
		return
	}
	aliasRegistry[alias] = canonical
}

// NormalizeFrameworkAlias resolves a raw framework name (possibly an
// alias registered via RegisterFrameworkAlias) to its canonical name.
// Unknown names are returned lowercased/trimmed but otherwise unchanged -
// engine-internal code should call this instead of a local
// magento/wp-style switch. Packages that already import
// internal/frameworks (internal/cmd, internal/desktop) should prefer
// frameworks.Normalize directly instead - this function exists only for
// engine-internal code, which cannot import internal/frameworks without
// creating an import cycle (frameworks already imports engine).
func NormalizeFrameworkAlias(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if canonical, ok := aliasRegistry[normalized]; ok {
		return canonical
	}
	return normalized
}
