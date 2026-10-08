package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/symfony"
)

func symfonyEnvLocal(t *testing.T, opts bootstrap.Options) string {
	t.Helper()
	dir := t.TempDir()
	opts.Runner = func(string) error { return nil }
	if err := symfony.NewSymfonyBootstrap(opts).Install(dir); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".env.local"))
	if err != nil {
		t.Fatalf("read .env.local: %v", err)
	}
	return string(data)
}

func TestSymfonyEnvLocalUsesResolvableMailHost(t *testing.T) {
	env := symfonyEnvLocal(t, bootstrap.Options{DBEngine: "mariadb", DBVersion: "10.11"})
	if !strings.Contains(env, "MAILER_DSN=smtp://mail:1025") {
		t.Fatalf("MAILER_DSN must use the resolvable mail host:\n%s", env)
	}
	if strings.Contains(env, "mailpit") {
		t.Fatalf("mailpit does not resolve inside the PHP container:\n%s", env)
	}
}

func TestSymfonyEnvLocalDerivesServerVersionFromStack(t *testing.T) {
	cases := []struct {
		engine, version, want string
	}{
		{"mariadb", "10.11", "serverVersion=10.11-MariaDB"},
		{"mysql", "8.4", "serverVersion=8.4&"},
	}
	for _, tc := range cases {
		env := symfonyEnvLocal(t, bootstrap.Options{DBEngine: tc.engine, DBVersion: tc.version})
		if !strings.Contains(env, tc.want) {
			t.Errorf("engine %s %s: want %q in:\n%s", tc.engine, tc.version, tc.want, env)
		}
		if strings.Contains(env, "11.4.0-MariaDB") {
			t.Errorf("hard-coded serverVersion leaked:\n%s", env)
		}
	}
}
