// internal/frameworks/gen/main.go
package main

import (
	"fmt"
	"os"

	"govard/internal/frameworks/gen/generator"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "generate frameworks registration:", err)
		os.Exit(1)
	}
}

// run discovers every framework package directly under the current
// directory (internal/frameworks/ when invoked via the go:generate
// directive in generate.go), orders them, and (re)writes
// all_generated.go with the result.
func run() error {
	names, err := generator.DiscoverFrameworkDirs(".")
	if err != nil {
		return err
	}

	order := generator.OrderByPriority(names, generator.PriorityOverrides)

	source, err := generator.RenderSource(names, order)
	if err != nil {
		return err
	}

	return os.WriteFile("all_generated.go", source, 0o644)
}
