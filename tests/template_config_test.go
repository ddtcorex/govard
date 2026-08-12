package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNginxTemplatesIncludeXdebugRouting(t *testing.T) {
	templateFiles := []string{
		"cakephp.conf",
		"default.conf",
		"drupal.conf",
		"laravel.conf",
		"magento1.conf",
		"magento2.conf",
		"shopware.conf",
		"symfony.conf",
		"wordpress.conf",
	}

	for _, name := range templateFiles {
		content := readTemplateFile(t, name)

		if !strings.Contains(content, "XDEBUG_SESSION=") {
			t.Fatalf("Expected Xdebug routing in %s", name)
		}
		if !strings.Contains(content, "resolver 127.0.0.11 ipv6=off valid=30s;") {
			t.Fatalf("Expected Docker DNS resolver in %s", name)
		}
		if !strings.Contains(content, "php-debug:9000") {
			t.Fatalf("Expected php-debug upstream target in %s", name)
		}
		if !strings.Contains(content, "default php:9000;") {
			t.Fatalf("Expected php upstream target in %s", name)
		}
	}
}

func TestWordPressTemplateSupportsDirectoryAdminRouting(t *testing.T) {
	content := readTemplateFile(t, "wordpress.conf")

	if !strings.Contains(content, "try_files $uri $uri/ /index.php?$query_string;") {
		t.Fatalf("expected wordpress.conf to preserve directory requests like /wp-admin/ before falling back to index.php")
	}
}

func TestHybridTemplateProxiesToApache(t *testing.T) {
	content := readTemplateFile(t, "hybrid.conf")

	if !strings.Contains(content, "set $apache_backend apache;") {
		t.Fatalf("Expected hybrid template to define apache backend variable")
	}
	if !strings.Contains(content, "proxy_pass http://$apache_backend:80;") {
		t.Fatalf("Expected hybrid template to proxy requests to apache")
	}
}

func TestBaseBlueprintIncludesXdebugFixes(t *testing.T) {
	content := readBlueprintFile(t, "includes/base.yml")

	if !strings.Contains(content, "host.docker.internal:host-gateway") {
		t.Fatalf("Expected host.docker.internal mapping in base.yml")
	}
	if !strings.Contains(content, "start_with_request=yes") {
		t.Fatalf("Expected start_with_request=yes in base.yml")
	}
}

func readTemplateFile(t *testing.T, name string) string {
	t.Helper()

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	templatePath := filepath.Join(projectRoot, "docker", "nginx", "etc", "templates", name)

	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("Failed to read template %s: %v", name, err)
	}

	return string(content)
}

func readBlueprintFile(t *testing.T, name string) string {
	t.Helper()

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintPath := filepath.Join(projectRoot, "internal", "blueprints", "files", name)

	content, err := os.ReadFile(blueprintPath)
	if err != nil {
		t.Fatalf("Failed to read blueprint %s: %v", name, err)
	}

	return string(content)
}
