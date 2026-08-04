package nextjs

import "strings"

// BuildRuntimeCommand builds the dev-server startup command for nextjs's
// runtime container, moved verbatim from internal/engine/render.go's
// buildNextJSRuntimeCommand. Installs dependencies if node_modules is
// missing (e.g. wiped independently of a fresh bootstrap) before running
// the dev server, matching emdash.BuildRuntimeCommand's resilience.
func BuildRuntimeCommand() string {
	return strings.Join([]string{
		`if [ ! -d node_modules ] || [ -z "$$(ls -A node_modules 2>/dev/null)" ]; then npm install; fi;`,
		`exec npm run dev -- --hostname 0.0.0.0 --port 80;`,
	}, " ")
}
