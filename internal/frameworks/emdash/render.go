package emdash

import (
	"fmt"
	"strings"
)

// BuildRuntimeCommand builds the dev-server startup command for emdash's
// runtime container, moved verbatim from internal/engine/render.go's
// buildEmdashRuntimeCommand.
func BuildRuntimeCommand(packageManager string, domain string) string {
	domain = strings.TrimSpace(domain)

	if packageManager == "pnpm" {
		return strings.Join([]string{
			"corepack enable >/dev/null 2>&1 || true;",
			"if ! command -v pnpm >/dev/null 2>&1; then corepack prepare pnpm@latest --activate >/dev/null 2>&1; fi;",
			`if [ ! -d node_modules ] || [ -z "$$(ls -A node_modules 2>/dev/null)" ]; then pnpm install; fi;`,
			fmt.Sprintf("exec pnpm dev --host 0.0.0.0 --port 80 --allowed-hosts %s;", domain),
		}, " ")
	}

	return strings.Join([]string{
		`if [ ! -d node_modules ] || [ -z "$$(ls -A node_modules 2>/dev/null)" ]; then npm install; fi;`,
		fmt.Sprintf("exec npm run dev -- --host 0.0.0.0 --port 80 --allowed-hosts %s;", domain),
	}, " ")
}
