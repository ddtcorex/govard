package magento2

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"govard/internal/engine"
)

//go:embed profiles.json
var profilesJSON embed.FS

type profileStack struct {
	SearchVersion  string `json:"search_version"`
	VarnishVersion string `json:"varnish_version"`
	NginxVersion   string `json:"nginx_version"`
	QueueVersion   string `json:"queue_version"`
	CacheVersion   string `json:"cache_version"`
}

type profileRule struct {
	Min             int    `json:"min"`
	Stack           string `json:"stack"`
	PHPVersion      string `json:"php_version"`
	DBVersion       string `json:"db_version"`
	Cache           string `json:"cache,omitempty"`
	Search          string `json:"search,omitempty"`
	SearchVersion   string `json:"search_version,omitempty"`
	VarnishVersion  string `json:"varnish_version"`
	NginxVersion    string `json:"nginx_version"`
	QueueVersion    string `json:"queue_version"`
	CacheVersion    string `json:"cache_version"`
	ComposerVersion string `json:"composer_version,omitempty"`
}

type patchVariant struct {
	Patch           *int          `json:"patch,omitempty"`
	PatchMin        *int          `json:"patch_min,omitempty"`
	PatchMax        *int          `json:"patch_max,omitempty"`
	PHPVersion      string        `json:"php_version"`
	DBVersion       string        `json:"db_version"`
	Cache           string        `json:"cache,omitempty"`
	CacheVersion    string        `json:"cache_version,omitempty"`
	Search          string        `json:"search,omitempty"`
	SearchVersion   string        `json:"search_version,omitempty"`
	QueueVersion    string        `json:"queue_version"`
	VarnishVersion  string        `json:"varnish_version"`
	NginxVersion    string        `json:"nginx_version"`
	ComposerVersion string        `json:"composer_version,omitempty"`
	Rules           []profileRule `json:"rules"`
}

type versionGroup struct {
	Major    int               `json:"major"`
	Minor    int               `json:"minor"`
	Defaults map[string]string `json:"defaults"`
	Patches  []patchVariant    `json:"patches"`
}

type profileRegistry struct {
	Stacks   map[string]profileStack `json:"stacks"`
	Versions []versionGroup          `json:"versions"`
}

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:-p\d+)?`)
var profiles profileRegistry

func init() {
	if data, err := profilesJSON.ReadFile("profiles.json"); err == nil {
		_ = json.Unmarshal(data, &profiles)
	}
}

// ResolveVersionProfile owns Magento 2's patch-level runtime compatibility
// matrix. Mage-OS intentionally clears this inherited resolver because its
// versioning is independent from Magento 2.
func ResolveVersionProfile(version string) (engine.VersionProfileOverride, string, bool) {
	major, minor, patch, pPatch, ok := parseVersion(version)
	if !ok {
		return engine.VersionProfileOverride{}, "", false
	}

	for _, group := range profiles.Versions {
		if group.Major != major || group.Minor != minor {
			continue
		}
		for _, variant := range group.Patches {
			if variant.Patch != nil && *variant.Patch != patch {
				continue
			}
			if variant.PatchMin != nil && patch < *variant.PatchMin {
				continue
			}
			if variant.PatchMax != nil && patch > *variant.PatchMax {
				continue
			}

			override := engine.VersionProfileOverride{
				DB:         group.Defaults["db"],
				Cache:      group.Defaults["cache"],
				Search:     group.Defaults["search"],
				Queue:      group.Defaults["queue"],
				WebRoot:    group.Defaults["web_root"],
				PHPVersion: variant.PHPVersion,
				DBVersion:  variant.DBVersion,
			}
			applyPatchBaselines(&override, variant)

			for i := range variant.Rules {
				if pPatch < variant.Rules[i].Min {
					continue
				}
				if stack, exists := profiles.Stacks[variant.Rules[i].Stack]; exists {
					applyStack(&override, stack)
				}
				applyRule(&override, variant.Rules[i])
				break
			}

			return override, fmt.Sprintf("version-specific:magento2@%d.%d.%d-p%d", major, minor, patch, pPatch), true
		}
	}
	return engine.VersionProfileOverride{}, "", false
}

func parseVersion(version string) (major, minor, patch, pPatch int, ok bool) {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if match := versionPattern.FindString(version); match != "" {
		version = match
	}
	parts := strings.SplitN(version, "-p", 2)
	core := strings.Split(parts[0], ".")
	if len(core) != 3 {
		return 0, 0, 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(core[0]); err != nil {
		return 0, 0, 0, 0, false
	}
	if minor, err = strconv.Atoi(core[1]); err != nil {
		return 0, 0, 0, 0, false
	}
	if patch, err = strconv.Atoi(core[2]); err != nil {
		return 0, 0, 0, 0, false
	}
	if len(parts) == 2 && parts[1] != "" {
		if pPatch, err = strconv.Atoi(parts[1]); err != nil {
			return 0, 0, 0, 0, false
		}
	}
	return major, minor, patch, pPatch, true
}

func applyPatchBaselines(o *engine.VersionProfileOverride, variant patchVariant) {
	if variant.Cache != "" {
		o.Cache = variant.Cache
	}
	if variant.Search != "" {
		o.Search = variant.Search
	}
	if variant.CacheVersion != "" {
		o.CacheVersion = variant.CacheVersion
	}
	if variant.SearchVersion != "" {
		o.SearchVersion = variant.SearchVersion
	}
	if variant.QueueVersion != "" {
		o.QueueVersion = variant.QueueVersion
	}
	if variant.VarnishVersion != "" {
		o.VarnishVersion = variant.VarnishVersion
	}
	if variant.NginxVersion != "" {
		o.NginxVersion = variant.NginxVersion
	}
	if variant.ComposerVersion != "" {
		o.ComposerVersion = variant.ComposerVersion
	}
}

func applyStack(o *engine.VersionProfileOverride, stack profileStack) {
	if stack.SearchVersion != "" {
		o.SearchVersion = stack.SearchVersion
	}
	if stack.VarnishVersion != "" {
		o.VarnishVersion = stack.VarnishVersion
	}
	if stack.NginxVersion != "" {
		o.NginxVersion = stack.NginxVersion
	}
	if stack.QueueVersion != "" {
		o.QueueVersion = stack.QueueVersion
	}
	if stack.CacheVersion != "" {
		o.CacheVersion = stack.CacheVersion
	}
}

func applyRule(o *engine.VersionProfileOverride, rule profileRule) {
	if rule.PHPVersion != "" {
		o.PHPVersion = rule.PHPVersion
	}
	if rule.DBVersion != "" {
		o.DBVersion = rule.DBVersion
	}
	if rule.Cache != "" {
		o.Cache = rule.Cache
	}
	if rule.Search != "" {
		o.Search = rule.Search
	}
	if rule.SearchVersion != "" {
		o.SearchVersion = rule.SearchVersion
	}
	if rule.ComposerVersion != "" {
		o.ComposerVersion = rule.ComposerVersion
	}
	if rule.VarnishVersion != "" {
		o.VarnishVersion = rule.VarnishVersion
	}
	if rule.NginxVersion != "" {
		o.NginxVersion = rule.NginxVersion
	}
	if rule.QueueVersion != "" {
		o.QueueVersion = rule.QueueVersion
	}
	if rule.CacheVersion != "" {
		o.CacheVersion = rule.CacheVersion
	}
}
