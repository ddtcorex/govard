package tests

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
)

// A `type: project` skeleton has no package name, so a plain `composer
// validate` exits 2 with publish errors ("name is required"). The row checks
// the manifest and lock consistency only, so it opts out of the publish checks.
func TestComposerValidateRowSkipsThePublishChecks(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "composer.json")
	_, argvs := runWithFake(t, "P2-12", engine.Config{Framework: "laravel"}, root)
	want := []string{"tool", "composer", "validate", "--no-check-publish"}
	if len(argvs) != 1 || !reflect.DeepEqual(argvs[0], want) {
		t.Fatalf("P2-12 argv = %v, want [%v]", argvs, want)
	}
	it, _ := findItem("P2-12")
	if !strings.Contains(it.Title, strings.Join(want[1:], " ")) {
		t.Fatalf("P2-12 title %q does not contain its argv", it.Title)
	}
}
