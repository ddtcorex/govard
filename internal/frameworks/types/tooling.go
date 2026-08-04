package types

// ToolCommand describes a framework-owned CLI exposed by `govard tool`.
// The owning framework is resolved by the registry; callers do not maintain a
// separate framework-name allowlist.
type ToolCommand struct {
	Name        string
	Aliases     []string
	Short       string
	Binary      string
	PrependArgs []string
	DefaultUser string
}
