package tests

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every island container must exist in both HTML entry points and must be
// mounted from main.js. The mirror test only compares above the app-root marker,
// so a container added below it would silently not be mirrored; and a mount
// whose container is missing makes mountIsland return null, which drops the
// feature with no error anywhere.
func TestDesktopIslandContainersAreMirroredAndMounted(t *testing.T) {
	containers := []string{
		"metricsIsland", "updatePromptIsland", "logsIsland", "settingsDrawerMount",
		"envList", "projectHero", "activeServicesList", "envVarsList",
		"remotesIsland", "syncOptionsModalMount",
		"globalHealthIsland", "globalServicesList", "globalLogsIsland",
	}
	for _, entry := range []string{
		"../desktop/frontend/index.html",
		"../desktop/frontend/preview.html",
	} {
		data, err := os.ReadFile(entry)
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		for _, id := range containers {
			if !strings.Contains(string(data), `id="`+id+`"`) {
				t.Errorf("%s is missing the %s container", entry, id)
			}
		}
	}
	src := readRepoFile(t, "desktop/frontend/main.js")
	for _, id := range containers {
		if !regexp.MustCompile(`mountIsland\(\s*"` + id + `"`).MatchString(src) {
			t.Errorf("main.js never mounts %s", id)
		}
	}
}
