package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// A remote that uses every field `remote add` has no flag for: url, the db
// credentials, paths.media and the whole nested deploy block are YAML-only, so
// a re-add that does not merge deletes them.
const remoteAddExistingYAML = `project_name: mergecase
framework: magento2
domain: mergecase.test
remotes:
    r1:
        host: first.example
        user: deploy
        port: 2222
        path: /var/www/app
        url: https://r1.example
        db_name: shopdb
        db_user: shopuser
        db_pass: secret
        protected: true
        capabilities:
            db: false
        paths:
            media: pub/media
        deploy:
            branch: release
            repository: git@example.com:shop.git
`

const remoteAddFreshYAML = `project_name: mergecase
framework: magento2
domain: mergecase.test
`

type remoteAddEnv struct {
	t      *testing.T
	config string
}

func newRemoteAddEnv(t *testing.T, configYAML string) *remoteAddEnv {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GOVARD_AUTH_STORE_PATH", filepath.Join(dir, "auth.json"))

	configPath := filepath.Join(dir, ".govard.yml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	return &remoteAddEnv{t: t, config: configPath}
}

// add runs one `remote add`, resetting the flags first: pflag never clears
// Changed, so in a single process a second invocation would look as if it had
// passed every flag the first one used. The flags are reset again on cleanup,
// because that state is package-global and would otherwise leak into the remote
// tests living in other files.
func (e *remoteAddEnv) add(args ...string) error {
	e.t.Helper()
	root := cmd.RootCommandForTest()
	addCmd, _, err := root.Find([]string{"remote", "add"})
	if err != nil {
		e.t.Fatalf("find remote add: %v", err)
	}
	resetCommandFlags(addCmd)
	e.t.Cleanup(func() { resetCommandFlags(addCmd) })

	root.SetArgs(append([]string{"remote", "add"}, args...))
	return root.Execute()
}

func (e *remoteAddEnv) list(args ...string) (string, error) {
	e.t.Helper()
	root := cmd.RootCommandForTest()
	listCmd, _, err := root.Find([]string{"remote", "list"})
	if err != nil {
		e.t.Fatalf("find remote list: %v", err)
	}
	resetCommandFlags(listCmd)

	// cobra hands the buffer to OutOrStdout, so the table never touches fd 1;
	// the writers are cleared again because rootCmd is a package-level command.
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	e.t.Cleanup(func() {
		root.SetOut(nil)
		root.SetErr(nil)
	})

	root.SetArgs(append([]string{"remote", "list"}, args...))
	// Not `return buf.String(), root.Execute()`: Go evaluates the operands
	// left to right, so the buffer would be read before the command ran.
	runErr := root.Execute()
	return buf.String(), runErr
}

func (e *remoteAddEnv) remote(name string) engine.RemoteConfig {
	e.t.Helper()
	data, err := os.ReadFile(e.config)
	if err != nil {
		e.t.Fatalf("read config: %v", err)
	}
	var doc struct {
		Remotes map[string]engine.RemoteConfig `yaml:"remotes"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		e.t.Fatalf("unmarshal config: %v", err)
	}
	remote, ok := doc.Remotes[name]
	if !ok {
		e.t.Fatalf("remote %q is not in the config:\n%s", name, data)
	}
	return remote
}

func resetCommandFlags(command *cobra.Command) {
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		if err := flag.Value.Set(flag.DefValue); err != nil {
			panic(err)
		}
		flag.Changed = false
	})
}

func TestRemoteAddReAddPreservesFieldsWithoutFlags(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddExistingYAML)
	if err := env.add("r1", "--host", "second.example"); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	got := env.remote("r1")

	if got.Host != "second.example" {
		t.Errorf("host = %q, want the value just passed", got.Host)
	}

	// Flags that exist but were not passed keep their previous value.
	if got.User != "deploy" || got.Port != 2222 || got.Path != "/var/www/app" {
		t.Errorf("unspecified flags were reset: user=%q port=%d path=%q", got.User, got.Port, got.Path)
	}

	// Fields with no flag at all can only survive by merging.
	if got.URL != "https://r1.example" {
		t.Errorf("url = %q, want it preserved", got.URL)
	}
	if got.DBName != "shopdb" || got.DBUser != "shopuser" || got.DBPass != "secret" {
		t.Errorf("db credentials were dropped: name=%q user=%q pass=%q", got.DBName, got.DBUser, got.DBPass)
	}
	if got.Paths.Media != "pub/media" {
		t.Errorf("paths.media = %q, want it preserved", got.Paths.Media)
	}
	if got.Deploy == nil || got.Deploy.Branch != "release" || got.Deploy.Repository != "git@example.com:shop.git" {
		t.Errorf("deploy block was dropped: %+v", got.Deploy)
	}
	if got.Protected == nil || !*got.Protected {
		t.Errorf("protected was dropped: %+v", got.Protected)
	}
	if got.Capabilities == nil || got.Capabilities.DB == nil || *got.Capabilities.DB {
		t.Errorf("capabilities.db:false was dropped: %+v", got.Capabilities)
	}
}

func TestRemoteAddCanStillClearProtectionAndCapabilities(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddExistingYAML)
	if err := env.add("r1", "--protected=false", "--capabilities", "none"); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	got := env.remote("r1")

	if got.Protected != nil && *got.Protected {
		t.Errorf("--protected=false must clear protection, got %+v", got.Protected)
	}
	if got.Capabilities != nil && got.Capabilities.DB != nil {
		t.Errorf("--capabilities none must reset the restriction, got %+v", got.Capabilities)
	}
}

func TestRemoteAddReplaceDropsUnspecifiedFields(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddExistingYAML)
	if err := env.add("r1", "--host", "second.example", "--user", "deploy", "--path", "/var/www/app", "--replace"); err != nil {
		t.Fatalf("remote add --replace: %v", err)
	}
	got := env.remote("r1")

	if got.Host != "second.example" {
		t.Errorf("host = %q, want the value passed", got.Host)
	}
	if got.DBPass != "" || got.URL != "" || got.Deploy != nil {
		t.Errorf("--replace must not merge: db_pass=%q url=%q deploy=%+v", got.DBPass, got.URL, got.Deploy)
	}
	if got.Protected != nil {
		t.Errorf("--replace must not carry protection over, got %+v", got.Protected)
	}
}

func TestRemoteAddNewRemoteTakesFlagDefaults(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	if err := env.add("fresh", "--host", "h.example", "--user", "u", "--path", "/tmp"); err != nil {
		t.Fatalf("remote add: %v", err)
	}
	got := env.remote("fresh")

	if got.Host != "h.example" || got.User != "u" || got.Path != "/tmp" {
		t.Errorf("new remote = %+v, want the passed values", got)
	}
	if got.Port != 22 {
		t.Errorf("port = %d, want the flag default 22", got.Port)
	}
	if got.Protected != nil {
		t.Errorf("a new remote must stay unprotected when --protected is omitted, got %+v", got.Protected)
	}
}

// The key path was invisible: with the default keychain method it lives only in
// the auth store, `.govard.yml` records nothing, and `remote list` showed
// neither the method nor the key — so "did --key-path take effect?" had no
// answer from the CLI. The source label comes from ResolveSSHKeyPath, which
// already reports whether the value came from config, the store, the
// environment or a default probe.
func TestRemoteListShowsAuthMethodAndResolvedKey(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)

	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := env.add("kf", "--host", "h.example", "--user", "u", "--path", "/tmp",
		"--auth-method", "keyfile", "--key-path", keyPath); err != nil {
		t.Fatalf("remote add: %v", err)
	}

	out, err := env.list()
	if err != nil {
		t.Fatalf("remote list: %v", err)
	}
	for _, want := range []string{"AUTH", "KEY", "keyfile", keyPath + " [config]"} {
		if !strings.Contains(out, want) {
			t.Errorf("remote list output is missing %q:\n%s", want, out)
		}
	}
}

func TestRemoteListShowsKeychainKeyFromTheAuthStore(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)

	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := env.add("kc", "--host", "h.example", "--user", "u", "--path", "/tmp",
		"--key-path", keyPath); err != nil {
		t.Fatalf("remote add: %v", err)
	}

	out, err := env.list()
	if err != nil {
		t.Fatalf("remote list: %v", err)
	}
	// The store entry is the only record of this key, so the row must name both
	// the path and where it came from.
	for _, want := range []string{"keychain", keyPath + " [store:keychain]"} {
		if !strings.Contains(out, want) {
			t.Errorf("remote list output is missing %q:\n%s", want, out)
		}
	}
}

func (e *remoteAddEnv) remotes() map[string]engine.RemoteConfig {
	e.t.Helper()
	data, err := os.ReadFile(e.config)
	if err != nil {
		e.t.Fatalf("read config: %v", err)
	}
	var doc struct {
		Remotes map[string]engine.RemoteConfig `yaml:"remotes"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		e.t.Fatalf("unmarshal config: %v", err)
	}
	return doc.Remotes
}

func (e *remoteAddEnv) authStore() map[string]string {
	e.t.Helper()
	data, err := os.ReadFile(os.Getenv("GOVARD_AUTH_STORE_PATH"))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}
		}
		e.t.Fatalf("read auth store: %v", err)
	}
	var entries map[string]string
	if err := json.Unmarshal(data, &entries); err != nil {
		e.t.Fatalf("parse auth store: %v", err)
	}
	return entries
}

