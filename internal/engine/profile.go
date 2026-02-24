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
	DBType           string
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
	XdebugSession    string
}

type RuntimeProfileResult struct {
	Profile  RuntimeProfile
	Source   string
	Notes    []string
	Warnings []string
}

type runtimeProfileOverride struct {
	PHPVersion     string
	NodeVersion    string
	DBType         string
	DBVersion      string
	WebRoot        string
	WebServer      string
	Cache          string
	CacheVersion   string
	NginxVersion   string
	ApacheVersion  string
	Search         string
	SearchVersion  string
	VarnishVersion string
	Queue          string
	QueueVersion   string
}

var majorVersionPattern = regexp.MustCompile(`\d+`)
var majorMinorPattern = regexp.MustCompile(`\d+\.\d+`)
var magentoVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:-p\d+)?`)

var frameworkMajorOverrides = map[string]map[int]runtimeProfileOverride{
	"laravel": {
		10: {PHPVersion: "8.2"},
		11: {PHPVersion: "8.3"},
		12: {PHPVersion: "8.4"},
	},
	"symfony": {
		6: {PHPVersion: "8.2"},
		7: {PHPVersion: "8.3"},
	},
	"drupal": {
		10: {PHPVersion: "8.3"},
		11: {PHPVersion: "8.4"},
	},
	"wordpress": {
		6: {PHPVersion: "8.3"},
	},
}

func ResolveRuntimeProfile(framework string, version string) (RuntimeProfileResult, error) {
	framework = strings.TrimSpace(strings.ToLower(framework))
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
			DBType:           normalizeProfileValue(fwConfig.DefaultDB, "none"),
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
			XdebugSession:    "PHPSTORM",
		},
		Source: "framework-defaults",
	}

	normalizeProfile(&result.Profile)
	if result.Profile.DBType == "mysql" && result.Profile.DBVersion == "" && fwConfig.DefaultMySQLVer != "" {
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

	if framework == "magento2" {
		override, source, ok := resolveMagento2Override(version)
		if !ok {
			result.Warnings = append(result.Warnings, fmt.Sprintf("No version-specific profile for %s version %q. Using framework defaults.", framework, version))
			return result, nil
		}
		applyRuntimeProfileOverride(&result.Profile, override)
		normalizeProfile(&result.Profile)
		result.Source = source
		return result, nil
	}
	if framework == "laravel" {
		override, source, ok := resolveLaravelOverride(version)
		if ok {
			applyRuntimeProfileOverride(&result.Profile, override)
			normalizeProfile(&result.Profile)
			result.Source = source
			return result, nil
		}
	}
	if framework == "symfony" {
		override, source, ok := resolveSymfonyOverride(version)
		if ok {
			applyRuntimeProfileOverride(&result.Profile, override)
			normalizeProfile(&result.Profile)
			result.Source = source
			return result, nil
		}
	}
	if framework == "wordpress" {
		override, source, ok := resolveWordPressOverride(version)
		if ok {
			applyRuntimeProfileOverride(&result.Profile, override)
			normalizeProfile(&result.Profile)
			result.Source = source
			return result, nil
		}
	}

	overrideSet, hasFrameworkOverrides := frameworkMajorOverrides[framework]
	if !hasFrameworkOverrides {
		result.Notes = append(result.Notes, fmt.Sprintf("No version-specific profiles defined for framework %s.", framework))
		return result, nil
	}

	override, ok := overrideSet[major]
	if !ok {
		result.Warnings = append(result.Warnings, fmt.Sprintf("No version-specific profile for %s major %d. Using framework defaults.", framework, major))
		return result, nil
	}

	applyRuntimeProfileOverride(&result.Profile, override)
	normalizeProfile(&result.Profile)
	result.Source = fmt.Sprintf("version-specific:%s@%d", framework, major)
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

func applyRuntimeProfileOverride(profile *RuntimeProfile, override runtimeProfileOverride) {
	if profile == nil {
		return
	}
	if override.PHPVersion != "" {
		profile.PHPVersion = override.PHPVersion
	}
	if override.NodeVersion != "" {
		profile.NodeVersion = override.NodeVersion
	}
	if override.DBType != "" {
		profile.DBType = override.DBType
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
}

func normalizeProfile(profile *RuntimeProfile) {
	if profile == nil {
		return
	}

	profile.DBType = normalizeProfileValue(profile.DBType, "none")
	profile.Cache = normalizeProfileValue(profile.Cache, "none")
	profile.Search = normalizeProfileValue(profile.Search, "none")
	profile.Queue = normalizeProfileValue(profile.Queue, "none")
	profile.WebServer = normalizeProfileValue(profile.WebServer, "nginx")

	if profile.DBType == "none" {
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

func resolveMagento2Override(version string) (runtimeProfileOverride, string, bool) {
	major, minor, patch, pPatch, ok := parseMagentoVersion(version)
	if !ok {
		return runtimeProfileOverride{}, "", false
	}

	// Source alignment:
	// - Adobe Commerce System Requirements
	// - Internal Magento version mapping
	if major == 2 && minor == 4 {
		return resolveMagento24Override(major, minor, patch, pPatch)
	}
	return resolveLegacyMagento2Override(major, minor, patch, pPatch)
}

func resolveMagento24Override(major int, minor int, patch int, pPatch int) (runtimeProfileOverride, string, bool) {
	override := runtimeProfileOverride{
		DBType:  "mariadb",
		Cache:   "redis",
		Search:  "opensearch",
		Queue:   "rabbitmq",
		WebRoot: "/pub",
	}

	switch {
	case patch >= 9:
		override.PHPVersion = "8.4"
		override.DBVersion = "11.4"
		override.CacheVersion = "7.2"
		override.SearchVersion = "3.0.0"
		override.VarnishVersion = "7.6"
		override.QueueVersion = "4.1"
	case patch == 8:
		override.PHPVersion = "8.4"
		override.DBVersion = "11.4"
		override.CacheVersion = "7.2"
		override.VarnishVersion = "7.6"
		override.QueueVersion = "4.1"
		if pPatch >= 2 || pPatch == 0 {
			override.SearchVersion = "3.0.0"
		} else {
			override.SearchVersion = "2.19.0"
		}
	case patch == 7:
		override.PHPVersion = "8.3"
		override.DBVersion = "10.6"
		override.CacheVersion = "7.2"
		override.SearchVersion = "2.12.0"
		override.VarnishVersion = "7.4"
		override.QueueVersion = "3.13"
		if pPatch >= 5 {
			override.SearchVersion = "2.19.0"
		}
		if pPatch >= 6 {
			override.DBVersion = "10.11"
		}
		if pPatch >= 7 {
			override.QueueVersion = "4.1"
		}
	case patch == 6:
		override.PHPVersion = "8.2"
		override.DBVersion = "10.6"
		override.CacheVersion = "7.0"
		override.SearchVersion = "2.5.0"
		override.QueueVersion = "3.9"
		if pPatch >= 5 {
			override.SearchVersion = "2.12.0"
		}
		if pPatch >= 6 {
			override.QueueVersion = "3.12"
		}
		if pPatch >= 7 {
			override.QueueVersion = "3.13"
		}
		if pPatch >= 8 {
			override.CacheVersion = "7.2"
		}
		if pPatch >= 10 {
			override.SearchVersion = "2.19.0"
		}
		if pPatch >= 11 {
			override.DBVersion = "10.11"
		}
		if pPatch >= 12 {
			override.QueueVersion = "4.1"
		}
	case patch == 5:
		override.PHPVersion = "8.1"
		override.DBVersion = "10.4"
		override.CacheVersion = "6.2"
		override.SearchVersion = "1.2.0"
		override.QueueVersion = "3.9"
		if pPatch >= 7 {
			override.SearchVersion = "1.3.0"
			override.CacheVersion = "7.0"
		}
		if pPatch >= 8 {
			override.DBVersion = "10.5"
			override.QueueVersion = "3.11"
		}
		if pPatch >= 9 {
			override.QueueVersion = "3.13"
		}
		if pPatch >= 10 {
			override.CacheVersion = "7.2"
		}
		if pPatch >= 11 {
			override.DBVersion = "10.6"
			override.SearchVersion = "1.3.20"
		}
		if pPatch >= 12 {
			override.SearchVersion = "2.19.0"
		}
		if pPatch >= 14 {
			override.QueueVersion = "4.1"
		}
	case patch == 4:
		override.PHPVersion = "8.1"
		override.DBVersion = "10.4"
		override.CacheVersion = "6.2"
		override.SearchVersion = "1.2.0"
		override.QueueVersion = "3.9"
		if pPatch >= 8 {
			override.SearchVersion = "1.3.0"
			override.CacheVersion = "7.0"
		}
		if pPatch >= 9 {
			override.DBVersion = "10.5"
		}
		if pPatch >= 11 {
			override.CacheVersion = "7.2"
		}
		if pPatch >= 12 {
			override.DBVersion = "10.6"
			override.SearchVersion = "1.3.20"
		}
		if pPatch >= 13 {
			override.SearchVersion = "2.19.0"
		}
	case patch == 3:
		override.PHPVersion = "7.4"
		override.DBVersion = "10.4"
		override.Cache = "redis"
		override.CacheVersion = "6.0"
		override.Search = "elasticsearch"
		override.SearchVersion = "7.10.2"
		override.VarnishVersion = "6.0"
		override.QueueVersion = "3.8"
	case patch == 2:
		override.PHPVersion = "7.4"
		override.DBVersion = "10.4"
		override.Cache = "redis"
		override.CacheVersion = "6.0"
		override.Search = "elasticsearch"
		override.SearchVersion = "7.9.3"
		override.QueueVersion = "3.8"
	case patch == 1:
		override.PHPVersion = "7.4"
		override.DBVersion = "10.4"
		override.Cache = "redis"
		override.CacheVersion = "6.0"
		override.Search = "elasticsearch"
		override.SearchVersion = "7.9.3"
		override.QueueVersion = "3.8"
	case patch == 0:
		override.PHPVersion = "7.4"
		override.DBVersion = "10.4"
		override.Cache = "redis"
		override.CacheVersion = "5.0"
		override.Search = "elasticsearch"
		override.SearchVersion = "7.6.2"
		override.QueueVersion = "3.8"
	default:
		return runtimeProfileOverride{}, "", false
	}

	return override, fmt.Sprintf("version-specific:magento2@%d.%d.%d-p%d", major, minor, patch, pPatch), true
}

func resolveLegacyMagento2Override(major int, minor int, patch int, pPatch int) (runtimeProfileOverride, string, bool) {
	override := runtimeProfileOverride{
		DBType:  "mariadb",
		Cache:   "redis",
		Search:  "elasticsearch",
		Queue:   "rabbitmq",
		WebRoot: "/",
	}

	switch {
	case major == 2 && minor == 3:
		override.VarnishVersion = "6.0"
		if patch == 0 {
			override.PHPVersion = "7.1"
			override.DBVersion = "10.1"
			override.CacheVersion = "5.0"
			override.SearchVersion = "5.6.16"
			override.QueueVersion = "3.7"
		} else if patch <= 2 {
			override.PHPVersion = "7.2"
			override.DBVersion = "10.2"
			override.CacheVersion = "5.0"
			override.SearchVersion = "6.8.23"
			override.QueueVersion = "3.7"
		} else if patch == 3 {
			override.PHPVersion = "7.2"
			override.DBVersion = "10.2"
			override.CacheVersion = "5.0"
			override.SearchVersion = "6.8.23"
			override.QueueVersion = "3.8"
		} else if patch == 4 {
			override.PHPVersion = "7.2"
			override.DBVersion = "10.2"
			override.CacheVersion = "5.0"
			override.SearchVersion = "6.8.23"
			override.QueueVersion = "3.8"
		} else if patch <= 6 {
			override.PHPVersion = "7.3"
			override.DBVersion = "10.4"
			override.CacheVersion = "5.0"
			override.SearchVersion = "7.6.2"
			override.QueueVersion = "3.8"
		} else {
			override.PHPVersion = "7.4"
			override.DBVersion = "10.4"
			override.CacheVersion = "5.0"
			override.SearchVersion = "7.9.3"
			override.QueueVersion = "3.8"
		}
	case major == 2 && minor == 2:
		override.PHPVersion = "7.1"
		override.DBVersion = "10.1"
		override.CacheVersion = "5.0"
		override.SearchVersion = "5.6.16"
		override.VarnishVersion = "6.0"
		override.QueueVersion = "3.7"
		if patch == 0 {
			override.DBVersion = "10.0"
		}
	case major == 2 && (minor == 1 || minor == 0):
		override.PHPVersion = "7.1"
		override.DBVersion = "10.0"
		override.CacheVersion = "5.0"
		override.SearchVersion = "2.4.6"
		override.VarnishVersion = "6.0"
		override.QueueVersion = "3.7"
	default:
		return runtimeProfileOverride{}, "", false
	}

	return override, fmt.Sprintf("version-specific:magento2@%d.%d.%d-p%d", major, minor, patch, pPatch), true
}

func parseMagentoVersion(version string) (major int, minor int, patch int, pPatch int, ok bool) {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	match := magentoVersionPattern.FindString(version)
	if match != "" {
		version = match
	}
	if version == "" {
		return 0, 0, 0, 0, false
	}

	parts := strings.SplitN(version, "-p", 2)
	core := parts[0]
	coreParts := strings.Split(core, ".")
	if len(coreParts) < 3 {
		return 0, 0, 0, 0, false
	}

	major, err := strconv.Atoi(coreParts[0])
	if err != nil {
		return 0, 0, 0, 0, false
	}
	minor, err = strconv.Atoi(coreParts[1])
	if err != nil {
		return 0, 0, 0, 0, false
	}
	patch, err = strconv.Atoi(coreParts[2])
	if err != nil {
		return 0, 0, 0, 0, false
	}

	pPatch = 0
	if len(parts) == 2 && parts[1] != "" {
		pPatch, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, 0, false
		}
	}

	return major, minor, patch, pPatch, true
}

func resolveLaravelOverride(version string) (runtimeProfileOverride, string, bool) {
	major, ok := ExtractMajorVersion(version)
	if !ok {
		return runtimeProfileOverride{}, "", false
	}

	switch major {
	case 11:
		return runtimeProfileOverride{PHPVersion: "8.3", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:laravel@11", true
	case 10:
		return runtimeProfileOverride{PHPVersion: "8.2", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:laravel@10", true
	case 9:
		return runtimeProfileOverride{PHPVersion: "8.1", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:laravel@9", true
	case 8:
		return runtimeProfileOverride{PHPVersion: "8.0", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:laravel@8", true
	case 7:
		return runtimeProfileOverride{PHPVersion: "7.4", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:laravel@7", true
	case 6:
		return runtimeProfileOverride{PHPVersion: "7.4", DBType: "mariadb", DBVersion: "10.3"}, "version-specific:laravel@6", true
	case 5:
		return runtimeProfileOverride{PHPVersion: "7.0", DBType: "mariadb", DBVersion: "10.2"}, "version-specific:laravel@5", true
	case 4:
		return runtimeProfileOverride{PHPVersion: "7.0", DBType: "mariadb", DBVersion: "10.1"}, "version-specific:laravel@4", true
	default:
		return runtimeProfileOverride{}, "", false
	}
}

func resolveSymfonyOverride(version string) (runtimeProfileOverride, string, bool) {
	major, minor, ok := parseMajorMinor(version)
	if !ok {
		return runtimeProfileOverride{}, "", false
	}

	if major == 7 {
		if minor >= 2 {
			return runtimeProfileOverride{PHPVersion: "8.3", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:symfony@7.2", true
		}
		if minor == 1 || minor == 0 {
			return runtimeProfileOverride{PHPVersion: "8.2", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:symfony@7.1", true
		}
	}
	if major == 6 {
		switch {
		case minor >= 4:
			return runtimeProfileOverride{PHPVersion: "8.2", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:symfony@6.4", true
		case minor == 3 || minor == 2:
			return runtimeProfileOverride{PHPVersion: "8.1", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:symfony@6.3", true
		case minor == 1 || minor == 0:
			return runtimeProfileOverride{PHPVersion: "8.1", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:symfony@6.1", true
		}
	}
	if major == 5 {
		switch {
		case minor >= 4:
			return runtimeProfileOverride{PHPVersion: "8.0", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:symfony@5.4", true
		case minor == 3 || minor == 2:
			return runtimeProfileOverride{PHPVersion: "7.4", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:symfony@5.3", true
		}
	}

	return runtimeProfileOverride{}, "", false
}

func resolveWordPressOverride(version string) (runtimeProfileOverride, string, bool) {
	major, minor, ok := parseMajorMinor(version)
	if !ok {
		return runtimeProfileOverride{}, "", false
	}

	if major == 6 {
		switch {
		case minor >= 6:
			return runtimeProfileOverride{PHPVersion: "8.2", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:wordpress@6.7", true
		case minor == 5 || minor == 4:
			return runtimeProfileOverride{PHPVersion: "8.1", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:wordpress@6.5", true
		case minor == 3:
			return runtimeProfileOverride{PHPVersion: "8.1", DBType: "mariadb", DBVersion: "10.6"}, "version-specific:wordpress@6.3", true
		case minor == 2:
			return runtimeProfileOverride{PHPVersion: "8.0", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:wordpress@6.2", true
		case minor == 1 || minor == 0:
			return runtimeProfileOverride{PHPVersion: "8.0", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:wordpress@6.1", true
		}
	}
	if major == 5 {
		switch {
		case minor == 9:
			return runtimeProfileOverride{PHPVersion: "7.4", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:wordpress@5.9", true
		case minor == 8:
			return runtimeProfileOverride{PHPVersion: "7.4", DBType: "mariadb", DBVersion: "10.4"}, "version-specific:wordpress@5.8", true
		case minor == 7:
			return runtimeProfileOverride{PHPVersion: "7.4", DBType: "mariadb", DBVersion: "10.3"}, "version-specific:wordpress@5.7", true
		}
	}

	return runtimeProfileOverride{}, "", false
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
