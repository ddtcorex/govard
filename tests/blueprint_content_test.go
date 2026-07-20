package tests

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/engine"

	"gopkg.in/yaml.v3"
)

func TestRenderMagento2Blueprint(t *testing.T) {
	testBlueprintRender(t, "magento2", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php-magento2:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: magento",
	})

	// Regression: web service must be connected to govard-proxy so Caddy can route to it.
	// Without this, `govard env up` succeeds but the site returns 502 Bad Gateway.
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)
	t.Setenv("GOVARD_BLUEPRINTS_DIR", func() string {
		_, filename, _, _ := runtime.Caller(0)
		return filepath.Join(filepath.Dir(filename), "..", "internal", "blueprints", "files")
	}())

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "magento2",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			Services: engine.Services{
				DB: "mariadb",
			},
		},
	}
	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("read compose file: %v", err)
	}

	var composeStruct struct {
		Services map[string]struct {
			Networks interface{} `yaml:"networks"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(content, &composeStruct); err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}
	webSvc, ok := composeStruct.Services["web"]
	if !ok {
		t.Fatal("web service not found in parsed compose file")
	}

	hasProxy := false
	if networks, ok := webSvc.Networks.([]interface{}); ok {
		for _, n := range networks {
			if n == "govard-proxy" {
				hasProxy = true
				break
			}
		}
	} else if networks, ok := webSvc.Networks.(map[string]interface{}); ok {
		if _, ok := networks["govard-proxy"]; ok {
			hasProxy = true
		}
	}
	if !hasProxy {
		t.Errorf("web service must be connected to govard-proxy network (required for Caddy routing). Networks: %v", webSvc.Networks)
	}
}

func TestRenderLaravelBlueprint(t *testing.T) {
	testBlueprintRender(t, "laravel", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: laravel",
	})
}

func TestRenderBlueprintAddsKnownProjectDomainsToPHPRuntimeHosts(t *testing.T) {
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:         "/workspace/project-b",
		ProjectName:  "project-b",
		Domain:       "project-b.test",
		ExtraDomains: []string{"brand-b.test"},
		Framework:    "laravel",
	}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	content := renderComposeWithConfig(t, engine.Config{
		ProjectName:    "project-a",
		Framework:      "laravel",
		Domain:         "project-a.test",
		LinkedProjects: []string{"project-b"},
		Stack: engine.Stack{
			Features: engine.Features{
				Xdebug: true,
			},
		},
	})

	if count := strings.Count(content, "project-b.test:host-gateway"); count != 2 {
		t.Fatalf("expected project-b.test host mapping twice (php + php-debug), got %d\n%s", count, content)
	}
}

func TestRenderBlueprintMountsGovardRootCAIntoPHPRuntimes(t *testing.T) {
	tempDir := t.TempDir()
	govardHome := setTestGovardHome(t, tempDir)

	// We need to set up blueprints in the tempDir because we are calling RenderBlueprint directly on it
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")
	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	certDir := filepath.Join(govardHome, "ssl")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("create cert dir: %v", err)
	}

	certPath := filepath.Join(certDir, "root.crt")
	if err := os.WriteFile(certPath, []byte("test-root-ca"), 0o644); err != nil {
		t.Fatalf("write root ca: %v", err)
	}

	config := engine.Config{
		ProjectName: "project-a",
		Framework:   "laravel",
		Domain:      "project-a.test",
		Stack: engine.Stack{
			Features: engine.Features{
				Xdebug: true,
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	contentBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	content := string(contentBytes)

	mount := certPath + ":/usr/local/share/ca-certificates/govard.crt:ro"
	if count := strings.Count(content, mount); count != 2 {
		t.Fatalf("expected Govard Root CA mount twice (php + php-debug), got %d\n%s", count, content)
	}
}

func TestPHPDebugServiceHasStackUlimitToPreventXdebugSegfault(t *testing.T) {
	// Alpine/musl allocates an ~80KB default thread stack (vs several MB on glibc).
	// Xdebug's own bug tracker (bugs.xdebug.org #2342) confirms that combined with
	// xdebug.start_with_request=yes, this overflows on ordinary recursive PHP calls
	// and crashes with SIGSEGV rather than a clean PHP error.
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "project-a",
		Framework:   "laravel",
		Domain:      "project-a.test",
		Stack: engine.Stack{
			Features: engine.Features{
				Xdebug: true,
			},
		},
	})

	var composeStruct struct {
		Services map[string]struct {
			Ulimits map[string]interface{} `yaml:"ulimits"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(content), &composeStruct); err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}

	debugSvc, ok := composeStruct.Services["php-debug"]
	if !ok {
		t.Fatal("php-debug service not found in parsed compose file")
	}

	stack, ok := debugSvc.Ulimits["stack"]
	if !ok {
		t.Fatalf("expected php-debug service to raise the stack ulimit; got ulimits: %v", debugSvc.Ulimits)
	}

	var stackBytes int64
	switch v := stack.(type) {
	case int:
		stackBytes = int64(v)
	case int64:
		stackBytes = v
	default:
		t.Fatalf("unexpected type for stack ulimit: %T (%v)", stack, stack)
	}

	const minStackBytes = 8 * 1024 * 1024
	if stackBytes < minStackBytes {
		t.Fatalf("expected php-debug stack ulimit >= %d bytes, got %d", minStackBytes, stackBytes)
	}
}

