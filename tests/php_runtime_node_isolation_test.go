package tests

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPHPRuntimeDoesNotBundleOrConfigureNode(t *testing.T) {
	for _, check := range []struct {
		path      string
		forbidden []string
	}{
		{
			path: filepath.Join("docker", "php", "Dockerfile"),
			forbidden: []string{
				"nodejs",
				"npm install -g",
			},
		},
		{
			path: filepath.Join("docker", "php", "etc", "entrypoint.sh"),
			forbidden: []string{
				"NODE_VERSION",
				"unofficial-builds.nodejs.org",
			},
		},
		{
			path: filepath.Join("internal", "blueprints", "files", "includes", "base.yml"),
			forbidden: []string{
				"NODE_VERSION:",
			},
		},
	} {
		t.Run(check.path, func(t *testing.T) {
			content := readProjectFileForTest(t, check.path)
			for _, value := range check.forbidden {
				if strings.Contains(content, value) {
					t.Fatalf("PHP runtime must not contain %q in %s", value, check.path)
				}
			}
		})
	}
}
