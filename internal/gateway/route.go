package gateway

import "strings"

// RouteUsername maps a project name to the gateway's username shape
// ([a-z0-9-]+, the charset authorized-keys-command.sh authenticates).
// It lowercases the name and turns every underscore into a hyphen,
// returning the slug and true when the result is nonempty and holds only
// that charset, and ("", false) otherwise. A project whose name cannot be
// a gateway username registers no route: the deploy wiring skips gateway
// registration for it (see internal/deploy/sandbox_lifecycle.go) instead
// of registering a route that could never authenticate.
//
// Reserved names are NOT checked here: AddTarget owns that list, and this
// helper must not duplicate it.
func RouteUsername(project string) (string, bool) {
	slug := strings.ReplaceAll(strings.ToLower(project), "_", "-")
	if slug == "" {
		return "", false
	}
	for i := 0; i < len(slug); i++ {
		c := slug[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return "", false
	}
	return slug, true
}