func TestPHPDebugServiceImageEncodesXdebugVersionOverride(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "project-a",
		Framework:   "laravel",
		Domain:      "project-a.test",
		Stack: engine.Stack{
			PHPVersion:    "8.4",
			XdebugVersion: "3.4.0",
			Features: engine.Features{
				Xdebug: true,
			},
		},
	})

	var composeStruct struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(content), &composeStruct); err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}

	debugSvc, ok := composeStruct.Services["php-debug"]
	if !ok {
		t.Fatal("php-debug service not found in parsed compose file")
	}

	const wantSuffix = "8.4-debug-xdebug-3.4.0"
	if !strings.HasSuffix(debugSvc.Image, wantSuffix) {
		t.Fatalf("expected php-debug image to end with %q, got %q", wantSuffix, debugSvc.Image)
	}
}

func TestPHPDebugServiceImageOmitsXdebugVersionSuffixWhenUnset(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "project-a",
		Framework:   "laravel",
		Domain:      "project-a.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Features: engine.Features{
				Xdebug: true,
			},
		},
	})

	var composeStruct struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(content), &composeStruct); err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}

	debugSvc, ok := composeStruct.Services["php-debug"]
	if !ok {
		t.Fatal("php-debug service not found in parsed compose file")
	}

	if !strings.HasSuffix(debugSvc.Image, "8.4-debug") {
		t.Fatalf("expected php-debug image to end with '8.4-debug', got %q", debugSvc.Image)
	}
	if strings.Contains(debugSvc.Image, "-xdebug-") {
		t.Fatalf("expected no xdebug version suffix when unset, got %q", debugSvc.Image)
	}
}

func TestRenderBlueprintReRendersWhenKnownProjectDomainsChange(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        "/workspace/project-b",
		ProjectName: "project-b",
		Domain:      "project-b.test",
		Framework:   "laravel",
	}); err != nil {
		t.Fatalf("seed initial registry entry: %v", err)
	}

	config := engine.Config{
		ProjectName:    "project-a",
		Framework:      "laravel",
		Domain:         "project-a.test",
		LinkedProjects: []string{"project-b", "project-c"},
		Stack: engine.Stack{
			Features: engine.Features{
				Xdebug: true,
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	before, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read initial compose: %v", err)
	}
	if strings.Contains(string(before), "project-c.test:host-gateway") {
		t.Fatalf("did not expect project-c.test before registry update:\n%s", string(before))
	}

	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        "/workspace/project-c",
		ProjectName: "project-c",
		Domain:      "project-c.test",
		Framework:   "wordpress",
	}); err != nil {
		t.Fatalf("seed updated registry entry: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(after), "project-c.test:host-gateway") {
		t.Fatalf("expected project-c.test host mapping after registry update:\n%s", string(after))
	}
}

