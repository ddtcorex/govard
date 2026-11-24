package tests

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDesktopRuntimeVersionMatchesWailsModule pins the npm runtime to the Go
// module: the committed bindings import "@wailsio/runtime", so a Go-side bump
// with an npm-side miss only fails inside the webview at runtime. The two
// version strings differ in shape (a Go module version carries a leading "v",
// an npm version never does), so the comparison normalizes that one difference.
func TestDesktopRuntimeVersionMatchesWailsModule(t *testing.T) {
	mod, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	goVersion, err := wailsRequirement(mod)
	if err != nil {
		t.Fatalf("read the wails requirement: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "desktop", "frontend", "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("parse package.json: %v", err)
	}
	npmVersion := pkg.Dependencies["@wailsio/runtime"]
	if npmVersion != goVersion {
		t.Fatalf("@wailsio/runtime %q does not match the wails module %q", npmVersion, goVersion)
	}
}

// wailsRequirement reads the module version from either require form: a line
// inside a require block, or a single-line `require path version`. The leading
// "v" is trimmed because npm versions never carry it.
func wailsRequirement(goMod []byte) (string, error) {
	match := wailsRequirementPattern.FindSubmatch(goMod)
	if match == nil {
		return "", errors.New("go.mod has no wails v3 requirement")
	}
	return strings.TrimPrefix(string(match[1]), "v"), nil
}

var wailsRequirementPattern = regexp.MustCompile(`(?m)^\s*(?:require\s+)?github\.com/wailsapp/wails/v3\s+(\S+)(?:\s*//.*)?\s*$`)

func TestWailsRequirementReadsBothRequireForms(t *testing.T) {
	for name, goMod := range map[string]string{
		"require block": "module m\n\nrequire (\n\tgithub.com/wailsapp/wails/v3 v3.1.0\n)\n",
		"single line":   "module m\n\nrequire github.com/wailsapp/wails/v3 v3.1.0\n",
		"indirect":      "module m\n\nrequire (\n\tgithub.com/wailsapp/wails/v3 v3.1.0 // indirect\n)\n",
	} {
		got, err := wailsRequirement([]byte(goMod))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != "3.1.0" {
			t.Fatalf("%s: got %q, want 3.1.0", name, got)
		}
	}
}

func TestWailsRequirementIgnoresCommentsAndRejectsAbsence(t *testing.T) {
	if _, err := wailsRequirement([]byte("module m\n\n// github.com/wailsapp/wails/v3 v3.0.0\n")); err == nil {
		t.Fatal("a commented-out requirement must not count")
	}
	if _, err := wailsRequirement([]byte("module m\n")); err == nil {
		t.Fatal("a go.mod with no requirement must report an error")
	}
}
