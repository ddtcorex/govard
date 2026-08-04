package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestApacheSupportTemplateMatchesDockerTemplate(t *testing.T) {
	projectRoot := testProjectRoot(t)
	assertFilesEqual(
		t,
		filepath.Join(projectRoot, "docker", "apache", "etc", "httpd.conf"),
		filepath.Join(projectRoot, "internal", "blueprints", "files", "support", "apache", "httpd.conf"),
	)
}

// frameworkNginxTemplateOwners maps an nginx support template's file name to
// the framework package that now owns it (relocated out of
// internal/blueprints/files/support/nginx/templates into
// internal/frameworks/<owner>/blueprint/ by the framework consolidation
// refactor). Templates with no entry here (default.conf, hybrid.conf) are
// generic and still live directly under internal/blueprints/files/support/nginx/templates.
var frameworkNginxTemplateOwners = map[string]string{
	"cakephp.conf":    "cakephp",
	"drupal.conf":     "drupal",
	"laravel.conf":    "laravel",
	"magento1.conf":   "magento1",
	"magento2.conf":   "magento2",
	"prestashop.conf": "prestashop",
	"shopware.conf":   "shopware",
	"symfony.conf":    "symfony",
	"wordpress.conf":  "wordpress",
}

// resolveNginxSupportTemplatePath returns the current on-disk path for an
// nginx support template, whether it still lives in the shared
// internal/blueprints/files support tree or was relocated to its owning
// framework package's blueprint/ directory.
func resolveNginxSupportTemplatePath(projectRoot, name string) string {
	if owner, ok := frameworkNginxTemplateOwners[name]; ok {
		return filepath.Join(projectRoot, "internal", "frameworks", owner, "blueprint", name)
	}
	return filepath.Join(projectRoot, "internal", "blueprints", "files", "support", "nginx", "templates", name)
}

func TestNginxSupportTemplatesMatchDockerTemplates(t *testing.T) {
	projectRoot := testProjectRoot(t)

	dockerDir := filepath.Join(projectRoot, "docker", "nginx", "etc", "templates")

	entries, err := os.ReadDir(dockerDir)
	if err != nil {
		t.Fatalf("read docker nginx templates: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".conf" {
			continue
		}
		assertFilesEqual(
			t,
			filepath.Join(dockerDir, entry.Name()),
			resolveNginxSupportTemplatePath(projectRoot, entry.Name()),
		)
	}
}

func testProjectRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..")
}

func assertFilesEqual(t *testing.T, leftPath, rightPath string) {
	t.Helper()

	left, err := os.ReadFile(leftPath)
	if err != nil {
		t.Fatalf("read %s: %v", leftPath, err)
	}
	right, err := os.ReadFile(rightPath)
	if err != nil {
		t.Fatalf("read %s: %v", rightPath, err)
	}

	if string(left) != string(right) {
		t.Fatalf("expected files to match:\nleft:  %s\nright: %s", leftPath, rightPath)
	}
}