func TestRenderBlueprintReRendersWhenGovardRootCAAppears(t *testing.T) {
	tempDir := t.TempDir()
	govardHome := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "project-a",
		Framework:   "laravel",
		Domain:      "project-a.test",
		Stack: engine.Stack{
			Features: engine.Features{
				Xdebug: true,
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	before, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read initial compose: %v", err)
	}
	if strings.Contains(string(before), "/usr/local/share/ca-certificates/govard.crt:ro") {
		t.Fatalf("did not expect Govard Root CA mount before certificate export:\n%s", string(before))
	}

	certDir := filepath.Join(govardHome, "ssl")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("create cert dir: %v", err)
	}
	certPath := filepath.Join(certDir, "root.crt")
	if err := os.WriteFile(certPath, []byte("test-root-ca"), 0o644); err != nil {
		t.Fatalf("write root ca: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read updated compose: %v", err)
	}
	if !strings.Contains(string(after), certPath+":/usr/local/share/ca-certificates/govard.crt:ro") {
		t.Fatalf("expected Govard Root CA mount after certificate export:\n%s", string(after))
	}
}

func TestRenderLaravelBlueprintWithQueue(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "laravel-queue-test",
		Framework:   "laravel",
		Domain:      "laravel-queue.test",
		Stack: engine.Stack{
			Features: engine.Features{
				Queue: true,
			},
			Services: engine.Services{
				Queue: "redis", // any non-empty value works for laravel's check
			},
		},
	})

	for _, expected := range []string{
		"queue:",
		"php artisan queue:work",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("Expected %q to be in generated compose file for laravel with queue enabled", expected)
		}
	}
}

func TestRenderNextjsBlueprint(t *testing.T) {
	testBlueprintRender(t, "nextjs", []string{
		"image: node:24",
		"working_dir: /app",
		"command: npm run dev -- --hostname 0.0.0.0 --port 80",
	})
}

func TestRenderEmdashBlueprint(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "emdash-test",
		Framework:   "emdash",
		Domain:      "emdash.test",
		Stack: engine.Stack{
			NodeVersion: "22",
		},
	})

	for _, expected := range []string{
		"image: node:22",
		"working_dir: /app",
		"exec npm run dev -- --host 0.0.0.0 --port 80 --allowed-hosts emdash.test",
		"GOVARD_TRUSTED_DOMAIN=emdash.test",
		"node-modules:/app/node_modules",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("Expected %q to be in generated compose file for emdash:\n%s", expected, content)
		}
	}
	if strings.Contains(content, "PM=") || strings.Contains(content, "$PM") {
		t.Fatalf("expected emdash compose output to avoid shell PM interpolation hazards:\n%s", content)
	}
}

