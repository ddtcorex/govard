package engine

import "strings"

var defaultChownDirectories = map[string][]string{}

// RegisterDefaultChownDirectories installs framework-owned ownership paths.
func RegisterDefaultChownDirectories(framework string, directories []string) {
	key := strings.ToLower(strings.TrimSpace(framework))
	if key == "" {
		return
	}
	defaultChownDirectories[key] = append([]string(nil), directories...)
}

// DefaultChownDirectoriesForFramework returns a defensive copy of the
// framework-specific ownership paths registered at startup.
func DefaultChownDirectoriesForFramework(framework string) []string {
	values := defaultChownDirectories[strings.ToLower(strings.TrimSpace(framework))]
	return append([]string(nil), values...)
}
