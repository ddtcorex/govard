package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"govard/internal/frameworks/gen/generator"
)

func TestRenderSourceProducesValidGo(t *testing.T) {
	source, err := generator.RenderSource(
		[]string{"cakephp", "django"},
		[]string{"django", "cakephp"},
	)
	if err != nil {
		t.Fatalf("RenderSource() error = %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "all_generated.go", source, 0)
	if err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, source)
	}

	for _, importPath := range []string{
		`"govard/internal/frameworks/cakephp"`,
		`"govard/internal/frameworks/django"`,
	} {
		if !strings.Contains(string(source), importPath) {
			t.Errorf("expected import %s, got:\n%s", importPath, source)
		}
	}

	if !strings.HasPrefix(string(source), "// Code generated") {
		t.Errorf("expected a 'Code generated' header, got:\n%s", source)
	}

	var registerArgs []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fnIdent, ok := call.Fun.(*ast.Ident)
		if !ok || fnIdent.Name != "Register" || len(call.Args) != 1 {
			return true
		}
		defCall, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := defCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		registerArgs = append(registerArgs, pkgIdent.Name)
		return true
	})

	want := []string{"django", "cakephp"}
	if !reflect.DeepEqual(registerArgs, want) {
		t.Errorf("Register() call order = %v, want %v", registerArgs, want)
	}
}

func TestRenderSourceIsIdempotent(t *testing.T) {
	first, err := generator.RenderSource([]string{"cakephp"}, []string{"cakephp"})
	if err != nil {
		t.Fatalf("RenderSource() error = %v", err)
	}
	second, err := generator.RenderSource([]string{"cakephp"}, []string{"cakephp"})
	if err != nil {
		t.Fatalf("RenderSource() error = %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("RenderSource() is not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