func TestRenderEmdashBlueprintWithDetectedPNPM(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(`{"packageManager":"pnpm@10.11.0"}`), 0644); err != nil {
		t.Fatalf("failed to write package.json: %v", err)
	}

	config := engine.Config{
		ProjectName: "emdash-pnpm-test",
		Framework:   "emdash",
		Domain:      "emdash-pnpm.test",
		Stack: engine.Stack{
			NodeVersion: "22",
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	contentBytes, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	content := string(contentBytes)

	if !strings.Contains(content, "exec pnpm dev --host 0.0.0.0 --port 80 --allowed-hosts emdash-pnpm.test") {
		t.Fatalf("expected pnpm dev command in compose output:\n%s", content)
	}
	if !strings.Contains(content, "GOVARD_TRUSTED_DOMAIN=emdash-pnpm.test") {
		t.Fatalf("expected trusted forwarded domain env in compose output:\n%s", content)
	}
	if !strings.Contains(content, "corepack enable") {
		t.Fatalf("expected corepack bootstrapping in compose output:\n%s", content)
	}
	if strings.Contains(content, "PM=") || strings.Contains(content, "$PM") {
		t.Fatalf("expected pnpm compose output to avoid shell PM interpolation hazards:\n%s", content)
	}
}

func TestRenderNextjsBlueprintSkipsManagedWebServerAssets(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-nextjs-no-managed-web-assets",
		Framework:   "nextjs",
		Domain:      "nextjs.test",
		Stack: engine.Stack{
			NodeVersion: "24",
			WebServer:   "nginx",
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render nextjs blueprint: %v", err)
	}

	if _, err := os.Stat(filepath.Join(homeDir, "nginx", config.ProjectName, "default.conf")); !os.IsNotExist(err) {
		t.Fatalf("expected nextjs not to render managed nginx config, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "apache", config.ProjectName, "httpd.conf")); !os.IsNotExist(err) {
		t.Fatalf("expected nextjs not to render managed apache config, got err=%v", err)
	}
}

func TestRenderEmdashBlueprintSkipsManagedWebServerAssets(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-emdash-no-managed-web-assets",
		Framework:   "emdash",
		Domain:      "emdash.test",
		Stack: engine.Stack{
			NodeVersion: "22",
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render emdash blueprint: %v", err)
	}

	if _, err := os.Stat(filepath.Join(homeDir, "nginx", config.ProjectName, "default.conf")); !os.IsNotExist(err) {
		t.Fatalf("expected emdash not to render managed nginx config, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "apache", config.ProjectName, "httpd.conf")); !os.IsNotExist(err) {
		t.Fatalf("expected emdash not to render managed apache config, got err=%v", err)
	}
}

func TestRenderMagento1Blueprint(t *testing.T) {
	testBlueprintRender(t, "magento1", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php-magento1:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: magento",
	})
}

func TestRenderDrupalBlueprint(t *testing.T) {
	testBlueprintRender(t, "drupal", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: drupal",
	})
}

func TestRenderSymfonyBlueprint(t *testing.T) {
	testBlueprintRender(t, "symfony", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: symfony",
	})
}

func TestRenderShopwareBlueprint(t *testing.T) {
	testBlueprintRender(t, "shopware", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: shopware",
	})
}

func TestRenderCakephpBlueprint(t *testing.T) {
	testBlueprintRender(t, "cakephp", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: cakephp",
	})
}

func TestRenderWordpressBlueprint(t *testing.T) {
	testBlueprintRender(t, "wordpress", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: wordpress",
	})
}

func TestRenderPrestaShopBlueprint(t *testing.T) {
	testBlueprintRender(t, "prestashop", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: prestashop",
	})
}

func TestRenderCustomBlueprint(t *testing.T) {
	testBlueprintRender(t, "custom", []string{
		"image: ddtcorex/govard-nginx:1.28",
		"image: ddtcorex/govard-php:",
		"image: ddtcorex/govard-mariadb:",
		"MYSQL_DATABASE: app",
	})
}

