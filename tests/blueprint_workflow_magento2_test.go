package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/blueprints"
	"govard/internal/engine"
)

func TestRenderMagento2BlueprintPreparesAuditProfilerCustomMounts(t *testing.T) {
	for _, testCase := range []struct {
		webServer      string
		customDir      string
		containerMount string
	}{
		{webServer: "nginx", customDir: engine.ProjectNginxCustomDir, containerMount: "/etc/nginx/custom:ro"},
		{webServer: "apache", customDir: engine.ProjectApacheCustomDir, containerMount: "/usr/local/apache2/conf/custom:ro"},
		{webServer: "hybrid", customDir: engine.ProjectApacheCustomDir, containerMount: "/usr/local/apache2/conf/custom:ro"},
	} {
		t.Run(testCase.webServer, func(t *testing.T) {
			root := t.TempDir()
			setTestGovardHome(t, t.TempDir())
			if err := os.CopyFS(filepath.Join(engine.GovardHomeDir(), "blueprints"), blueprints.FS); err != nil {
				t.Fatal(err)
			}
			config := engine.Config{ProjectName: "audit-shop", Framework: "magento2", Domain: "audit-shop.test"}
			config.Stack.PHPVersion = "8.4"
			config.Stack.Services = engine.Services{WebServer: testCase.webServer, DB: "mariadb", Search: "none", Cache: "none", Queue: "none"}

			if err := engine.RenderBlueprint(root, config); err != nil {
				t.Fatal(err)
			}
			customPath := filepath.Join(root, testCase.customDir)
			if info, err := os.Stat(customPath); err != nil || !info.IsDir() {
				t.Fatalf("audit profiler custom directory %q was not prepared: %v", customPath, err)
			}
			compose, err := os.ReadFile(engine.ComposeFilePath(root, config.ProjectName))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(compose), customPath+":"+testCase.containerMount) {
				t.Fatalf("compose does not mount audit profiler custom dir:\n%s", compose)
			}
		})
	}
}

func TestRenderMagento2NginxIncludesAuditProfilerInsidePHPLocation(t *testing.T) {
	root := t.TempDir()
	setTestGovardHome(t, t.TempDir())
	if err := os.CopyFS(filepath.Join(engine.GovardHomeDir(), "blueprints"), blueprints.FS); err != nil {
		t.Fatal(err)
	}
	config := engine.Config{ProjectName: "audit-shop", Framework: "magento2", Domain: "audit-shop.test"}
	config.Stack.PHPVersion = "8.4"
	config.Stack.Services = engine.Services{WebServer: "nginx", DB: "mariadb", Search: "none", Cache: "none", Queue: "none"}

	if err := engine.RenderBlueprint(root, config); err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(filepath.Join(engine.GovardHomeDir(), "nginx", config.ProjectName, "default.conf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(rendered)
	phpLocation := strings.Index(content, "location ~ (index|get|static|report|404|503|health_check)\\.php$")
	includeLine := strings.Index(content, "include /etc/nginx/custom/audit-profiler/*.conf;")
	locationEnd := strings.Index(content[phpLocation:], "\n    }")
	if phpLocation < 0 || includeLine < phpLocation || locationEnd < 0 || includeLine >= phpLocation+locationEnd {
		t.Fatalf("audit profiler include is not inside Magento's PHP FastCGI location:\n%s", content)
	}
}

func TestRenderMagento2AuditProfilerMountDoesNotInvalidateNextRender(t *testing.T) {
	root := t.TempDir()
	setTestGovardHome(t, t.TempDir())
	if err := os.CopyFS(filepath.Join(engine.GovardHomeDir(), "blueprints"), blueprints.FS); err != nil {
		t.Fatal(err)
	}
	config := engine.Config{ProjectName: "audit-shop", Framework: "magento2", Domain: "audit-shop.test"}
	config.Stack.PHPVersion = "8.4"
	config.Stack.Services = engine.Services{WebServer: "nginx", DB: "mariadb", Search: "none", Cache: "none", Queue: "none"}

	if err := engine.RenderBlueprint(root, config); err != nil {
		t.Fatal(err)
	}
	hashPath := engine.ComposeFilePath(root, config.ProjectName) + ".hash"
	first, err := os.ReadFile(hashPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.RenderBlueprint(root, config); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(hashPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("audit profiler mount changed the next render hash: first=%q second=%q", first, second)
	}
}
