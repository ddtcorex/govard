package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var routeLine = regexp.MustCompile(`^\s*(\w+):\s*\["(\w+)",\s*"(\w+)"\],`)

// Every bridge route must resolve to a generated binding. Catches a
// tag-less generation (no service files) and a renamed Go method.
func TestDesktopBridgeRoutesMatchGeneratedBindings(t *testing.T) {
	bridge, err := os.ReadFile(filepath.Join("..", "desktop", "frontend", "services", "bridge.js"))
	if err != nil {
		t.Fatal(err)
	}
	bindingsDir := filepath.Join("..", "desktop", "frontend", "bindings", "govard", "internal", "desktop")
	routes := 0
	for _, line := range strings.Split(string(bridge), "\n") {
		m := routeLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		routes++
		service, method := m[2], m[3]
		file := filepath.Join(bindingsDir, bindingFileName(service))
		src, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("route %s: no generated binding file %s (was bindings generated with -f \"-tags desktop\"?)", m[1], file)
			continue
		}
		if !regexp.MustCompile(`export function ` + method + `\(`).Match(src) {
			t.Errorf("route %s: %s has no exported %s", m[1], file, method)
		}
	}
	if routes < 55 {
		t.Fatalf("parsed %d routes from bridge.js, want at least 55", routes)
	}
}

// bindingFileName matches the generator's per-service file naming: the service
// type name lower-cased, with no separator.
func bindingFileName(service string) string {
	return strings.ToLower(service) + ".js"
}
