package tests

import (
	"strings"
	"testing"
)

// The Wails 3 migration moved the desktop frontend to Vite + React islands
// and the toolchain to Node.js 24; contributor docs must not teach the old facts.
func TestContributorDocsDropStaleDesktopFacts(t *testing.T) {
	for _, rel := range []string{
		"AGENTS.md",
		"docs/developer/architecture.md",
		"docs/vi/developer/architecture.md",
		"docs/developer/contributing.md",
		"docs/vi/developer/contributing.md",
	} {
		if strings.Contains(strings.ToLower(readRepoFile(t, rel)), "vanilla js") {
			t.Errorf("%s still describes the desktop frontend as vanilla JS", rel)
		}
	}
	agents := readRepoFile(t, "AGENTS.md")
	if strings.Contains(agents, "Node.js 20") {
		t.Error("AGENTS.md still lists Node.js 20 as the runtime")
	}
	if !strings.Contains(agents, "Node.js 24") {
		t.Error("AGENTS.md must list Node.js 24 (the desktop toolchain and CI floor)")
	}
	readme := readRepoFile(t, "README.md")
	if strings.Contains(readme, "needs Go 1.25+, Node 20+") {
		t.Error("README.md contributor build floor must be Node 24+")
	}
	if !strings.Contains(readme, "Node 20+, CLI only") {
		t.Error("README.md must keep the npm channel's own Node 20+ floor")
	}
}
