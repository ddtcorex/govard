//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// dockerFreeDocsPage is the reference page listing every command that runs
// without Docker.
const dockerFreeDocsPage = "docs/reference/docker-free.md"

// dockerFreeDocsPageVi is its Vietnamese counterpart, which carries the same
// table and is checked the same way so a translated page cannot drift.
const dockerFreeDocsPageVi = "docs/vi/reference/docker-free.md"

// dockerFreeDocsRow matches one row of that page's command table. Only rows
// whose first cell is a `govard ...` command are considered, so the page can
// carry unrelated tables (requirement legend, exit codes) without confusing the
// check.
var dockerFreeDocsRow = regexp.MustCompile("^\\| `(govard [^`]+)` \\| `([^`]+)` \\|$")

// TestDockerFreeDocsListMatchesCapabilities keeps the documented list of
// Docker-free commands in step with the shipped binary. The binary is the
// authority here rather than a package-level walk, because cobra registers
// `help` and the `completion` shells lazily, only once the root command runs —
// exactly the commands a hand-maintained list is most likely to drop.
func TestDockerFreeDocsListMatchesCapabilities(t *testing.T) {
	env := NewTestEnvironment(t)

	result := env.RunGovard(t, env.ProjectRoot, "capabilities", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("govard capabilities --json exited %d: %s%s", result.ExitCode, result.Stdout, result.Stderr)
	}

	var payload struct {
		SchemaVersion int `json:"schema_version"`
		Commands      []struct {
			Command  string `json:"command"`
			Requires string `json:"requires"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &payload); err != nil {
		t.Fatalf("parse capabilities --json: %v\n%s", err, result.Stdout)
	}
	if payload.SchemaVersion != 1 {
		t.Fatalf("capabilities schema_version = %d, want 1", payload.SchemaVersion)
	}

	fromBinary := map[string]string{}
	for _, row := range payload.Commands {
		if requiresDocker(row.Requires) {
			continue
		}
		if previous, duplicate := fromBinary[row.Command]; duplicate {
			t.Fatalf("capabilities lists %q twice (%q and %q)", row.Command, previous, row.Requires)
		}
		fromBinary[row.Command] = row.Requires
	}
	if len(fromBinary) == 0 {
		t.Fatal("capabilities reported no Docker-free command; the guard would be vacuous")
	}

	for _, page := range []string{dockerFreeDocsPage, dockerFreeDocsPageVi} {
		documented := parseDockerFreeDocsPage(t, env.ProjectRoot, page)

		for _, command := range sortedKeys(fromBinary) {
			requires := fromBinary[command]
			got, ok := documented[command]
			if !ok {
				t.Errorf("%s is Docker-free in govard capabilities but missing from %s", command, page)
				continue
			}
			if got != requires {
				t.Errorf("%s requires %q in govard capabilities but %q in %s", command, requires, got, page)
			}
		}
		for _, command := range sortedKeys(documented) {
			if _, ok := fromBinary[command]; !ok {
				t.Errorf("%s is documented as Docker-free in %s but govard capabilities does not list it", command, page)
			}
		}
	}
}

func requiresDocker(requires string) bool {
	for _, capability := range strings.Split(requires, ",") {
		if strings.TrimSpace(capability) == "docker" {
			return true
		}
	}
	return false
}

func sortedKeys(rows map[string]string) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// parseDockerFreeDocsPage reads the command table of the Docker-free page.
func parseDockerFreeDocsPage(t *testing.T, projectRoot, page string) map[string]string {
	t.Helper()

	path := filepath.Join(projectRoot, filepath.FromSlash(page))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", page, err)
	}

	documented := map[string]string{}
	for number, line := range strings.Split(string(content), "\n") {
		match := dockerFreeDocsRow.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		command := strings.TrimSpace(match[1])
		if previous, duplicate := documented[command]; duplicate {
			t.Fatalf("%s line %d: %s is listed twice (%q and %q)", page, number+1, command, previous, match[2])
		}
		documented[command] = strings.TrimSpace(match[2])
	}
	if len(documented) == 0 {
		t.Fatalf("%s lists no command rows; expected rows shaped `| `govard <path>` | `<requirement>` |`", page)
	}
	return documented
}

// TestDockerFreeDocsPageIsNavigable guards the page against the two ways a new
// docs page goes missing in this site: no sidebar entry, and no link from the
// pages a reader starts at.
func TestDockerFreeDocsPageIsNavigable(t *testing.T) {
	env := NewTestEnvironment(t)

	sidebar, err := os.ReadFile(filepath.Join(env.ProjectRoot, "docs", ".vitepress", "config.ts"))
	if err != nil {
		t.Fatalf("read vitepress config: %v", err)
	}
	for _, want := range []string{"'/reference/docker-free'", "'/vi/reference/docker-free'"} {
		if !strings.Contains(string(sidebar), want) {
			t.Errorf("docs/.vitepress/config.ts has no sidebar entry for %s", want)
		}
	}

	for _, page := range []string{
		"README.md",
		"docs/getting-started/installation.md",
		"docs/reference/cli-commands.md",
	} {
		content, err := os.ReadFile(filepath.Join(env.ProjectRoot, filepath.FromSlash(page)))
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		if !strings.Contains(string(content), "docker-free") {
			t.Errorf("%s does not link to the Docker-free reference page", page)
		}
	}

	// The Vietnamese page is what the SEO pass pairs with the English one for
	// hreflang; a missing counterpart silently drops that link.
	if _, err := os.Stat(filepath.Join(env.ProjectRoot, "docs", "vi", "reference", "docker-free.md")); err != nil {
		t.Errorf("docs/vi/reference/docker-free.md is missing: %v", err)
	}
}
