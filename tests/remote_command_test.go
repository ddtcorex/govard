package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"

	"gopkg.in/yaml.v3"
)

func TestRemoteAddWritesConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"remote", "add", "staging", "--host", "example.com", "--user", "deploy", "--path", "/var/www/html"})
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
	if !ok || remotes["staging"] == nil {
		t.Fatal("expected remotes.staging")
	}
	staging, ok := remotes["staging"].(map[string]interface{})
	if !ok {
		t.Fatal("expected remotes.staging object")
	}
	if ok && staging["environment"] != nil {
		t.Fatalf("expected environment field to be omitted from YAML, got %v", staging["environment"])
	}
	if staging["capabilities"] != nil {
		t.Fatalf("expected capabilities to be omitted from YAML by default (allowed), got %v", staging["capabilities"])
	}
}

func TestRemoteAddWithPortWritesConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"remote", "add", "staging", "--host", "example.com", "--user", "deploy", "--path", "/var/www/html", "--port", "2222"})
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

	remotes := out["remotes"].(map[string]interface{})
	staging := remotes["staging"].(map[string]interface{})
	if staging["port"] != 2222 {
		t.Fatalf("expected port 2222, got %v", staging["port"])
	}
}

func TestRemoteAddPreservesQuotedHomeRelativePath(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"remote", "add", "staging", "--host", "example.com", "--user", "deploy", "--path", "~/public_html"})
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
	if !ok || remotes["staging"] == nil {
		t.Fatal("expected remotes.staging")
	}
	staging, ok := remotes["staging"].(map[string]interface{})
	if !ok {
		t.Fatal("expected remotes.staging object")
	}

	if got := staging["path"]; got != "~/public_html" {
		t.Fatalf("expected remote path to remain home-relative, got %v", got)
	}
}

func TestRemoteAddKnownHostsEnablesStrictHostKey(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{
		"remote", "add", "staging",
		"--host", "example.com",
		"--user", "deploy",
		"--path", "/var/www/html",
		"--known-hosts-file", "/tmp/govard-known-hosts",
	})
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
	remotes := out["remotes"].(map[string]interface{})
	staging := remotes["staging"].(map[string]interface{})
	auth := staging["auth"].(map[string]interface{})
	if auth["strict_host_key"] != true {
		t.Fatalf("expected strict_host_key true, got %v", auth["strict_host_key"])
	}
	if auth["known_hosts_file"] != "/tmp/govard-known-hosts" {
		t.Fatalf("expected known_hosts_file set, got %v", auth["known_hosts_file"])
	}
}

