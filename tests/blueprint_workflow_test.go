package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/blueprints"
	"govard/internal/engine"

	"gopkg.in/yaml.v3"
)

func TestFullSetupLogic(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "full-setup-*")
	defer os.RemoveAll(tempDir)

	projectName := filepath.Base(tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: projectName,
		Framework:   "magento2",
		Domain:      projectName + ".test",
		Stack: engine.Stack{
			PHPVersion: "8.1",
			WebServer:  "nginx",
			Features: engine.Features{
				Varnish: true,
			},
		},
	}

	data, _ := yaml.Marshal(&config)
	_ = os.WriteFile(filepath.Join(tempDir, "govard.yml"), data, 0644)

	err := engine.RenderBlueprint(tempDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	renderPath := engine.ComposeFilePath(tempDir, config.ProjectName)
	rendered, _ := os.ReadFile(renderPath)

	if !strings.Contains(string(rendered), "govard-proxy") {
		t.Error("Rendered compose file missing govard-proxy network")
	}

	if !strings.Contains(string(rendered), "external: true") {
		t.Error("govard-proxy network should be marked as external")
	}
}

// TestRenderBlueprintReRendersWhenComposeFileMissing is a regression test for the bug where
// RenderBlueprintWithProfile would skip rendering (due to a matching hash) even when the
// rendered compose file had been deleted from disk — causing `govard env up` to fail with
// "no such file or directory" in the Start stage.
func TestRenderBlueprintReRendersWhenComposeFileMissing(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "render-missing-compose-*")
	defer os.RemoveAll(tempDir)

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "magento2",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
		},
	}

	// First render — produces compose file + hash.
	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	if _, err := os.Stat(composePath); err != nil {
		t.Fatalf("compose file missing after first render: %v", err)
	}

	// Simulate the compose file being deleted (e.g. manual cleanup, tmp-dir wipe).
	if err := os.Remove(composePath); err != nil {
		t.Fatalf("could not remove compose file: %v", err)
	}

	// Second render — config unchanged, so hash would normally cause a skip.
	// This must NOT skip, because the compose file is gone.
	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	if _, err := os.Stat(composePath); err != nil {
		t.Errorf("compose file still missing after second render (hash-skip regression): %v", err)
	}
}

func TestRenderBlueprintReRendersWhenBlueprintContentsChange(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	before, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read first compose file: %v", err)
	}
	if !strings.Contains(string(before), "govard-net:") {
		t.Fatalf("expected initial compose output to contain govard-net network, got:\n%s", string(before))
	}

	basePath := filepath.Join(destBlueprintsDir, "includes", "base.yml")
	baseContent, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read copied base blueprint: %v", err)
	}
	updated := strings.Replace(string(baseContent), "govard-net", "govard-net-reloaded", 1)
	if updated == string(baseContent) {
		t.Fatal("expected blueprint content replacement to change base.yml")
	}
	if err := os.WriteFile(basePath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write modified base blueprint: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read second compose file: %v", err)
	}
	if !strings.Contains(string(after), "- govard-net-reloaded") {
		t.Fatalf("expected compose output to re-render after blueprint change, got:\n%s", string(after))
	}
}

func TestRenderBlueprintReRendersWhenProjectComposeOverrideChanges(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	before, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read first compose file: %v", err)
	}
	if strings.Contains(string(before), "OVERRIDE_FLAG") {
		t.Fatalf("expected initial compose output to exclude override marker, got:\n%s", string(before))
	}

	overridePath := filepath.Join(tempDir, engine.ProjectComposeOverridePath)
	if err := os.MkdirAll(filepath.Dir(overridePath), 0o755); err != nil {
		t.Fatalf("create override dir: %v", err)
	}
	overrideContent := "services:\n  web:\n    environment:\n      OVERRIDE_FLAG: enabled\n"
	if err := os.WriteFile(overridePath, []byte(overrideContent), 0o644); err != nil {
		t.Fatalf("write compose override: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read second compose file: %v", err)
	}
	if !strings.Contains(string(after), "OVERRIDE_FLAG: enabled") {
		t.Fatalf("expected compose output to re-render after compose override change, got:\n%s", string(after))
	}
}

