package tests

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestInitPreservesExistingRemotesAndHooks(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "govard.yml")
	config := `project_name: test
domain: test.test
recipe: magento2
remotes:
  dev:
    host: dev.example.com
    user: deploy
    path: /srv/www/dev
    environment: dev
hooks:
  post_sync:
    - name: warmup
      run: echo warmup
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--recipe", "magento2", "--framework-version", "2.4.7-p3"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}

	remotes, ok := out["remotes"].(map[string]interface{})
	if !ok || remotes["dev"] == nil {
		t.Fatalf("expected remotes.dev to be preserved, got %#v", out["remotes"])
	}

	hooks, ok := out["hooks"].(map[string]interface{})
	if !ok || hooks["post_sync"] == nil {
		t.Fatalf("expected hooks.post_sync to be preserved, got %#v", out["hooks"])
	}
}

func TestInitOmitsRuntimeUserAndGroupFromConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(`{"name":"demo/project"}`), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--recipe", "magento2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	configPath := filepath.Join(tempDir, "govard.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if strings.Contains(content, "user_id:") {
		t.Fatalf("expected govard.yml to omit user_id, got:\n%s", content)
	}
	if strings.Contains(content, "group_id:") {
		t.Fatalf("expected govard.yml to omit group_id, got:\n%s", content)
	}
	for _, key := range []string{
		"php_version:",
		"node_version:",
		"db_type:",
		"db_version:",
		"queue_version:",
		"web_server:",
		"services:",
		"search:",
		"cache:",
		"queue:",
		"varnish:",
	} {
		if !strings.Contains(content, key) {
			t.Fatalf("expected govard.yml to include %q, got:\n%s", key, content)
		}
	}
	if strings.Contains(content, "redis:") {
		t.Fatalf("expected derived feature redis to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "elasticsearch:") {
		t.Fatalf("expected derived feature elasticsearch to be omitted, got:\n%s", content)
	}

	cfg, err := engine.LoadBaseConfigFromDir(tempDir, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	uid := os.Getuid()
	gid := os.Getgid()
	if uid >= 0 && cfg.Stack.UserID != uid {
		t.Fatalf("expected runtime UserID %d, got %d", uid, cfg.Stack.UserID)
	}
	if gid >= 0 && cfg.Stack.GroupID != gid {
		t.Fatalf("expected runtime GroupID %d, got %d", gid, cfg.Stack.GroupID)
	}
}

func TestInitOmitsEmptyQueueVersionAndHooks(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(`{"name":"demo/project"}`), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--recipe", "symfony"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	configPath := filepath.Join(tempDir, "govard.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if strings.Contains(content, "queue_version:") {
		t.Fatalf("expected govard.yml to omit empty queue_version, got:\n%s", content)
	}
	if strings.Contains(content, "hooks:") {
		t.Fatalf("expected govard.yml to omit empty hooks, got:\n%s", content)
	}
}
