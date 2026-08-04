package frameworks

import (
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
)

// ResolveRemoteAdminPath resolves the route to open for a framework's remote
// admin panel. Definitions may provide a non-standard default and, when
// needed, a remote probe. Unknown frameworks retain the generic route.
func ResolveRemoteAdminPath(framework, remoteName string, remoteCfg engine.RemoteConfig) (string, error) {
	path := DefaultAdminPath(framework)
	definition, ok := Get(framework)
	if !ok {
		return path, nil
	}
	if configured := strings.Trim(strings.TrimSpace(definition.DefaultAdminPath), "/"); configured != "" {
		path = configured
	}
	if definition.ResolveRemoteAdminPath == nil {
		return path, nil
	}

	resolved, err := definition.ResolveRemoteAdminPath(remoteName, remoteCfg)
	if configured := strings.Trim(strings.TrimSpace(resolved), "/"); configured != "" {
		path = configured
	}
	return path, err
}

// DefaultAdminPath returns a framework's declared local admin route, falling
// back to the generic route for unknown frameworks.
func DefaultAdminPath(framework string) string {
	path := conventions.DefaultAdminPath
	definition, ok := Get(framework)
	if !ok {
		return path
	}
	if configured := strings.Trim(strings.TrimSpace(definition.DefaultAdminPath), "/"); configured != "" {
		return configured
	}
	return path
}