func TestRenderBlueprintWithFeatures(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento-full",
		Framework:   "magento2",
		Domain:      "magento.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",

			DBVersion: "10.6",
			WebServer: "nginx",
			Services: engine.Services{
				DB:     "mariadb",
				Cache:  "redis",
				Search: "opensearch",
			},
			Features: engine.Features{
				Xdebug:  true,
				Varnish: true,
				Cache:   true,
				Search:  true,
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
	if !strings.Contains(contentStr, "aliases:") || !strings.Contains(contentStr, "- opensearch") {
		t.Error("Expected opensearch alias for elasticsearch service")
	}
}

func TestRenderMagento2BlueprintHybridWebServer(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento-hybrid",
		Framework:   "magento2",
		Domain:      "magento-hybrid.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",

			DBVersion: "11.4",
			Services: engine.Services{
				WebServer: "hybrid",
				DB:        "mariadb",
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
	if !strings.Contains(contentStr, "image: ddtcorex/govard-apache:2.4") {
		t.Fatalf("expected apache image in hybrid mode, got:\n%s", contentStr)
	}
}

func TestRenderMagento2BlueprintWithVarnishAcrossWebServers(t *testing.T) {
	cases := []struct {
		name                string
		webServer           string
		expectedWebImage    string
		expectApacheSidecar bool
	}{
		{
			name:             "nginx",
			webServer:        "nginx",
			expectedWebImage: "image: ddtcorex/govard-nginx:1.28",
		},
		{
			name:             "apache",
			webServer:        "apache",
			expectedWebImage: "image: ddtcorex/govard-apache:2.4",
		},
		{
			name:                "hybrid",
			webServer:           "hybrid",
			expectedWebImage:    "image: ddtcorex/govard-nginx:1.28",
			expectApacheSidecar: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			homeDir := setTestGovardHome(t, tempDir)

			_, filename, _, _ := runtime.Caller(0)
			projectRoot := filepath.Join(filepath.Dir(filename), "..")
			blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

			destBlueprintsDir := filepath.Join(tempDir, "blueprints")
			if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
				t.Fatalf("Failed to copy blueprints: %v", err)
			}

			config := engine.Config{
				ProjectName: "test-magento-varnish-" + tc.name,
				Framework:   "magento2",
				Domain:      "magento-varnish-" + tc.name + ".test",
				Stack: engine.Stack{
					PHPVersion: "8.4",
					Features: engine.Features{
						Varnish: true,
					},
					Services: engine.Services{
						WebServer: tc.webServer,
						DB:        "mariadb",
						Search:    "none",
						Cache:     "none",
						Queue:     "none",
					},
				},
			}

			if err := engine.RenderBlueprint(tempDir, config); err != nil {
				t.Fatalf("Failed to render blueprint: %v", err)
			}

			content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
			if err != nil {
				t.Fatalf("Failed to read generated compose file: %v", err)
			}
			contentStr := string(content)

			if !strings.Contains(contentStr, "varnish:") {
				t.Fatalf("expected varnish service in compose output, got:\n%s", contentStr)
			}
			if !strings.Contains(contentStr, tc.expectedWebImage) {
				t.Fatalf("expected web image %q, got:\n%s", tc.expectedWebImage, contentStr)
			}
			if !strings.Contains(contentStr, "image: ddtcorex/govard-varnish:8.0") {
				t.Fatalf("expected managed varnish image, got:\n%s", contentStr)
			}
			if !strings.Contains(contentStr, "- web") {
				t.Fatalf("expected varnish to depend on web service, got:\n%s", contentStr)
			}

			hasApacheSidecar := strings.Contains(contentStr, "\n    apache:\n")
			if tc.expectApacheSidecar && !hasApacheSidecar {
				t.Fatalf("expected apache sidecar for hybrid mode, got:\n%s", contentStr)
			}
			if !tc.expectApacheSidecar && hasApacheSidecar {
				t.Fatalf("did not expect apache sidecar for %s mode, got:\n%s", tc.webServer, contentStr)
			}

			vclPath := filepath.Join(homeDir, "varnish", config.ProjectName, "default.vcl")
			vclContent, err := os.ReadFile(vclPath)
			if err != nil {
				t.Fatalf("expected varnish VCL at %s: %v", vclPath, err)
			}
			vclStr := string(vclContent)

			if !strings.Contains(vclStr, `.host = "`+config.ProjectName+`-web-1"`) {
				t.Fatalf("expected varnish VCL backend host to target web service, got:\n%s", vclStr)
			}
			if !strings.Contains(vclStr, `.url = "/health_check.php"`) {
				t.Fatalf("expected varnish VCL health probe, got:\n%s", vclStr)
			}
		})
	}
}

func TestRenderMagento2BlueprintWithMageRunMappings(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento2-mage-run-map",
		Framework:   "magento2",
		Domain:      "main.test",
		StoreDomains: engine.StoreDomainMappings{
			"brand-a.test": {
				Code: "base",
				Type: "website",
			},
			"brand-b.test": {
				Code: "brand_b",
				Type: "store",
			},
		},
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "nginx",
				DB:        "mariadb",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "/etc/nginx/conf.d/mage-run-map.conf:ro") {
		t.Fatalf("expected nginx mage-run mapping volume in compose output, got:\n%s", contentStr)
	}

	mapPath := filepath.Join(homeDir, "nginx", config.ProjectName, "mage-run-map.conf")
	mapContent, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("expected nginx mage-run map file at %s: %v", mapPath, err)
	}
	mapStr := string(mapContent)

	for _, expected := range []string{"brand-a.test", "brand-b.test", "website", "store", "base", "brand_b"} {
		if !strings.Contains(mapStr, expected) {
			t.Fatalf("expected nginx mage-run map file to contain %q, got:\n%s", expected, mapStr)
		}
	}
}