// A key path that cannot be an ssh identity used to be stored as a success; the
// only symptom was an ssh "Identity file ... not accessible" warning buried in
// an error govard classifies as a network failure.
func TestRemoteAddRefusesAKeyPathThatDoesNotExist(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	missing := filepath.Join(t.TempDir(), "nope")

	err := env.add("bad", "--host", "h.example", "--user", "u", "--path", "/tmp", "--key-path", missing)
	if err == nil {
		t.Fatal("a key path that does not exist must be refused")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal should name the escape hatch: %v", err)
	}
	if _, saved := env.remotes()["bad"]; saved {
		t.Error("a refused key path must not save the remote")
	}
}

func TestRemoteAddForceStoresAKeyPathAnyway(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	missing := filepath.Join(t.TempDir(), "nope")

	if err := env.add("bad", "--host", "h.example", "--user", "u", "--path", "/tmp", "--key-path", missing, "--force"); err != nil {
		t.Fatalf("--force must store the path anyway: %v", err)
	}
	if got := env.authStore()["remote.bad.key_path"]; got != missing {
		t.Errorf("auth store holds %q, want %q", got, missing)
	}
}

func TestRemoteAddRefusesAPublicKeyAndNamesThePrivateOne(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "id_ed25519")
	public := private + ".pub"
	if err := os.WriteFile(private, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	if err := os.WriteFile(public, []byte("ssh-ed25519 AAAA"), 0o644); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	err := env.add("bad", "--host", "h.example", "--user", "u", "--path", "/tmp", "--key-path", public)
	if err == nil {
		t.Fatal("a public key must be refused")
	}
	if !strings.Contains(err.Error(), private) {
		t.Errorf("the refusal should name the private key %s: %v", private, err)
	}
}

