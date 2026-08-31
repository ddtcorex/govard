package engine

import (
	"regexp"
	"strings"
)

var (
	tablePrefixPattern = regexp.MustCompile(`^[A-Za-z0-9_]*$`)
)

func NormalizeTablePrefix(prefix string) string {
	return strings.TrimSpace(prefix)
}

func ValidateTablePrefix(prefix string) bool {
	return tablePrefixPattern.MatchString(NormalizeTablePrefix(prefix))
}

func SafeTablePrefix(prefix string) string {
	normalized := NormalizeTablePrefix(prefix)
	if !ValidateTablePrefix(normalized) {
		return ""
	}
	return normalized
}

// TablePrefixDetector inspects a project root and returns its configured
// database table prefix, or "" if none is set/detectable. Implementations
// live with the framework that owns the corresponding config format.
type TablePrefixDetector func(root string) string

var tablePrefixDetectors = map[string]TablePrefixDetector{}

// RegisterTablePrefixDetector registers detector as the table-prefix
// detector for framework, keyed the same way FrameworkSupportsTablePrefix/
// DetectFrameworkTablePrefix look it up (post normalizeFrameworkManifestKey
// aliasing). Called from frameworks.Register (alongside RegisterDetection/
// RegisterFrameworkConfig/RegisterFrameworkManifest) so a framework package
// can own its own table-prefix detection instead of a case in this file's
// switch. Not safe for concurrent calls; intended usage is registration
// during package init(), before FrameworkSupportsTablePrefix/
// DetectFrameworkTablePrefix are ever called. Not every framework needs an
// entry - only ones with table-prefix support register here.
func RegisterTablePrefixDetector(framework string, detector TablePrefixDetector) {
	tablePrefixDetectors[normalizeFrameworkManifestKey(framework)] = detector
}

// getTablePrefixDetector looks up the registered table-prefix detector for
// framework (post normalizeFrameworkManifestKey aliasing).
func getTablePrefixDetector(framework string) (TablePrefixDetector, bool) {
	detector, ok := tablePrefixDetectors[normalizeFrameworkManifestKey(framework)]
	return detector, ok
}

func FrameworkSupportsTablePrefix(framework string) bool {
	_, ok := getTablePrefixDetector(framework)
	return ok
}

func DetectFrameworkTablePrefix(root string, framework string) string {
	detector, ok := getTablePrefixDetector(framework)
	if !ok {
		return ""
	}
	return detector(root)
}
