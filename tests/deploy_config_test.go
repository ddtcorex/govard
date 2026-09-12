package tests

import (
	"errors"
	"govard/internal/frameworks/magento2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
)

// writeFile writes a fixture file, creating parent directories as needed.
// Shared by the deploy test files in this package.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDeployConfigDecodesAndDefaults(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  keep_releases: 3
  verify:
    url: https://sample.test
    timeout: 15s
  hooks:
    - name: reload
      on: publish:activate
      position: after
      run: touch /tmp/reload
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    branch: staging
    publish: symlink
`)

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Deploy.KeepReleases != 3 {
		t.Fatalf("keep_releases = %d, want 3", cfg.Deploy.KeepReleases)
	}
	if cfg.Deploy.Verify.Timeout != "15s" {
		t.Fatalf("verify.timeout = %q, want 15s", cfg.Deploy.Verify.Timeout)
	}
	if cfg.Deploy.Verify.URL != "https://sample.test" {
		t.Fatalf("verify.url = %q, want https://sample.test", cfg.Deploy.Verify.URL)
	}
	if len(cfg.Deploy.Hooks) != 1 || cfg.Deploy.Hooks[0].Name != "reload" {
		t.Fatalf("hooks = %+v, want one hook named reload", cfg.Deploy.Hooks)
	}
	remote := cfg.Remotes["staging"]
	if remote.Branch != "staging" || remote.Publish != "symlink" {
		t.Fatalf("remote topology = %+v, want branch=staging publish=symlink", remote)
	}
}

func TestDeployConfigDefaultsAreAppliedWhenAbsent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := cfg.Deploy.KeepReleasesOr(); got != engine.DefaultKeepReleases {
		t.Fatalf("KeepReleasesOr() = %d, want %d", got, engine.DefaultKeepReleases)
	}
	if cfg.Deploy.CommandTimeout != "" {
		t.Fatalf("command_timeout default = %q, want empty so the executor applies its own default", cfg.Deploy.CommandTimeout)
	}
	if cfg.Remotes["staging"].Local {
		t.Fatal("local must default to false")
	}
}

func TestNormalizeDeployConfigTrimsValues(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  artifact_dir: "  artifacts  "
  verify:
    url: "  https://sample.test  "
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    branch: "  staging  "
    publish: "  SYMLINK  "
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Deploy.ArtifactDir != "artifacts" {
		t.Fatalf("artifact_dir = %q, want trimmed", cfg.Deploy.ArtifactDir)
	}
	if cfg.Deploy.Verify.URL != "https://sample.test" {
		t.Fatalf("verify.url = %q, want trimmed", cfg.Deploy.Verify.URL)
	}
	if cfg.Remotes["staging"].Branch != "staging" || cfg.Remotes["staging"].Publish != "symlink" {
		t.Fatalf("remote = %+v, want trimmed branch and lowercased publish", cfg.Remotes["staging"])
	}
}

func TestResolveOptionsLayerPrecedence(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  keep_releases: 5
  verify:
    url: https://project.example.com
  settings:
    static_content_locales: en_US
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    branch: staging
    deploy:
      keep_releases: 2
      verify:
        url: https://staging.example.com
      settings:
        static_content_locales: en_US fr_FR
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	opts, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if opts.KeepReleases != 2 {
		t.Fatalf("keep_releases = %d, want the per-remote override 2", opts.KeepReleases)
	}
	if opts.VerifyURL != "https://staging.example.com" {
		t.Fatalf("verify url = %q, want the per-remote override", opts.VerifyURL)
	}
	if opts.Settings["static_content_locales"] != "en_US fr_FR" {
		t.Fatalf("settings = %v, want the per-remote override", opts.Settings)
	}
	if opts.Branch != "staging" {
		t.Fatalf("branch = %q, want staging", opts.Branch)
	}
	if !opts.Verify || opts.SkipLock {
		t.Fatalf("verify must default to true and locking must default to on, got verify=%v skip_lock=%v", opts.Verify, opts.SkipLock)
	}
	if opts.CommandTimeout <= 0 || opts.VerifyTimeout <= 0 {
		t.Fatalf("timeouts must default to positive values, got %s / %s", opts.CommandTimeout, opts.VerifyTimeout)
	}

	forced, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{Branch: "hotfix", KeepReleases: 9})
	if err != nil {
		t.Fatalf("resolve with flags: %v", err)
	}
	if forced.Branch != "hotfix" || forced.KeepReleases != 9 {
		t.Fatalf("flag override = %+v, want branch=hotfix keep=9", forced)
	}
}

func TestResolveOptionsRejectsUnknownRemoteAndBadSelectors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    branch: staging
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if _, err := deploy.ResolveOptionsForTest(cfg, "nope", deploy.Overrides{}); err == nil {
		t.Fatal("want an error for an unknown remote")
	}
	if _, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{Revision: "abc", Tag: "v1"}); err == nil {
		t.Fatal("want an error when --revision and --tag are combined")
	}
}

// writeDeployConfig writes a minimal project with one remote, so the build-mode
// cases below differ only in the value under test.
func writeDeployConfig(t *testing.T, deployBlock string) engine.Config {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
`+deployBlock+`
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    branch: staging
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func TestResolveOptionsBuildModeMatrix(t *testing.T) {
	cases := []struct {
		name       string
		over       deploy.Overrides
		configured string
		want       string
	}{
		{name: "auto without an artifact is a server build", want: deploy.BuildServer},
		{name: "auto with a flag artifact dir is an artifact build", over: deploy.Overrides{ArtifactDir: "artifacts"}, want: deploy.BuildArtifact},
		{name: "auto with a configured artifact dir is an artifact build", configured: "artifacts", want: deploy.BuildArtifact},
		{name: "an explicit server build wins over a supplied artifact", over: deploy.Overrides{Build: deploy.BuildServer, ArtifactDir: "artifacts"}, want: deploy.BuildServer},
		{name: "an explicit artifact build needs a supplied artifact", over: deploy.Overrides{Build: deploy.BuildArtifact, ArtifactDir: "artifacts"}, want: deploy.BuildArtifact},
		{name: "an explicit auto behaves like the default", over: deploy.Overrides{Build: deploy.BuildAuto, ArtifactDir: "artifacts"}, want: deploy.BuildArtifact},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := ""
			if tc.configured != "" {
				block = "deploy:\n  artifact_dir: " + tc.configured
			}
			cfg := writeDeployConfig(t, block)
			opts, err := deploy.ResolveOptionsForTest(cfg, "staging", tc.over)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if opts.Build != tc.want {
				t.Fatalf("build mode = %q, want %q", opts.Build, tc.want)
			}
		})
	}
}

func TestResolveOptionsBuildModeWithoutAnArtifactIsAnError(t *testing.T) {
	cfg := writeDeployConfig(t, "")
	_, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{Build: deploy.BuildArtifact})
	if err == nil {
		t.Fatal("want an error when --build=artifact has no artifact directory")
	}
	message := err.Error()
	if !strings.Contains(message, "--artifact-dir") || !strings.Contains(message, "deploy.artifact_dir") {
		t.Fatalf("the error must name both ways to supply an artifact, got %q", message)
	}
}

func TestResolveOptionsRejectsAnUnknownBuildMode(t *testing.T) {
	cfg := writeDeployConfig(t, "")
	if _, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{Build: "bogus"}); err == nil {
		t.Fatal("want an error for an unknown --build value")
	}
}

func TestDeployArtifactDirConfigurationReachesOptions(t *testing.T) {
	cfg := writeDeployConfig(t, "deploy:\n  artifact_dir: artifacts")
	opts, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if opts.ArtifactDir != "artifacts" {
		t.Fatalf("artifact dir = %q, want artifacts", opts.ArtifactDir)
	}
}

// `--lock=false` is the only spelling pflag accepts for turning the lock off, so
// it must actually reach the option the executor reads. It was once stored in a
// field nothing read, which made the flag a no-op.
func TestDisablingTheLockReachesTheResolvedOptions(t *testing.T) {
	dir := t.TempDir()
	cfg := deployRemoteConfig(t, dir, "git@example.com:acme/shop.git")

	disabled := false
	opts, err := deploy.ResolveOptionsForTest(cfg, "local", deploy.Overrides{Lock: &disabled})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !opts.SkipLock {
		t.Fatal("--lock=false must reach Options.SkipLock")
	}

	enabled := true
	opts, err = deploy.ResolveOptionsForTest(cfg, "local", deploy.Overrides{Lock: &enabled})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if opts.SkipLock {
		t.Fatal("--lock=true must keep locking on")
	}
}

// `deploy.lock_stale_after` is presented as configurable in spec 12.1; the
// threshold was a constant in the command layer, so a deploy that legitimately
// runs longer than two hours could not say so.
func TestLockStaleAfterIsConfiguration(t *testing.T) {
	dir := t.TempDir()
	cfg := deployRemoteConfig(t, dir, "git@example.com:acme/shop.git")

	opts, err := deploy.ResolveOptionsForTest(cfg, "local", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if opts.LockStaleAfter != deploy.DefaultLockStaleAfter {
		t.Fatalf("default lock_stale_after = %s, want %s", opts.LockStaleAfter, deploy.DefaultLockStaleAfter)
	}

	writeFile(t, filepath.Join(dir, ".govard.local.yml"), `
