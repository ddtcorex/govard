package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type RuntimeProfile struct {
	Framework        string
	FrameworkVersion string
	PHPVersion       string
	NodeVersion      string
	DB               string
	DBVersion        string
	WebRoot          string
	WebServer        string
	Cache            string
	CacheVersion     string
	NginxVersion     string
	ApacheVersion    string
	Search           string
	SearchVersion    string
	VarnishVersion   string
	Queue            string
	QueueVersion     string
	ComposerVersion  string
	XdebugSession    string
}

type RuntimeProfileResult struct {
	Profile  RuntimeProfile
	Source   string
	Notes    []string
	Warnings []string
}

// VersionProfileOverride carries the runtime-profile fields a
// framework-version-specific resolver wants to override on top of
// framework defaults (empty string = "don't override this field").
// Exported (renamed from the former unexported runtimeProfileOverride) so
// framework packages can construct/return it via their registered
// FrameworkDefinition.VersionProfileResolver.
type VersionProfileOverride struct {
	PHPVersion      string
	NodeVersion     string
	DB              string
	DBVersion       string
	WebRoot         string
	WebServer       string
	Cache           string
	CacheVersion    string
	NginxVersion    string
	ApacheVersion   string
	Search          string
	SearchVersion   string
	VarnishVersion  string
	Queue           string
	QueueVersion    string
	ComposerVersion string
}

var majorVersionPattern = regexp.MustCompile(`\d+`)
var majorMinorPattern = regexp.MustCompile(`\d+\.\d+`)

func ResolveRuntimeProfile(framework string, version string) (RuntimeProfileResult, error) {
	framework = NormalizeFrameworkAlias(framework)
	version = strings.TrimSpace(version)
	if framework == "" {
		return RuntimeProfileResult{}, fmt.Errorf("framework is required")
	}

	fwConfig, ok := GetFrameworkConfig(framework)
	if !ok {
		return RuntimeProfileResult{}, fmt.Errorf("unsupported framework: %s", framework)
	}

	result := RuntimeProfileResult{
		Profile: RuntimeProfile{
			Framework:        framework,
			FrameworkVersion: version,
			PHPVersion:       fwConfig.DefaultPHP,
			NodeVersion:      fwConfig.DefaultNodeVer,
			DB:               normalizeProfileValue(fwConfig.DefaultDB, "none"),
			DBVersion:        fwConfig.DefaultDBVer,
			WebRoot:          fwConfig.NGINXPUBLIC,
			WebServer:        normalizeProfileValue(fwConfig.DefaultWebServer, "nginx"),
			NginxVersion:     fwConfig.DefaultNginxVer,
			ApacheVersion:    fwConfig.DefaultApacheVer,
			Cache:            normalizeProfileValue(fwConfig.DefaultCache, "none"),
			CacheVersion:     fwConfig.DefaultCacheVer,
			Search:           normalizeProfileValue(fwConfig.DefaultSearch, "none"),
			SearchVersion:    fwConfig.DefaultSearchVer,
			VarnishVersion:   fwConfig.DefaultVarnishVer,
			Queue:            normalizeProfileValue(fwConfig.DefaultQueue, "none"),
			QueueVersion:     fwConfig.DefaultQueueVer,
			ComposerVersion:  fwConfig.DefaultComposerVer,
			XdebugSession:    "PHPSTORM",
		},
		Source: "framework-defaults",
	}

	normalizeProfile(&result.Profile)
	if result.Profile.DB == "mysql" && result.Profile.DBVersion == "" && fwConfig.DefaultMySQLVer != "" {
		result.Profile.DBVersion = fwConfig.DefaultMySQLVer
	}

	if version == "" {
		result.Notes = append(result.Notes, "Framework version is not detected. Using framework defaults.")
		return result, nil
	}

	major, ok := ExtractMajorVersion(version)
	if !ok {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not parse major version from %q. Using framework defaults.", version))
		return result, nil
	}

	if resolver, ok := GetVersionProfileResolver(framework); ok {
		override, source, ok := resolver(version)
		if !ok {
			result.Warnings = append(result.Warnings, fmt.Sprintf("No version-specific profile for %s version %q. Using framework defaults.", framework, version))
			return result, nil
		}
		applyRuntimeProfileOverride(&result.Profile, override)
		normalizeProfile(&result.Profile)
		result.Source = source
		return result, nil
	}
	_, minor, _ := parseMajorMinor(version)
	if override, source, ok := resolveFrameworkProfileFromRegistry(framework, major, minor); ok {
		applyRuntimeProfileOverride(&result.Profile, override)
		normalizeProfile(&result.Profile)
		result.Source = source
		return result, nil
	}
	result.Warnings = append(result.Warnings, fmt.Sprintf("No version-specific profile for %s major %d. Using framework defaults.", framework, major))
	return result, nil
}