func TestRenderMagento2BlueprintWithRenderedNginxConfig(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento2-nginx-default-conf",
		Framework:   "magento2",
		Domain:      "main.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			WebRoot:    "/pub",
			Services: engine.Services{
				WebServer: "nginx",
				DB:        "mariadb",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "/etc/nginx/templates/magento2.conf:ro") {
		t.Fatalf("expected rendered nginx template volume mount in compose output, got:\n%s", contentStr)
	}

	configPath := filepath.Join(homeDir, "nginx", config.ProjectName, "default.conf")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected rendered nginx default.conf at %s: %v", configPath, err)
	}
	configStr := string(configContent)

	for _, expected := range []string{"root $MAGE_ROOT/pub;", "fastcgi_param  MAGE_RUN_CODE $mage_run_code;", "location ~ (index|get|static|report|404|503|health_check)\\.php$"} {
		if !strings.Contains(configStr, expected) {
			t.Fatalf("expected rendered nginx default.conf to contain %q, got:\n%s", expected, configStr)
		}
	}
}

func TestRenderMagento2BlueprintHybridWithRenderedNginxConfig(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento2-hybrid-nginx-default-conf",
		Framework:   "magento2",
		Domain:      "main.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "hybrid",
				DB:        "mariadb",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "/etc/nginx/templates/hybrid.conf:ro") {
		t.Fatalf("expected hybrid web service to mount rendered nginx template, got:\n%s", contentStr)
	}

	configPath := filepath.Join(homeDir, "nginx", config.ProjectName, "default.conf")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected rendered hybrid nginx default.conf at %s: %v", configPath, err)
	}
	configStr := string(configContent)

	if !strings.Contains(configStr, "proxy_pass http://$apache_backend:80;") {
		t.Fatalf("expected hybrid nginx default.conf to proxy to apache backend, got:\n%s", configStr)
	}
}

func TestRenderMagento1BlueprintApacheWithMageRunMappings(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento1-mage-run-map",
		Framework:   "magento1",
		Domain:      "main.test",
		StoreDomains: engine.StoreDomainMappings{
			"brand-a.test": {
				Code: "base",
				Type: "website",
			},
			"brand-b.test": {
				Code: "brand_b",
				Type: "store",
			},
		},
		Stack: engine.Stack{
			PHPVersion: "8.1",
			Services: engine.Services{
				WebServer: "apache",
				DB:        "mariadb",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "/usr/local/apache2/conf") {
		t.Fatalf("expected apache config directory volume in compose output, got:\n%s", contentStr)
	}

	mapPath := filepath.Join(homeDir, "apache", config.ProjectName, "mage-run-map.conf")
	mapContent, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("expected apache mage-run map file at %s: %v", mapPath, err)
	}
	mapStr := string(mapContent)

	for _, expected := range []string{"brand-a\\.test", "brand-b\\.test", "MAGE_RUN_TYPE=website", "MAGE_RUN_TYPE=store", "MAGE_RUN_CODE=base", "MAGE_RUN_CODE=brand_b"} {
		if !strings.Contains(mapStr, expected) {
			t.Fatalf("expected apache mage-run map file to contain %q, got:\n%s", expected, mapStr)
		}
	}

	mirroredPath := filepath.Join(homeDir, "apache", config.ProjectName, "extra", "mage-run-map.conf")
	mirroredContent, err := os.ReadFile(mirroredPath)
	if err != nil {
		t.Fatalf("expected mirrored apache mage-run map file at %s: %v", mirroredPath, err)
	}
	if string(mirroredContent) != mapStr {
		t.Fatalf("expected mirrored apache mage-run map file to match root file, got:\nroot:\n%s\nmirrored:\n%s", mapStr, string(mirroredContent))
	}
}

