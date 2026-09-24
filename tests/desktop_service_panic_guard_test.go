package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every method the frontend can call must recover panics into an error,
// because the proxies that used to do it are gone.
var boundServiceTypes = map[string]bool{
	"SettingsService": true, "OnboardingService": true, "EnvironmentService": true,
	"RemoteService": true, "SystemService": true, "LogService": true,
	"GlobalServiceService": true, "UpdateService": true,
}

var lifecycleMethods = map[string]bool{"Setup": true, "ServiceStartup": true, "ServiceShutdown": true}

func TestDesktopBoundServiceMethodsRecoverPanics(t *testing.T) {
	dir := filepath.Join("..", "internal", "desktop")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "test_helpers") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() || lifecycleMethods[fn.Name.Name] {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			recv, ok := star.X.(*ast.Ident)
			if !ok || !boundServiceTypes[recv.Name] {
				continue
			}
			results := fn.Type.Results
			if results == nil || len(results.List) == 0 {
				continue
			}
			last := results.List[len(results.List)-1]
			if ident, ok := last.Type.(*ast.Ident); !ok || ident.Name != "error" {
				continue
			}
			if !startsWithRecoverPanic(fn) {
				t.Errorf("%s: (%s).%s must start with `defer RecoverPanic(&err, %q)` and use a named error result", name, recv.Name, fn.Name.Name, fn.Name.Name)
			}
		}
	}
}

func startsWithRecoverPanic(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	d, ok := fn.Body.List[0].(*ast.DeferStmt)
	if !ok {
		return false
	}
	ident, ok := d.Call.Fun.(*ast.Ident)
	return ok && ident.Name == "RecoverPanic"
}
