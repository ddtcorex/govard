package tests

import (
	"testing"

	"govard/internal/cmd"
)

// WordPress projects without composer.json must not run dump-autoload, while
// frameworks with a PHP dependency workflow retain the existing best-effort
// behavior. This guards against reintroducing a name comparison in cmd.
func TestComposerDumpAutoloadPolicyComesFromFrameworkDefinition(t *testing.T) {
	if cmd.ShouldRunComposerDumpAutoloadForTest("wordpress", false) {
		t.Fatal("WordPress without composer.json should skip composer dump-autoload")
	}
	if !cmd.ShouldRunComposerDumpAutoloadForTest("wordpress", true) {
		t.Fatal("WordPress with composer.json should run composer dump-autoload")
	}
	if !cmd.ShouldRunComposerDumpAutoloadForTest("laravel", false) {
		t.Fatal("Laravel without composer.json should retain best-effort dump-autoload behavior")
	}
}
