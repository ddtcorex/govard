package engine

import (
	"strings"
	"testing"
)

func contains(values []string, pattern string) bool {
	for _, value := range values {
		if value == pattern || strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func TestGlintIgnoreQuickDeep(t *testing.T) {
	quick := LintIgnore(true)
	if contains(quick, "vendor") == false {
		t.Fatal("quick must ignore vendor")
	}
	if contains(quick, "dev/tests") == false {
		t.Fatal("quick must ignore dev/tests")
	}
	if contains(quick, "pub/media") == false {
		t.Fatal("always ignore pub/media")
	}
	deep := LintIgnore(false)
	if contains(deep, "vendor") {
		t.Fatal("deep must not ignore vendor")
	}
	if contains(deep, "pub/media") == false {
		t.Fatal("deep must still ignore pub/media")
	}
}

func TestStableVolumeKey(t *testing.T) {
	a := StableVolumeKey("bebe9")
	b := StableVolumeKey("bebe9")
	if a != b {
		t.Fatal("stable")
	}
	if StableVolumeKey("bebe9") == StableVolumeKey("bebe9-123") {
		t.Fatal("must differ")
	}
}

func TestCategoryPrefix(t *testing.T) {
	// env.php parsing: empty, mg_, custom
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"empty", "<?php return ['db'=>['table_prefix'=>'']];", ""},
		{"mg", "<?php return ['db'=>['table_prefix'=>'mg_']];", "mg_"},
		{"custom", "<?php return ['db'=>['table_prefix'=>'custom_']];", "custom_"},
		{"empty double quote", `<?php return ["db"=>["table_prefix"=>""]];`, ""},
		{"mg double quote", `<?php $c=["db"=>["table_prefix"=>"mg_"]];`, "mg_"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseTablePrefix(c.content)
			if got != c.want {
				t.Fatalf("ParseTablePrefix(%q) = %q, want %q", c.content, got, c.want)
			}
		})
	}
	// SHOW TABLES inference fallback
	if got := InferTablePrefix([]string{"url_rewrite"}); got != "" {
		t.Fatalf("InferTablePrefix url_rewrite = %q, want empty", got)
	}
	if got := InferTablePrefix([]string{"mg_url_rewrite"}); got != "mg_" {
		t.Fatalf("InferTablePrefix mg_url_rewrite = %q, want mg_", got)
	}
	if got := InferTablePrefix([]string{"custom_url_rewrite"}); got != "custom_" {
		t.Fatalf("InferTablePrefix custom_url_rewrite = %q, want custom_", got)
	}
	if got := InferTablePrefix([]string{"mg_url_rewrite", "mg_catalog_category_entity"}); got != "mg_" {
		t.Fatalf("InferTablePrefix mg_ set = %q, want mg_", got)
	}
	if got := InferTablePrefix(nil); got != "" {
		t.Fatalf("InferTablePrefix nil = %q, want empty", got)
	}
	// Resolve prefers env.php over inference
	if got := ResolveTablePrefix("'table_prefix' => 'mg_'", []string{"custom_url_rewrite"}); got != "mg_" {
		t.Fatalf("ResolveTablePrefix prefers env got %q want mg_", got)
	}
	if got := ResolveTablePrefix("", []string{"mg_url_rewrite"}); got != "mg_" {
		t.Fatalf("ResolveTablePrefix fallback got %q want mg_", got)
	}
	if got := ResolveTablePrefix("", nil); got != "" {
		t.Fatalf("ResolveTablePrefix empty got %q want empty", got)
	}
	// Also test TablePrefix alias
	if got := TablePrefix("'table_prefix' => 'custom_'", []string{"mg_url_rewrite"}); got != "custom_" {
		t.Fatalf("TablePrefix alias got %q want custom_", got)
	}
}

func TestIsActiveExists(t *testing.T) {
	// true cases - column exists
	if !HasIsActive([]string{"entity_id", "is_active", "level"}) {
		t.Fatal("HasIsActive should be true when is_active present")
	}
	if !IsActiveExists([]string{"is_active"}) {
		t.Fatal("IsActiveExists should be true")
	}
	if !HasIsActiveColumn([]string{"is_active"}) {
		t.Fatal("HasIsActiveColumn should be true")
	}
	// false cases - example-project has no is_active column, only level
	if HasIsActive([]string{"entity_id", "level", "parent_id"}) {
		t.Fatal("HasIsActive should be false when is_active missing")
	}
	if IsActiveExists([]string{"entity_id", "level"}) {
		t.Fatal("IsActiveExists should be false")
	}
	if HasIsActive(nil) {
		t.Fatal("HasIsActive nil should be false")
	}
	if HasIsActive([]string{}) {
		t.Fatal("HasIsActive empty should be false")
	}
	// prefix-aware variant: table name is prefix+catalog_category_entity, but check is same
	if !HasIsActiveForTable([]string{"is_active"}, "mg_") {
		t.Fatal("HasIsActiveForTable mg_ should be true")
	}
	if HasIsActiveForTable([]string{"level"}, "mg_") {
		t.Fatal("HasIsActiveForTable mg_ should be false when missing")
	}
}