func TestRemoteAddKeychainStoresKeyPathInAuthStore(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	storePath := filepath.Join(tempDir, "auth.json")
	t.Setenv("GOVARD_AUTH_STORE_PATH", storePath)

	// The key must exist: `remote add` refuses a key path that is not a readable
	// private key. HOME points at the temp dir so the tilde in --key-path still
	// exercises expansion, but against a real file.
	t.Setenv("HOME", tempDir)
	if err := os.MkdirAll(filepath.Join(tempDir, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(tempDir, ".ssh", "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"remote", "add", "staging",
		"--host", "example.com",
		"--user", "deploy",
		"--path", "/var/www/html",
		"--auth-method", "keychain",
		"--key-path", "~/.ssh/id_ed25519",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		t.Fatal(err)
	}

	remotes := cfg["remotes"].(map[string]interface{})
	staging := remotes["staging"].(map[string]interface{})
	if authRaw, ok := staging["auth"]; ok {
		auth := authRaw.(map[string]interface{})
		if _, exists := auth["method"]; exists {
			t.Fatalf("expected default keychain auth method to be omitted, got %v", auth["method"])
		}
		if _, exists := auth["key_path"]; exists {
			t.Fatalf("expected empty auth.key_path to be omitted, got %v", auth["key_path"])
		}
	}

	storeData, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read auth store: %v", err)
	}
	var entries map[string]string
	if err := json.Unmarshal(storeData, &entries); err != nil {
		t.Fatalf("parse auth store: %v", err)
	}
	storedKeyPath := entries["remote.staging.key_path"]
	if storedKeyPath != keyPath {
		t.Fatalf("auth store holds %q, want the normalised absolute path %q", storedKeyPath, keyPath)
	}
}

func TestRemoteAuditTailCommand(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\ndomain: test.test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(tempDir, "remote.log")
	logPayload := strings.Join([]string{
		`{"timestamp":"2026-02-12T00:00:00Z","operation":"remote.test.ssh","status":"success","remote":"staging","message":"ok"}`,
		`{"timestamp":"2026-02-12T00:00:01Z","operation":"sync.run","status":"plan","source":"staging","destination":"local","message":"plan generated"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(logPayload), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", logPath)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"remote", "audit", "tail", "--lines", "10", "--status", "plan"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sync.run") {
		t.Fatalf("expected sync.run in audit output, got: %s", out)
	}
	if strings.Contains(out, "remote.test.ssh") {
		t.Fatalf("did not expect filtered event in output, got: %s", out)
	}
}

func TestRemoteAuditStatsCommand(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\ndomain: test.test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(tempDir, "remote.log")
	logPayload := strings.Join([]string{
		`{"timestamp":"2026-02-12T00:00:00Z","operation":"remote.exec","status":"success","remote":"staging","message":"ok"}`,
		`{"timestamp":"2026-02-12T00:00:01Z","operation":"remote.exec","status":"failure","category":"auth","remote":"staging","message":"denied"}`,
		`{"timestamp":"2026-02-12T00:00:02Z","operation":"sync.run","status":"plan","source":"staging","destination":"local","message":"plan generated"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(logPayload), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", logPath)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"remote", "audit", "stats", "--lines", "10"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "events: 3") {
		t.Fatalf("expected event total in output, got: %s", out)
	}
	if !strings.Contains(out, "operation:") {
		t.Fatalf("expected operation section, got: %s", out)
	}
	if !strings.Contains(out, "remote.exec: 2") {
		t.Fatalf("expected remote.exec count, got: %s", out)
	}
}

func TestRemoteAuditStatsCommandJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\ndomain: test.test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(tempDir, "remote.log")
	logPayload := strings.Join([]string{
		`{"timestamp":"2026-02-12T00:00:00Z","operation":"remote.exec","status":"success","remote":"staging"}`,
		`{"timestamp":"2026-02-12T00:00:01Z","operation":"sync.run","status":"plan","source":"staging","destination":"local"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(logPayload), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", logPath)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"remote", "audit", "stats", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("parse json output: %v\n%s", err, buf.String())
	}
	if payload["total"] != float64(2) {
		t.Fatalf("expected total=2, got %#v", payload["total"])
	}
	byStatus, ok := payload["by_status"].(map[string]interface{})
	if !ok || byStatus["plan"] != float64(1) {
		t.Fatalf("expected by_status.plan=1, got %#v", payload["by_status"])
	}
}

func TestRemoteAuditTailCommandSinceFilter(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\ndomain: test.test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(tempDir, "remote.log")
	logPayload := strings.Join([]string{
		`{"timestamp":"2026-02-12T00:00:00Z","operation":"remote.exec","status":"success","remote":"staging"}`,
		`{"timestamp":"2026-02-13T00:00:00Z","operation":"sync.run","status":"plan","source":"staging","destination":"local"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(logPayload), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", logPath)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"remote", "audit", "tail", "--since", "2026-02-13"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sync.run") {
		t.Fatalf("expected sync.run in output, got: %s", out)
	}
	if strings.Contains(out, "remote.exec") {
		t.Fatalf("did not expect remote.exec in output, got: %s", out)
	}
}

func TestRemoteAuditStatsCommandSinceUntilFilter(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte("project_name: test\ndomain: test.test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(tempDir, "remote.log")
	logPayload := strings.Join([]string{
		`{"timestamp":"2026-02-11T00:00:00Z","operation":"remote.exec","status":"success","remote":"staging"}`,
		`{"timestamp":"2026-02-12T10:00:00Z","operation":"remote.exec","status":"failure","category":"auth","remote":"staging"}`,
		`{"timestamp":"2026-02-13T10:00:00Z","operation":"sync.run","status":"plan","source":"staging","destination":"local"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(logPayload), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", logPath)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"remote", "audit", "stats", "--since", "2026-02-12", "--until", "2026-02-12", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("parse json output: %v\n%s", err, buf.String())
	}
	if payload["total"] != float64(1) {
		t.Fatalf("expected total=1 for windowed stats, got %#v", payload["total"])
	}
	byOperation, ok := payload["by_operation"].(map[string]interface{})
	if !ok || byOperation["remote.exec"] != float64(1) {
		t.Fatalf("expected by_operation.remote.exec=1, got %#v", payload["by_operation"])
	}
}