func ExtractMajorVersion(version string) (int, bool) {
	match := majorVersionPattern.FindString(strings.TrimSpace(version))
	if match == "" {
		return 0, false
	}

	major, err := strconv.Atoi(match)
	if err != nil {
		return 0, false
	}
	return major, true
}

func applyRuntimeProfileOverride(profile *RuntimeProfile, override VersionProfileOverride) {
	if profile == nil {
		return
	}
	if override.PHPVersion != "" {
		profile.PHPVersion = override.PHPVersion
	}
	if override.NodeVersion != "" {
		profile.NodeVersion = override.NodeVersion
	}
	if override.DB != "" {
		profile.DB = override.DB
	}
	if override.DBVersion != "" {
		profile.DBVersion = override.DBVersion
	}
	if override.WebRoot != "" {
		profile.WebRoot = override.WebRoot
	}
	if override.WebServer != "" {
		profile.WebServer = override.WebServer
	}
	if override.Cache != "" {
		profile.Cache = override.Cache
	}
	if override.CacheVersion != "" {
		profile.CacheVersion = override.CacheVersion
	}
	if override.NginxVersion != "" {
		profile.NginxVersion = override.NginxVersion
	}
	if override.ApacheVersion != "" {
		profile.ApacheVersion = override.ApacheVersion
	}
	if override.Search != "" {
		profile.Search = override.Search
	}
	if override.SearchVersion != "" {
		profile.SearchVersion = override.SearchVersion
	}
	if override.VarnishVersion != "" {
		profile.VarnishVersion = override.VarnishVersion
	}
	if override.Queue != "" {
		profile.Queue = override.Queue
	}
	if override.QueueVersion != "" {
		profile.QueueVersion = override.QueueVersion
	}
	if override.ComposerVersion != "" {
		profile.ComposerVersion = override.ComposerVersion
	}
}

func normalizeProfile(profile *RuntimeProfile) {
	if profile == nil {
		return
	}

	profile.DB = normalizeProfileValue(profile.DB, "none")
	profile.Cache = normalizeProfileValue(profile.Cache, "none")
	profile.Search = normalizeProfileValue(profile.Search, "none")
	profile.Queue = normalizeProfileValue(profile.Queue, "none")
	profile.WebServer = normalizeProfileValue(profile.WebServer, "nginx")

	if profile.DB == "none" {
		profile.DBVersion = ""
	}
	if profile.Cache == "none" {
		profile.CacheVersion = ""
	}
	if profile.Search == "none" {
		profile.SearchVersion = ""
	}
	if profile.Queue == "none" {
		profile.QueueVersion = ""
	}
}

func normalizeProfileValue(raw string, fallback string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return fallback
	}
	return value
}

// VersionProfileResolver resolves version-specific runtime-profile
// overrides for one framework. Returns ok=false if version has no
// version-specific profile (caller falls back to framework defaults).
// Framework packages register this when they own patch-level compatibility
// data that cannot be expressed by the generic major/minor profile registry.
type VersionProfileResolver func(version string) (VersionProfileOverride, string, bool)

var versionProfileResolvers = map[string]VersionProfileResolver{}

// RegisterVersionProfileResolver registers resolver as the
// version-specific profile resolver for framework.
// Called from frameworks.Register. Not safe for concurrent calls;
// intended usage is registration during package init(), before
// ResolveRuntimeProfile is ever called. Frameworks without one fall through
// to the generic JSON-driven major/minor profile registry.
func RegisterVersionProfileResolver(framework string, resolver VersionProfileResolver) {
	versionProfileResolvers[strings.ToLower(strings.TrimSpace(framework))] = resolver
}

// GetVersionProfileResolver looks up the registered resolver for framework.
func GetVersionProfileResolver(framework string) (VersionProfileResolver, bool) {
	resolver, ok := versionProfileResolvers[strings.ToLower(strings.TrimSpace(framework))]
	return resolver, ok
}

func parseMajorMinor(version string) (major int, minor int, ok bool) {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" {
		return 0, 0, false
	}
	match := majorMinorPattern.FindString(version)
	if match == "" {
		major, ok := ExtractMajorVersion(version)
		if !ok {
			return 0, 0, false
		}
		return major, 0, true
	}
	parts := strings.Split(match, ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	mj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	mn, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return mj, mn, true
}
