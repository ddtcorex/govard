// internal/frameworks/generate.go
// Package frameworks' registration list (all_generated.go) is produced by
// this file's go:generate directive rather than hand-maintained - run
// `go generate ./internal/frameworks/...` (or `make generate`) after
// adding a new framework folder under internal/frameworks/<name>/. See
// docs/developer/adding-a-framework.md.
//
// Detection order matters for one known ambiguous-match case: a project
// whose dependencies match both Emdash's and Next.js's Detect signature
// must resolve to Emdash (the legacy pre-registry detector's tie-break).
// internal/frameworks/gen/generator/order.go's PriorityOverrides map is where that
// ordering is declared now, not here.
package frameworks

//go:generate go run ./gen
