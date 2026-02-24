package tests

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestRenderMagento2Blueprint(t *testing.T) {
	testBlueprintRender(t, "magento2", []string{
		"image: govard/nginx:1.28",
		"image: govard/php-magento2:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: magento",
	})
}

func TestRenderLaravelBlueprint(t *testing.T) {
	testBlueprintRender(t, "laravel", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: laravel",
		"queue:",
		"php artisan queue:work",
	})
}

func TestRenderNextjsBlueprint(t *testing.T) {
	testBlueprintRender(t, "nextjs", []string{
		"image: node:24-alpine",
		"working_dir: /app",
		"command: npm run dev -- --hostname 0.0.0.0 --port 80",
	})
}

func TestRenderMagento1Blueprint(t *testing.T) {
	testBlueprintRender(t, "magento1", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: magento",
	})
}

func TestRenderDrupalBlueprint(t *testing.T) {
	testBlueprintRender(t, "drupal", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: drupal",
	})
}

func TestRenderSymfonyBlueprint(t *testing.T) {
	testBlueprintRender(t, "symfony", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: symfony",
	})
}

func TestRenderShopwareBlueprint(t *testing.T) {
	testBlueprintRender(t, "shopware", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: shopware",
	})
}

func TestRenderCakephpBlueprint(t *testing.T) {
	testBlueprintRender(t, "cakephp", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: cakephp",
	})
}

func TestRenderWordpressBlueprint(t *testing.T) {
	testBlueprintRender(t, "wordpress", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: wordpress",
	})
}

func TestRenderCustomBlueprint(t *testing.T) {
	testBlueprintRender(t, "custom", []string{
		"image: govard/nginx:1.28",
		"image: govard/php:",
		"image: govard/mariadb:",
		"MYSQL_DATABASE: app",
	})
}

func TestRenderBlueprintWithFeatures(t *testing.T) {
	tempDir := t.TempDir()

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento-full",
		Recipe:      "magento2",
		Domain:      "magento.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			DBType:     "mariadb",
			DBVersion:  "10.6",
			WebServer:  "nginx",
			Features: engine.Features{
				Xdebug:        true,
				Varnish:       true,
				Redis:         true,
				Elasticsearch: true,
			},
		},
	}

	err := engine.RenderBlueprint(tempDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint with features: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "XDEBUG_MODE: debug") {
		t.Error("Expected Xdebug to be enabled")
	}
	if !strings.Contains(contentStr, "varnish:") {
		t.Error("Expected varnish service")
	}
	if !strings.Contains(contentStr, "redis:") {
		t.Error("Expected redis service")
	}
	if !strings.Contains(contentStr, "elasticsearch:") {
		t.Error("Expected elasticsearch service")
	}
}

func TestRenderMagento2BlueprintHybridWebServer(t *testing.T) {
	tempDir := t.TempDir()

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento-hybrid",
		Recipe:      "magento2",
		Domain:      "magento-hybrid.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			DBType:     "mariadb",
			DBVersion:  "11.4",
			Services: engine.Services{
				WebServer: "hybrid",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	err := engine.RenderBlueprint(tempDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "web:") || !strings.Contains(contentStr, "- apache") {
		t.Fatalf("expected web service to depend on apache in hybrid mode, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "NGINX_TEMPLATE=hybrid.conf") {
		t.Fatalf("expected hybrid nginx template, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "apache:") || !strings.Contains(contentStr, "APACHE_DOCUMENT_ROOT=/var/www/html/pub") {
		t.Fatalf("expected apache sidecar service in hybrid mode, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "image: govard/apache:2.4") {
		t.Fatalf("expected apache image in hybrid mode, got:\n%s", contentStr)
	}
}

func testBlueprintRender(t *testing.T, recipe string, expectedStrings []string) {
	t.Helper()

	tempDir := t.TempDir()

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-" + recipe,
		Recipe:      recipe,
		Domain:      recipe + ".test",
		Stack: engine.Stack{
			PHPVersion:  "8.3",
			NodeVersion: "24",
			DBType:      "mariadb",
			DBVersion:   "10.6",
			WebServer:   "nginx",
			Features: engine.Features{
				Xdebug:        false,
				Varnish:       false,
				Redis:         false,
				Elasticsearch: false,
			},
		},
	}

	err := engine.RenderBlueprint(tempDir, config)
	if err != nil {
		t.Fatalf("Failed to render %s blueprint: %v", recipe, err)
	}

	outputPath := engine.ComposeFilePath(tempDir, config.ProjectName)
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}

	contentStr := string(content)

	for _, expected := range expectedStrings {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Expected '%s' to be in generated compose file for %s", expected, recipe)
		}
	}

	if !strings.Contains(contentStr, "govard-net:") {
		t.Errorf("Expected govard-net network in %s compose file", recipe)
	}
	if !strings.Contains(contentStr, "govard-proxy:") {
		t.Errorf("Expected govard-proxy network in %s compose file", recipe)
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
