package types

// TestCommand describes one framework-owned project test invocation.
type TestCommand struct {
	Label  string
	Binary string
	Args   []string
}
