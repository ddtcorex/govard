package tests

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
)

// aliasAuthFixture configures one remote named "staging" whose identity lives
// only in the auth store under the canonical name, and puts a fake ssh and a
// fake ssh-copy-id on PATH that append their argv to a log. A command that
// authenticates with the typed alias instead of the canonical name cannot find
// the key, so the log shows no -i for it.
func aliasAuthFixture(t *testing.T) (keyPath string, logPath string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(
		"project_name: test\nremotes:\n  staging:\n    host: example.invalid\n    user: deploy\n    path: /var/www\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keyPath = filepath.Join(dir, "canonical_key")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath+".pub", []byte("ssh-ed25519 AAAA test"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := json.Marshal(map[string]string{"remote.staging.key_path": keyPath})
	if err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(storePath, store, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOVARD_AUTH_STORE_PATH", storePath)
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", filepath.Join(dir, "audit.log"))
	t.Setenv("HOME", dir)

	logPath = filepath.Join(dir, "argv.log")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$0 $*\" >> \"$FAKE_ARGV_LOG\"\necho govard-remote-ok govard-rsync-ok\n"
	for _, name := range []string{"ssh", "ssh-copy-id"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_ARGV_LOG", logPath)

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return keyPath, logPath
}

func TestRemoteAliasResolvesTheCanonicalKey(t *testing.T) {
	cases := []struct {
		name string
		args []string
		key  func(keyPath string) string
	}{
		{"exec", []string{"remote", "exec", "stg", "--", "uptime"}, func(k string) string { return k }},
		{"test", []string{"remote", "test", "STAGE"}, func(k string) string { return k }},
		{"copy-id", []string{"remote", "copy-id", "stg"}, func(k string) string { return k + ".pub" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keyPath, logPath := aliasAuthFixture(t)
			root := cmd.RootCommandForTest()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("fake ssh was never invoked: %v", err)
			}
			want := "-i " + tc.key(keyPath)
			if !strings.Contains(string(data), want) {
				t.Fatalf("argv %q does not carry the canonical identity %q", string(data), want)
			}
		})
	}
}
