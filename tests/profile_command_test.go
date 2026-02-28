package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
)

func TestProfileCommandExists(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"config", "profile"})
	if err != nil {
		t.Fatalf("find config profile: %v", err)
	}
	if command == nil || command.Use != "profile" {
		t.Fatal("expected config profile command")
	}
}

func TestProfileJSONOutput(t *testing.T) {
	tempDir := t.TempDir()
	composer := `{"require":{"laravel/framework":"^11.0"}}`
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composer), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "profile", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("profile json parse: %v, output=%s", err, buf.String())
	}

	detected, ok := payload["detected"].(map[string]interface{})
	if !ok {
		t.Fatal("missing detected object")
	}
	if detected["framework"] != "laravel" {
		t.Fatalf("expected detected framework laravel, got %v", detected["framework"])
	}

	selected, ok := payload["selected"].(map[string]interface{})
	if !ok {
		t.Fatal("missing selected object")
	}
	if selected["framework"] != "laravel" {
		t.Fatalf("expected selected framework laravel, got %v", selected["framework"])
	}
	if _, ok := selected["php_version"]; !ok {
		t.Fatal("missing selected.php_version field")
	}
	if _, ok := selected["db_type"]; !ok {
		t.Fatal("missing selected.db_type field")
	}
}
