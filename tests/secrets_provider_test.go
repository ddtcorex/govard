package tests

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/secrets"
)

type fakeSecretsProvider struct {
	name  string
	refs  map[string]string
	err   error
	calls []string
}

func (f *fakeSecretsProvider) Name() string {
	if strings.TrimSpace(f.name) == "" {
		return "fake"
	}
	return f.name
}

func (f *fakeSecretsProvider) Resolve(_ context.Context, ref string) (string, error) {
	f.calls = append(f.calls, ref)
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.refs[ref]
	if !ok {
		return "", errors.New("missing secret ref")
	}
	return value, nil
}

func TestSecretReferenceDetection(t *testing.T) {
	if !secrets.IsSecretReference("op://Engineering/demo/password") {
		t.Fatal("expected op:// reference to be detected")
	}
	if got := secrets.SecretProviderNameForReference("op://Engineering/demo/password"); got != "1password" {
		t.Fatalf("expected provider 1password, got %q", got)
	}
	if secrets.IsSecretReference("https://example.com") {
		t.Fatal("did not expect https URL to be treated as secret reference")
	}
}

func TestOPProviderResolveUsesOPRead(t *testing.T) {
	calls := 0
	provider := secrets.NewOPProviderWithRunner(func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls++
		if name != "op" {
			t.Fatalf("expected binary op, got %s", name)
		}
		joined := strings.Join(args, " ")
		if joined != "read op://Engineering/demo/password" {
			t.Fatalf("unexpected args: %s", joined)
		}
		return []byte("super-secret\n"), nil
	})

	value, err := provider.Resolve(context.Background(), "op://Engineering/demo/password")
	if err != nil {
		t.Fatalf("resolve secret: %v", err)
	}
	if value != "super-secret" {
		t.Fatalf("expected trimmed secret value, got %q", value)
	}
	if calls != 1 {
		t.Fatalf("expected one op invocation, got %d", calls)
	}
}

func TestSecretsProviderFactorySupportsAliases(t *testing.T) {
	for _, name := range []string{"1password", "op", "onepassword"} {
		provider, err := secrets.NewProvider(engine.ProviderRef{Kind: engine.ProviderKindSecrets, Name: name})
		if err != nil {
			t.Fatalf("new provider %s: %v", name, err)
		}
		if provider.Name() != "1password" {
			t.Fatalf("expected normalized provider name 1password, got %s", provider.Name())
		}
	}
}

func TestResolveRemoteConfigSecretsForTest(t *testing.T) {
	fake := &fakeSecretsProvider{refs: map[string]string{
		"op://Infra/staging/host": "staging.example.com",
		"op://Infra/staging/user": "deploy",
		"op://Infra/staging/path": "/srv/www/app",
		"op://Infra/staging/key":  "/opt/keys/staging",
	}}
	restore := cmd.SetSecretsProviderFactoryForTest(func(ref engine.ProviderRef) (secrets.Provider, error) {
		if ref.Kind != engine.ProviderKindSecrets {
			t.Fatalf("expected secrets provider kind, got %s", ref.Kind)
		}
		return fake, nil
	})
	defer restore()

	resolved, err := cmd.ResolveRemoteConfigSecretsForTest("staging", engine.RemoteConfig{
		Host: "op://Infra/staging/host",
		User: "op://Infra/staging/user",
		Path: "op://Infra/staging/path",
		Port: 22,
		Auth: engine.RemoteAuth{
			Method:  engine.RemoteAuthMethodKeyfile,
			KeyPath: "op://Infra/staging/key",
		},
	})
	if err != nil {
		t.Fatalf("resolve remote config secrets: %v", err)
	}
	if resolved.Host != "staging.example.com" {
		t.Fatalf("expected resolved host, got %q", resolved.Host)
	}
	if resolved.User != "deploy" {
		t.Fatalf("expected resolved user, got %q", resolved.User)
	}
	if resolved.Path != "/srv/www/app" {
		t.Fatalf("expected resolved path, got %q", resolved.Path)
	}
	if resolved.Auth.KeyPath != "/opt/keys/staging" {
		t.Fatalf("expected resolved key path, got %q", resolved.Auth.KeyPath)
	}
	if len(fake.calls) != 4 {
		t.Fatalf("expected 4 secret lookups, got %d", len(fake.calls))
	}
}

func TestSyncPlanResolvesRemoteSecrets(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo

domain: demo.test
recipe: laravel
remotes:
  staging:
    host: op://Infra/staging/host
    user: op://Infra/staging/user
    path: op://Infra/staging/path
`), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeSecretsProvider{refs: map[string]string{
		"op://Infra/staging/host": "staging.example.com",
		"op://Infra/staging/user": "deploy",
		"op://Infra/staging/path": "/srv/www/app",
	}}
	restore := cmd.SetSecretsProviderFactoryForTest(func(ref engine.ProviderRef) (secrets.Provider, error) {
		return fake, nil
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	buf := &strings.Builder{}
	root := cmd.RootCommandForTest()
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute sync plan with secret refs: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "remote: deploy@staging.example.com") {
		t.Fatalf("expected resolved remote target in plan output, got: %s", out)
	}
	if !strings.Contains(out, "root: /srv/www/app") {
		t.Fatalf("expected resolved remote path in plan output, got: %s", out)
	}
}

func TestSyncPlanFailsWhenSecretResolutionFails(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo

domain: demo.test
recipe: laravel
remotes:
  staging:
    host: op://Infra/staging/host
    user: deploy
    path: /srv/www/app
`), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := cmd.SetSecretsProviderFactoryForTest(func(ref engine.ProviderRef) (secrets.Provider, error) {
		_ = ref
		return &fakeSecretsProvider{err: errors.New("op unavailable")}, nil
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	buf := &strings.Builder{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file"})

	if err := root.Execute(); err != nil {
		t.Fatalf("expected sync --plan fallback instead of hard error, got: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "endpoint resolution failed") {
		t.Fatalf("expected endpoint resolution warning in plan output, got: %s", out)
	}
	if !strings.Contains(out, "remote 'staging' field host") {
		t.Fatalf("expected field context in fallback warning, got: %s", out)
	}
}