deploy:
  lock_stale_after: 30m
remotes:
  local:
    local: true
    path: /srv/public_html
    deploy_path: /srv
    branch: main
`)
	loaded, _, err := engine.LoadConfigFromDir(dir, false)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	opts, err = deploy.ResolveOptionsForTest(loaded, "local", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if opts.LockStaleAfter != 30*time.Minute {
		t.Fatalf("lock_stale_after = %s, want 30m", opts.LockStaleAfter)
	}

	writeFile(t, filepath.Join(dir, ".govard.local.yml"), `
deploy:
  lock_stale_after: not-a-duration
remotes:
  local:
    local: true
    path: /srv/public_html
    deploy_path: /srv
    branch: main
`)
	loaded, _, err = engine.LoadConfigFromDir(dir, false)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := deploy.ResolveOptionsForTest(loaded, "local", deploy.Overrides{}); !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("a malformed duration must be a configuration error, got %v", err)
	}
}

// Spec 5.2: the recipe validates `deploy.settings` and reports an unknown or
// invalid key as a configuration error. Without it a typo was silently ignored:
// `static_content_locale: en_US` shipped content in every locale the recipe
// happened to default to.
func TestValidateSettingsRefusesWhatTheRecipeDoesNotKnow(t *testing.T) {
	recipe := magento2.DeployRecipe()

	err := deploy.ValidateSettings(recipe, map[string]any{"static_content_locale": "en_US"})
	if !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
	}
	if !strings.Contains(err.Error(), "static_content_locale") {
		t.Fatalf("the refusal must name the key, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "static_content_locales") {
		t.Fatalf("the refusal must suggest the near miss, got %q", err.Error())
	}

	// A key the engine reads is known to every recipe, including the neutral one.
	if err := deploy.ValidateSettings(deploy.DefaultRecipe(), map[string]any{"shared_files": []string{"app/etc/env.php"}}); err != nil {
		t.Fatalf("an engine setting must be known: %v", err)
	}
	if err := deploy.ValidateSettings(deploy.DefaultRecipe(), map[string]any{"mage_mode": "production"}); !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("a framework setting must be unknown to the neutral recipe, got %v", err)
	}
}

func TestValidateSettingsChecksTheShape(t *testing.T) {
	recipe := magento2.DeployRecipe()

	valid := map[string]any{
		"shared_files":           []string{"app/etc/env.php"},
		"shared_dirs":            []any{"var/log", "pub/media"},
		"writable_mode":          "chmod",
		"static_jobs":            4,
		"worker_control":         true,
		"mage_mode":              "production",
		"magento_themes":         []string{"Magento/luma"},
		"static_content_locales": []any{"en_US", "fr_FR"},
		"content_version":        "",
		// The shared-path readers accept a bare string as well as a list, so
		// validation must not refuse what works.
		"sync_paths": "vendor",
	}
	if err := deploy.ValidateSettings(recipe, valid); err != nil {
		t.Fatalf("every declared shape must validate: %v", err)
	}

	// Each of these is silently wrong today: a number where the engine asserts a
	// string is read as empty, "four" is handed to `-j`, "yes" never equals the
	// "true" the recipe compares against, and a number where the theme list
	// belongs deploys every theme instead of the intended one.
	invalid := map[string]any{
		"mage_mode":      42,
		"static_jobs":    "four",
		"worker_control": "yes",
		"magento_themes": 42,
		"shared_dirs":    map[string]any{"var/log": true},
	}
	for key, value := range invalid {
		err := deploy.ValidateSettings(recipe, map[string]any{key: value})
		if !errors.Is(err, deploy.ErrInvalidConfiguration) {
			t.Errorf("deploy.settings.%s = %#v must be refused, got %v", key, value, err)
			continue
		}
		if !strings.Contains(err.Error(), key) {
			t.Errorf("the refusal for %s must name it, got %q", key, err.Error())
		}
	}
}

// A key a recipe knows about and does not implement is declared as unsupported,
// so a project setting it gets "not implemented" rather than a silent no-op.
func TestValidateSettingsNamesAnUnimplementedKey(t *testing.T) {
	recipe := deploy.RecipeForTest("stub", nil)
	recipe.Settings = []deploy.Setting{
		{Key: "not_yet", Kind: deploy.SettingUnsupported, Title: "arrives in a later release"},
	}
	err := deploy.ValidateSettings(recipe, map[string]any{"not_yet": true})
	if !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("the refusal must say the key is unimplemented, got %q", err.Error())
	}
}

// A recipe that declares the same deploy.settings key twice was undetectable: the
// last declaration silently wins, so validation could read one shape and `plan`
// print another, and only the Magento recipe had a test for it. Every path that
// builds a plan now refuses it, which is every command.
func TestBuildPlanRefusesARecipeThatDeclaresASettingTwice(t *testing.T) {
	recipe := deploy.RecipeForTest("duplicate", []deploy.Task{
		{ID: deploy.TaskCheck, Stage: deploy.StagePrepare, Command: "true"},
	})
	recipe.Settings = []deploy.Setting{
		{Key: "php_bin", Kind: deploy.SettingString, Title: "the interpreter"},
		{Key: "php_bin", Kind: deploy.SettingInt, Title: "the interpreter, again"},
	}

	_, err := deploy.BuildPlanForTest(recipe, nil, "staging")
	if err == nil {
		t.Fatal("a recipe that declares php_bin twice must be refused")
	}
	if !strings.Contains(err.Error(), "php_bin") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("the refusal must name the key and the recipe, got %v", err)
	}

	// The shipped recipe is clean, so the refusal does not fire on it.
	if _, err := deploy.BuildPlanForTest(magento2.DeployRecipe(), nil, "staging"); err != nil {
		t.Fatalf("the Magento recipe must build a plan: %v", err)
	}
}

// `split_static_deployment` is implemented now, so it is validated as the boolean
// it is rather than refused as unknown.
func TestSplitStaticDeploymentIsADeclaredBoolean(t *testing.T) {
	recipe := magento2.DeployRecipe()
	if !recipe.SettingsDeclare("split_static_deployment") {
		t.Fatal("the recipe reads the setting, so it must declare it")
	}
	if err := deploy.ValidateSettings(recipe, map[string]any{"split_static_deployment": true}); err != nil {
		t.Fatalf("a boolean must validate: %v", err)
	}
	if err := deploy.ValidateSettings(recipe, map[string]any{"split_static_deployment": "yes"}); !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("'yes' is not a boolean and must be refused, got %v", err)
	}
}

// The rendered argument lists the recipe itself produces must not be mistaken for
// project typos.
func TestValidateSettingsAcceptsRenderedArgumentLists(t *testing.T) {
	recipe := magento2.DeployRecipe()
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Settings: map[string]any{"magento_themes": []string{"Magento/luma"}},
	})
	if err := deploy.ValidateSettings(recipe, options.Settings); err != nil {
		t.Fatalf("the settings the engine renders must validate: %v", err)
	}
}