func TestRenderMagento1BlueprintApacheWithRenderedHTTPDConfig(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento1-apache-httpd",
		Framework:   "magento1",
		Domain:      "main.test",
		Stack: engine.Stack{
			PHPVersion: "8.1",
			Services: engine.Services{
				WebServer: "apache",
				DB:        "mariadb",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "/usr/local/apache2/conf") {
		t.Fatalf("expected apache config directory volume mount in compose output, got:\n%s", contentStr)
	}

	configPath := filepath.Join(homeDir, "apache", config.ProjectName, "httpd.conf")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected rendered apache httpd.conf at %s: %v", configPath, err)
	}
	configStr := string(configContent)

	for _, expected := range []string{`DocumentRoot "/var/www/html/"`, `IncludeOptional conf/extra/mage-run-map.conf`, `<Directory "/var/www/html/">`, `AcceptPathInfo On`, `RewriteRule ^/index\.php/(.*)$ /index.php [L]`} {
		if !strings.Contains(configStr, expected) {
			t.Fatalf("expected rendered apache httpd.conf to contain %q, got:\n%s", expected, configStr)
		}
	}

	mimeTypesPath := filepath.Join(homeDir, "apache", config.ProjectName, "mime.types")
	if _, err := os.Stat(mimeTypesPath); err != nil {
		t.Fatalf("expected rendered apache mime.types at %s: %v", mimeTypesPath, err)
	}
}

func TestRenderMagento2BlueprintHybridWithRenderedApacheHTTPDConfig(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-magento2-hybrid-httpd",
		Framework:   "magento2",
		Domain:      "main.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			WebRoot:    "/pub",
			Services: engine.Services{
				WebServer: "hybrid",
				DB:        "mariadb",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "/usr/local/apache2/conf") {
		t.Fatalf("expected hybrid apache service to mount rendered config directory, got:\n%s", contentStr)
	}

	configPath := filepath.Join(homeDir, "apache", config.ProjectName, "httpd.conf")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected rendered hybrid apache httpd.conf at %s: %v", configPath, err)
	}
	configStr := string(configContent)

	if !strings.Contains(configStr, `DocumentRoot "/var/www/html/pub"`) {
		t.Fatalf("expected hybrid apache httpd.conf to render Magento 2 docroot /var/www/html/pub, got:\n%s", configStr)
	}
}

func testBlueprintRender(t *testing.T, framework string, expectedStrings []string) {
	t.Helper()

	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "test-" + framework,
		Framework:   framework,
		Domain:      framework + ".test",
		Stack: engine.Stack{
			PHPVersion:  "8.3",
			NodeVersion: "24",

			DBVersion: "10.6",
			WebServer: "nginx",
			Services: engine.Services{
				DB: "mariadb",
			},
			Features: engine.Features{
				Xdebug:  false,
				Varnish: false,
				Cache:   false,
				Search:  false,
			},
		},
	}

	err := engine.RenderBlueprint(tempDir, config)
	if err != nil {
		t.Fatalf("Failed to render %s blueprint: %v", framework, err)
	}

	outputPath := engine.ComposeFilePath(tempDir, config.ProjectName)
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}

	contentStr := string(content)

	for _, expected := range expectedStrings {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Expected '%s' to be in generated compose file for %s", expected, framework)
		}
	}

	if !strings.Contains(contentStr, "govard-net:") {
		t.Errorf("Expected govard-net network in %s compose file", framework)
	}
	if !strings.Contains(contentStr, "govard-proxy:") {
		t.Errorf("Expected govard-proxy network in %s compose file", framework)
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

func setTestGovardHome(t *testing.T, root string) string {
	t.Helper()

	homeDir := filepath.Join(root, ".govard-home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create test govard home: %v", err)
	}
	t.Setenv("GOVARD_HOME_DIR", homeDir)
	return homeDir
}
