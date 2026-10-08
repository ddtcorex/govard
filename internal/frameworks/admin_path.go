package frameworks

import (
	"fmt"
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

// AdminPathNotice returns a one-line notice for frameworks that ship no admin
// panel of their own, saying which path was opened anyway. It is empty for every
// framework with a real or declared admin route.
func AdminPathNotice(framework string, openedPath string) string {
	definition, ok := Get(framework)
	if !ok || !definition.NoStockAdminRoute {
		return ""
	}
	name := definition.DisplayName
	if name == "" {
		name = framework
	}
	return fmt.Sprintf("%s has no stock admin route; opened /%s (the configured or default path).", name, strings.Trim(openedPath, "/"))
}