func TestRenderBlueprintReRendersWhenNginxCustomConfigDirChanges(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	hashPath := engine.ComposeFilePath(tempDir, config.ProjectName) + ".hash"
	beforeHash, err := os.ReadFile(hashPath)
	if err != nil {
		t.Fatalf("read first hash: %v", err)
	}

	customDir := filepath.Join(tempDir, engine.ProjectNginxCustomDir)
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("create nginx custom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "extra.conf"), []byte("location /extra { return 200; }\n"), 0o644); err != nil {
		t.Fatalf("write nginx custom snippet: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	afterHash, err := os.ReadFile(hashPath)
	if err != nil {
		t.Fatalf("read second hash: %v", err)
	}
	if string(beforeHash) == string(afterHash) {
		t.Fatalf("expected render hash to change after adding .govard/nginx/custom snippet, got same hash: %s", string(afterHash))
	}
}

func TestRenderBlueprintReRendersWhenApacheCustomConfigDirChanges(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "apache",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	hashPath := engine.ComposeFilePath(tempDir, config.ProjectName) + ".hash"
	beforeHash, err := os.ReadFile(hashPath)
	if err != nil {
		t.Fatalf("read first hash: %v", err)
	}

	customDir := filepath.Join(tempDir, engine.ProjectApacheCustomDir)
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("create apache custom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "extra.conf"), []byte("Alias /extra /var/www/html/extra\n"), 0o644); err != nil {
		t.Fatalf("write apache custom snippet: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	afterHash, err := os.ReadFile(hashPath)
	if err != nil {
		t.Fatalf("read second hash: %v", err)
	}
	if string(beforeHash) == string(afterHash) {
		t.Fatalf("expected render hash to change after adding .govard/apache/custom snippet, got same hash: %s", string(afterHash))
	}
}

func TestRenderBlueprintReRendersWhenPackageManagerSignalChanges(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "package-lock.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write package-lock.json: %v", err)
	}

	config := engine.Config{
		ProjectName: "emdash-pm-switch",
		Framework:   "emdash",
		Domain:      "emdash-pm-switch.test",
		Stack: engine.Stack{
			NodeVersion: "22",
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	before, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read first compose file: %v", err)
	}
	if !strings.Contains(string(before), "exec npm run dev -- --host 0.0.0.0 --port 80 --allowed-hosts emdash-pm-switch.test") {
		t.Fatalf("expected initial npm compose output, got:\n%s", string(before))
	}

	if err := os.WriteFile(filepath.Join(tempDir, "pnpm-workspace.yaml"), []byte("packages:\n  - .\n"), 0o644); err != nil {
		t.Fatalf("write pnpm-workspace.yaml: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read second compose file: %v", err)
	}
	if !strings.Contains(string(after), "exec pnpm dev --host 0.0.0.0 --port 80 --allowed-hosts emdash-pm-switch.test") {
		t.Fatalf("expected compose output to re-render for pnpm, got:\n%s", string(after))
	}
}

func TestRenderBlueprintReRendersWhenSSHAuthSockChanges(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	t.Setenv("SSH_AUTH_SOCK", "/tmp/ssh-old.sock")
	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	before, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read first compose file: %v", err)
	}
	if !strings.Contains(string(before), "/tmp/ssh-old.sock:/ssh-agent") {
		t.Fatalf("expected compose output to contain first SSH_AUTH_SOCK mount, got:\n%s", string(before))
	}

	t.Setenv("SSH_AUTH_SOCK", "/tmp/ssh-new.sock")
	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read second compose file: %v", err)
	}
	if !strings.Contains(string(after), "/tmp/ssh-new.sock:/ssh-agent") {
		t.Fatalf("expected compose output to re-render after SSH_AUTH_SOCK change, got:\n%s", string(after))
	}
	if strings.Contains(string(after), "/tmp/ssh-old.sock:/ssh-agent") {
		t.Fatalf("expected old SSH_AUTH_SOCK mount to be replaced, got:\n%s", string(after))
	}
}

func TestRenderBlueprintIncludesNginxCustomConfigDir(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	nginxConfPath := filepath.Join(engine.GovardHomeDir(), "nginx", config.ProjectName, "default.conf")
	before, err := os.ReadFile(nginxConfPath)
	if err != nil {
		t.Fatalf("read first nginx conf: %v", err)
	}
	if strings.Contains(string(before), "/etc/nginx/custom") {
		t.Fatalf("expected no custom include before .govard/nginx/custom exists, got:\n%s", string(before))
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	beforeCompose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read first compose file: %v", err)
	}
	if strings.Contains(string(beforeCompose), "/etc/nginx/custom") {
		t.Fatalf("expected no custom volume before .govard/nginx/custom exists, got:\n%s", string(beforeCompose))
	}

	customDir := filepath.Join(tempDir, engine.ProjectNginxCustomDir)
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("create nginx custom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "extra.conf"), []byte("location /extra { return 200; }\n"), 0o644); err != nil {
		t.Fatalf("write nginx custom snippet: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(nginxConfPath)
	if err != nil {
		t.Fatalf("read second nginx conf: %v", err)
	}
	if !strings.Contains(string(after), "include /etc/nginx/custom/*.conf;") {
		t.Fatalf("expected nginx conf to include custom directory, got:\n%s", string(after))
	}

	afterCompose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read second compose file: %v", err)
	}
	if !strings.Contains(string(afterCompose), customDir+":/etc/nginx/custom:ro") {
		t.Fatalf("expected compose output to mount nginx custom config dir, got:\n%s", string(afterCompose))
	}
}

func TestRenderBlueprintIncludesApacheCustomConfigDir(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "apache",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	httpdConfPath := filepath.Join(engine.GovardHomeDir(), "apache", config.ProjectName, "httpd.conf")
	before, err := os.ReadFile(httpdConfPath)
	if err != nil {
		t.Fatalf("read first httpd.conf: %v", err)
	}
	if strings.Contains(string(before), "conf/custom") {
		t.Fatalf("expected no custom include before .govard/apache/custom exists, got:\n%s", string(before))
	}

	customDir := filepath.Join(tempDir, engine.ProjectApacheCustomDir)
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("create apache custom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "extra.conf"), []byte("Alias /extra /var/www/html/extra\n"), 0o644); err != nil {
		t.Fatalf("write apache custom snippet: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	after, err := os.ReadFile(httpdConfPath)
	if err != nil {
		t.Fatalf("read second httpd.conf: %v", err)
	}
	if !strings.Contains(string(after), "IncludeOptional conf/custom/*.conf") {
		t.Fatalf("expected httpd.conf to include custom directory, got:\n%s", string(after))
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	afterCompose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose file: %v", err)
	}
	if !strings.Contains(string(afterCompose), customDir+":/usr/local/apache2/conf/custom:ro") {
		t.Fatalf("expected compose output to mount apache custom config dir, got:\n%s", string(afterCompose))
	}
}

func TestRenderBlueprintIncludesApacheCustomConfigDirInHybridMode(t *testing.T) {
	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "custom",
		Domain:      "sample-project.test",
		Stack: engine.Stack{
			PHPVersion: "8.4",
			Services: engine.Services{
				WebServer: "hybrid",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	customDir := filepath.Join(tempDir, engine.ProjectApacheCustomDir)
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("create apache custom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customDir, "extra.conf"), []byte("Alias /extra /var/www/html/extra\n"), 0o644); err != nil {
		t.Fatalf("write apache custom snippet: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("render failed: %v", err)
	}

	httpdConfPath := filepath.Join(engine.GovardHomeDir(), "apache", config.ProjectName, "httpd.conf")
	rendered, err := os.ReadFile(httpdConfPath)
	if err != nil {
		t.Fatalf("read httpd.conf: %v", err)
	}
	if !strings.Contains(string(rendered), "IncludeOptional conf/custom/*.conf") {
		t.Fatalf("expected httpd.conf to include custom directory in hybrid mode, got:\n%s", string(rendered))
	}

	composePath := engine.ComposeFilePath(tempDir, config.ProjectName)
	renderedCompose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose file: %v", err)
	}
	if !strings.Contains(string(renderedCompose), customDir+":/usr/local/apache2/conf/custom:ro") {
		t.Fatalf("expected hybrid apache service to mount apache custom config dir, got:\n%s", string(renderedCompose))
	}
}
