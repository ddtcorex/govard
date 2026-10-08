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

func symfonyConfigure(t *testing.T, envLocal string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(path, []byte(envLocal), 0o644); err != nil {
		t.Fatalf("write .env.local: %v", err)
	}
	opts := bootstrap.Options{DBHost: "db", DBUser: "app", DBPass: "secret", DBName: "app", DBEngine: "mariadb", DBVersion: "10.11", Runner: func(string) error { return nil }}
	if err := symfony.NewSymfonyBootstrap(opts).Configure(dir); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .env.local: %v", err)
	}
	return string(data)
}

func activeDatabaseURLs(env string) []string {
	var out []string
	for _, line := range strings.Split(env, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "DATABASE_URL=") {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func TestSymfonyConfigureReplacesExistingDatabaseURLCleanly(t *testing.T) {
	const want = `DATABASE_URL="mysql://app:secret@db:3306/app?serverVersion=10.11-MariaDB&charset=utf8mb4"`
	cases := map[string]string{
		"existing quoted":   "APP_ENV=dev\nDATABASE_URL=\"mysql://root:root@127.0.0.1:3306/old?serverVersion=8.0\"\nMAILER_DSN=null://null\n",
		"existing unquoted": "APP_ENV=dev\nDATABASE_URL=postgresql://u:p@127.0.0.1:5432/old\n",
		"commented only":    "APP_ENV=dev\n# DATABASE_URL=\"mysql://x:x@db:3306/x\"\n",
		"absent":            "APP_ENV=dev\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			env := symfonyConfigure(t, input)
			urls := activeDatabaseURLs(env)
			if len(urls) != 1 || urls[0] != want {
				t.Fatalf("want exactly %q, got %v in:\n%s", want, urls, env)
			}
			if strings.Contains(env, `""`) {
				t.Fatalf("doubled quotes in:\n%s", env)
			}
			if strings.Contains(input, "# DATABASE_URL") && !strings.Contains(env, "# DATABASE_URL=\"mysql://x:x@db:3306/x\"") {
				t.Fatalf("comment must stay untouched:\n%s", env)
			}
		})
	}
}

func TestSymfonyConfigureKeepsAlreadyConfiguredURL(t *testing.T) {
	input := "DATABASE_URL=\"mysql://app:secret@db:3306/app?serverVersion=10.11-MariaDB&charset=utf8mb4\"\n"
	if got := symfonyConfigure(t, input); got != input {
		t.Fatalf("an already configured URL must stay byte-identical, got:\n%s", got)
	}
}
