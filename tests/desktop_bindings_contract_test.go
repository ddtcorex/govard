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

// A bound method that no route reaches is surface the frontend cannot use and
// CI cannot see: the test above only proves that every route resolves, not that
// every binding is reachable. Each entry here is a deliberate exception with a
// reason, so adding a Go method that nobody wires up fails instead of sitting
// in the bound surface unnoticed.
var unreachableBindingReasons = map[string]string{
	"EnvironmentService.GetEnvironmentURL": "called by OpenEnvironment in Go; the frontend gets the URL through that route",
	"EnvironmentService.OpenDocs":          "no caller yet; a docs action is not in the UI",
	"EnvironmentService.QuickAction":       "superseded by QuickActionForProject",
	"LogService.GetLogs":                   "superseded by GetLogsForService",
	"LogService.StartLogStream":            "superseded by StartLogStreamForService",
}

var bindingIndexImport = regexp.MustCompile(`import \* as (\w+) from "\./(\w+)\.js";`)
var bindingExport = regexp.MustCompile(`export function (\w+)\(`)

func TestDesktopEveryBindingIsRoutedOrListed(t *testing.T) {
	bindingsDir := filepath.Join("..", "desktop", "frontend", "bindings", "govard", "internal", "desktop")
	index, err := os.ReadFile(filepath.Join(bindingsDir, "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := os.ReadFile(filepath.Join("..", "desktop", "frontend", "services", "bridge.js"))
	if err != nil {
		t.Fatal(err)
	}
	bridgeSrc := string(bridge)

	exports := map[string]bool{}
	for _, m := range bindingIndexImport.FindAllStringSubmatch(string(index), -1) {
		service, file := m[1], m[2]
		src, err := os.ReadFile(filepath.Join(bindingsDir, file+".js"))
		if err != nil {
			t.Fatalf("read generated %s: %v", file, err)
		}
		for _, fn := range bindingExport.FindAllStringSubmatch(string(src), -1) {
			name := service + "." + fn[1]
			exports[name] = true
			if strings.Contains(bridgeSrc, `["`+service+`", "`+fn[1]+`"]`) {
				continue
			}
			if _, ok := unreachableBindingReasons[name]; ok {
				continue
			}
			t.Errorf("%s is bound to the frontend but no route reaches it; route it in bridge.js or list it in unreachableBindingReasons with a reason", name)
		}
	}
	if len(exports) < 55 {
		t.Fatalf("parsed %d generated methods, want at least 55", len(exports))
	}
	for name := range unreachableBindingReasons {
		if !exports[name] {
			t.Errorf("unreachableBindingReasons lists %s, which is not a generated method any more; drop the entry", name)
		}
	}
}
