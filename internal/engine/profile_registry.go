package engine

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed profiles.json
var profilesJSON embed.FS

// RuntimeProfileFixture represents a single test case for version resolution.
type RuntimeProfileFixture struct {
	Name            string            `json:"name"`
	Framework       string            `json:"framework"`
	Version         string            `json:"version"`
	Source          string            `json:"source"`
	SourcePrefix    string            `json:"source_prefix"`
	WarningContains string            `json:"warning_contains"`
	Expected        map[string]string `json:"expected"`
	ExpectError     bool              `json:"expect_error"`
}

type frameworkRule struct {
	Major           int    `json:"major"`
	MinorMin        *int   `json:"minor_min,omitempty"`
	PHPVersion      string `json:"php_version"`
	DB              string `json:"db"`
	DBVersion       string `json:"db_version"`
	ComposerVersion string `json:"composer_version,omitempty"`
}

type profileRegistryData struct {
	Frameworks   map[string][]frameworkRule `json:"frameworks"`
	TestFixtures []RuntimeProfileFixture    `json:"test_fixtures"`
}

var registry profileRegistryData

func init() {
	if data, err := profilesJSON.ReadFile("profiles.json"); err == nil {
		_ = json.Unmarshal(data, &registry)
	}
}

// GetFrameworkTestFixtures returns the test cases embedded in both registries.
func GetFrameworkTestFixtures() []RuntimeProfileFixture {
	return registry.TestFixtures
}

// resolveFrameworkProfileFromRegistry looks up the technology stack for other frameworks.
func resolveFrameworkProfileFromRegistry(framework string, major int, minor int) (VersionProfileOverride, string, bool) {
	rules, ok := registry.Frameworks[framework]
	if !ok {
		return VersionProfileOverride{}, "", false
	}

	for _, rule := range rules {
		if rule.Major != major {
			continue
		}

		if rule.MinorMin != nil && minor < *rule.MinorMin {
			continue
		}

		override := VersionProfileOverride{
			PHPVersion:      rule.PHPVersion,
			DB:              rule.DB,
			DBVersion:       rule.DBVersion,
			ComposerVersion: rule.ComposerVersion,
		}

		var source string
		if rule.MinorMin != nil {
			source = fmt.Sprintf("version-specific:%s@%d.%d", framework, major, *rule.MinorMin)
		} else {
			source = fmt.Sprintf("version-specific:%s@%d", framework, major)
		}

		return override, source, true
	}

	return VersionProfileOverride{}, "", false
}
