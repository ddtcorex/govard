package engine

// VerifyToolItem is a framework-owned checklist entry: a `govard verify` item
// that runs one `govard tool <Tool> <Args...>` invocation. Frameworks declare
// the commands their own dev loop needs; internal/verify composes them so the
// registry never branches on a framework name.
type VerifyToolItem struct {
	// ID is the checklist id written into the run artifact. It must not
	// collide with a static registry id.
	ID string
	// Phase is the checklist phase the item belongs to: 2 or 3 for dev-loop
	// checks, 5 for a post-restore consistency check. It must be 1..5 — a value
	// outside that range is filtered out of every `--phase N` run and appears
	// only under a bare `--phase 0`, so the registry invariant test in tests/
	// rejects it rather than letting the item go quietly missing.
	Phase int
	// Title is the item title, and the "command" field of the run artifact.
	Title string
	// Tool is the `govard tool` subcommand, e.g. "artisan", "symfony", "wp".
	Tool string
	// Args are passed after Tool, verbatim.
	Args []string
}

var registeredVerifyToolItems = map[string][]VerifyToolItem{}

// RegisterVerifyToolItems projects a framework's declared checklist items into
// engine during framework registration. Called from frameworks.Register
// alongside RegisterTablePrefixDetector; not safe for concurrent calls, and
// intended to run during package init() before the verify registry is read.
//
// An empty registration is ignored rather than stored: a framework that
// re-registers through an alias must not blank a block it already declared.
func RegisterVerifyToolItems(name string, items []VerifyToolItem) {
	if len(items) == 0 {
		return
	}
	registeredVerifyToolItems[NormalizeFrameworkAlias(name)] = items
}

// VerifyToolItems returns the checklist items framework declared, or nil when
// it declared none (most frameworks do not).
func VerifyToolItems(name string) []VerifyToolItem {
	return registeredVerifyToolItems[NormalizeFrameworkAlias(name)]
}