// Both storage destinations must hold the same absolute path: the keychain
// branch normalised the value while the keyfile branch wrote the literal tilde.
func TestRemoteAddNormalisesTheKeyPathForBothAuthMethods(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	key := filepath.Join(home, ".ssh", "id_ed25519")
	if err := os.WriteFile(key, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("HOME", home)

	env := newRemoteAddEnv(t, remoteAddFreshYAML)

	if err := env.add("kf", "--host", "h.example", "--user", "u", "--path", "/tmp",
		"--auth-method", "keyfile", "--key-path", "~/.ssh/id_ed25519"); err != nil {
		t.Fatalf("keyfile add: %v", err)
	}
	if got := env.remote("kf").Auth.KeyPath; got != key {
		t.Errorf("keyfile stored %q in the config, want the absolute %q", got, key)
	}

	if err := env.add("kc", "--host", "h.example", "--user", "u", "--path", "/tmp",
		"--key-path", "~/.ssh/id_ed25519"); err != nil {
		t.Fatalf("keychain add: %v", err)
	}
	if got := env.authStore()["remote.kc.key_path"]; got != key {
		t.Errorf("keychain stored %q in the auth store, want the absolute %q", got, key)
	}
}

// Re-adding a keyfile remote without repeating --auth-method used to compare the
// flag's default (keychain) against nothing and push the rotated key into the
// auth store, blanking the config value — a silent migration of where the key
// lives, invisible in `.govard.yml`.
func TestRemoteAddKeyDestinationFollowsTheStoredAuthMethod(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "id_ed25519")
	rotated := filepath.Join(dir, "id_ed25519_rotated")
	for _, path := range []string{first, rotated} {
		if err := os.WriteFile(path, []byte("PRIVATE KEY"), 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
	}

	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	if err := env.add("kf", "--host", "h.example", "--user", "u", "--path", "/tmp",
		"--auth-method", "keyfile", "--key-path", first); err != nil {
		t.Fatalf("first add: %v", err)
	}

	// Rotate the key without naming --auth-method again.
	if err := env.add("kf", "--key-path", rotated); err != nil {
		t.Fatalf("re-add: %v", err)
	}

	if got := env.remote("kf").Auth.KeyPath; got != rotated {
		t.Errorf("auth.key_path = %q, want the rotated key %q kept in the config", got, rotated)
	}
	if got := env.remote("kf").Auth.Method; got != "keyfile" {
		t.Errorf("auth.method = %q, want the stored method preserved", got)
	}
	if got := env.authStore()["remote.kf.key_path"]; got != "" {
		t.Errorf("the re-add moved the key into the auth store: %q", got)
	}
}

// The sandbox is a real ssh target with a real identity — `sandbox up`
// provisions a keyfile — so its row names that identity instead of "-".
func TestRemoteListShowsTheSandboxIdentity(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)

	sandboxCfg := engine.RemoteConfig{
		Host:    "127.0.0.1",
		Port:    32998,
		User:    "deployer",
		Sandbox: true,
		Auth:    engine.RemoteAuth{Method: "keyfile", KeyPath: ".govard/sandbox/id_ed25519"},
	}
	restore := cmd.StubSandboxResolverForTest(func(context.Context, string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return sandboxCfg, deploy.SandboxLivenessRunning, nil
	})
	t.Cleanup(restore)

	out, err := env.list()
	if err != nil {
		t.Fatalf("remote list: %v", err)
	}
	for _, want := range []string{"running", "keyfile", ".govard/sandbox/id_ed25519 [config]"} {
		if !strings.Contains(out, want) {
			t.Errorf("the sandbox row is missing %q:\n%s", want, out)
		}
	}
}
