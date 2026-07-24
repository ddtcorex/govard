package generator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// excludedDirs are subdirectories of internal/frameworks that hold shared
// support code, not a framework's own Definition() - DiscoverFrameworkDirs
// skips them.
var excludedDirs = map[string]bool{
	"gen":   true,
	"types": true,
}

// DiscoverFrameworkDirs lists the framework package directories directly
// under root (normally "." when run via the go:generate directive in
// internal/frameworks/generate.go, so root is internal/frameworks/
// itself). Each returned name is a directory whose non-test .go files
// declare a top-level `func Definition()` - the convention every
// internal/frameworks/<name> package follows (see
// docs/developer/adding-a-framework.md). A directory with .go files but
// no such function is a mistake (e.g. a typo'd package) and returns an
// error rather than being silently skipped, so it can't hide a
// misconfigured framework package from registration.
func DiscoverFrameworkDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || excludedDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		dirPath := filepath.Join(root, entry.Name())
		hasDefinition, err := dirDeclaresDefinition(dirPath)
		if err != nil {
			return nil, err
		}
		if !hasDefinition {
			return nil, fmt.Errorf("%s: no top-level func Definition() found in a non-test .go file - every internal/frameworks/<name> package must export one (see docs/developer/adding-a-framework.md)", dirPath)
		}
		names = append(names, entry.Name())
	}

	sort.Strings(names)
	return names, nil
}

// dirDeclaresDefinition reports whether any non-test .go file directly
// inside dirPath declares a top-level function named Definition.
func dirDeclaresDefinition(dirPath string) (bool, error) {
	files, err := filepath.Glob(filepath.Join(dirPath, "*.go"))
	if err != nil {
		return false, fmt.Errorf("glob %s: %w", dirPath, err)
	}

	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return false, fmt.Errorf("parse %s: %w", file, err)
		}

		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.Name == "Definition" {
				return true, nil
			}
		}
	}

	return false, nil
}
