package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"govard/internal/frameworks/magento2"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/cmd"
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
    deploy:
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
	if remote.Deploy.Branch != "staging" || remote.Deploy.Publish != "symlink" {
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
    deploy:
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
	if cfg.Remotes["staging"].Deploy.Branch != "staging" || cfg.Remotes["staging"].Deploy.Publish != "symlink" {
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
    deploy:
      branch: staging
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
    deploy:
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
    deploy:
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

// `releases`, `status` and `unlock` only read the target, so they resolve
// read options: a remote with no branch configured is still readable, while a
// deploy to it is rightly refused.
func TestResolveReadOptionsNeedsNoSourceSelector(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
remotes:
  bare:
    host: bare.example.com
    user: deploy
    path: /home/deploy/public_html
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if _, err := deploy.ResolveOptionsForTest(cfg, "bare", deploy.Overrides{}); err == nil {
		t.Fatal("a deploy to a branchless remote must be refused")
	}
	opts, err := deploy.ResolveReadOptions(cfg, "bare", deploy.Overrides{})
	if err != nil {
		t.Fatalf("read resolve: %v", err)
	}
	if opts.SkipLock || opts.CommandTimeout <= 0 || opts.LockStaleAfter <= 0 {
		t.Fatalf("read options must keep the lock and timeout defaults, got %+v", opts)
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
    deploy:
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
    deploy:
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

	_, err := deploy.BuildPlanForTest(recipe, nil)
	if err == nil {
		t.Fatal("a recipe that declares php_bin twice must be refused")
	}
	if !strings.Contains(err.Error(), "php_bin") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("the refusal must name the key and the recipe, got %v", err)
	}

	// The shipped recipe is clean, so the refusal does not fire on it.
	if _, err := deploy.BuildPlanForTest(magento2.DeployRecipe(), nil); err != nil {
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

func TestDeployBlockTopologyKeysAreTrimmed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  repository: "  git@example.com:app/shop.git  "
  branch: "  master  "
  publish: " SYMLINK "
  deploy_path: "  /home/deploy/.deployer  "
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      branch: staging
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Deploy.Repository != "git@example.com:app/shop.git" {
		t.Fatalf("repository = %q, want trimmed", cfg.Deploy.Repository)
	}
	if cfg.Deploy.Branch != "master" {
		t.Fatalf("branch = %q, want trimmed", cfg.Deploy.Branch)
	}
	if cfg.Deploy.Publish != "symlink" {
		t.Fatalf("publish = %q, want trimmed and lowercased", cfg.Deploy.Publish)
	}
	if cfg.Deploy.DeployPath != "/home/deploy/.deployer" {
		t.Fatalf("deploy_path = %q, want trimmed", cfg.Deploy.DeployPath)
	}
}

func TestResolveOptionsFallsBackToDeployBlockTopology(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  repository: git@example.com:app/shop.git
  publish: symlink
  keep_releases: 5
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      repository: git@example.com:app/staging-fork.git
      branch: staging
  production:
    host: prod.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      branch: master
      repository: git@example.com:app/shop-explicit.git
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	staging, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve staging: %v", err)
	}
	if staging.Repository != "git@example.com:app/staging-fork.git" {
		t.Fatalf("repository = %q, want the remote deploy override", staging.Repository)
	}
	if staging.Publish != "symlink" {
		t.Fatalf("publish = %q, want the project default", staging.Publish)
	}

	production, err := deploy.ResolveOptionsForTest(cfg, "production", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve production: %v", err)
	}
	if production.Repository != "git@example.com:app/shop-explicit.git" {
		t.Fatalf("repository = %q, want the remote deploy override", production.Repository)
	}
	if production.Branch != "master" {
		t.Fatalf("branch = %q, want master", production.Branch)
	}
}

func TestDeployPathFallsBackToDeployBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  deploy_path: /home/deploy/.deployer
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      branch: staging
  production:
    host: prod.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      branch: master
      deploy_path: /srv/prod/.deployer
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for remote, want := range map[string]string{
		"staging":    "/home/deploy/.deployer",
		"production": "/srv/prod/.deployer",
	} {
		opts, err := deploy.ResolveOptionsForTest(cfg, remote, deploy.Overrides{})
		if err != nil {
			t.Fatalf("resolve %s: %v", remote, err)
		}
		host, err := deploy.HostForConfigForTest(cfg, remote, opts)
		if err != nil {
			t.Fatalf("host %s: %v", remote, err)
		}
		if host.DeployPath != want {
			t.Fatalf("%s deploy_path = %q, want %q", remote, host.DeployPath, want)
		}
	}
}

func TestDeployTopologyDefaultsPreserveTildePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  deploy_path: ~/.deployer
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: ~/public_html
    deploy:
      branch: staging
  production:
    host: prod.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      branch: master
      deploy_path: ~/.prod-deployer
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for remote, want := range map[string]string{
		"staging":    "~/.deployer",
		"production": "~/.prod-deployer",
	} {
		opts, err := deploy.ResolveOptionsForTest(cfg, remote, deploy.Overrides{})
		if err != nil {
			t.Fatalf("resolve %s: %v", remote, err)
		}
		host, err := deploy.HostForConfigForTest(cfg, remote, opts)
		if err != nil {
			t.Fatalf("host %s: %v", remote, err)
		}
		if host.DeployPath != want {
			t.Fatalf("%s deploy_path = %q, want byte-identical %q", remote, host.DeployPath, want)
		}
	}
	if cfg.Remotes["staging"].Path != "~/public_html" {
		t.Fatalf("remote path = %q, want untouched tilde form", cfg.Remotes["staging"].Path)
	}
}

func TestResolveOptionsRemoteDeployOverrideWinsForAllKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  repository: git@example.com:app/shop.git
  branch: master
  publish: symlink
  deploy_path: /home/deploy/.deployer
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      repository: git@example.com:app/staging-fork.git
      branch: staging
      publish: in_place
      deploy_path: /srv/staging/.deployer
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	opts, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if opts.Repository != "git@example.com:app/staging-fork.git" {
		t.Fatalf("repository = %q, want the remote deploy override", opts.Repository)
	}
	if opts.Branch != "staging" {
		t.Fatalf("branch = %q, want the remote deploy override", opts.Branch)
	}
	if opts.Publish != "in_place" {
		t.Fatalf("publish = %q, want the remote deploy override", opts.Publish)
	}
	host, err := deploy.HostForConfigForTest(cfg, "staging", opts)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	if host.DeployPath != "/srv/staging/.deployer" {
		t.Fatalf("deploy_path = %q, want the remote deploy override", host.DeployPath)
	}
}

func TestResolveOptionsRejectsBadProjectPublish(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  publish: bogus
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      branch: staging
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	_, err = deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{})
	if err == nil {
		t.Fatalf("expected an unsupported-strategy error, got nil")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error must name the value, got %q", err.Error())
	}
}

func TestFlagBranchBeatsProjectDefaultBranch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  branch: master
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
	def, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if def.Branch != "master" {
		t.Fatalf("branch = %q, want the project default", def.Branch)
	}
	flagged, err := deploy.ResolveOptionsForTest(cfg, "staging", deploy.Overrides{Branch: "hotfix"})
	if err != nil {
		t.Fatalf("resolve with flag: %v", err)
	}
	if flagged.Branch != "hotfix" {
		t.Fatalf("branch = %q, want the flag value", flagged.Branch)
	}
}

func TestPlanJSONBranchComesFromProjectDefault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  branch: master
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
`)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	out := &bytes.Buffer{}
	command := cmd.RootCommandForTest()
	command.SetArgs([]string{"deploy", "plan", "staging", "--json", "--build", "server"})
	command.SetOut(out)
	command.SetErr(io.Discard)
	t.Cleanup(func() {
		flags := cmd.DeployPlanCommand().Flags()
		_ = flags.Set("build", "auto")
		_ = flags.Set("json", "false")
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("deploy plan --json: %v\n%s", err, out.String())
	}
	var document struct {
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, out.String())
	}
	if document.Branch != "master" {
		t.Fatalf("plan branch = %q, want the project default", document.Branch)
	}
}

func TestFlatDeployTopologyKeysRejected(t *testing.T) {
	for _, key := range []string{"branch", "repository", "publish", "deploy_path"} {
		t.Run(key, func(t *testing.T) {
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
    `+key+`: some-value
`)
			_, _, err := engine.LoadConfigFromDir(root, true)
			if err == nil {
				t.Fatalf("load config with flat remotes.staging.%s: want error, got nil", key)
			}
			if !errors.Is(err, engine.ErrRemovedRemoteDeployKey) {
				t.Fatalf("error = %v, want it wrapped in ErrRemovedRemoteDeployKey for exit 4", err)
			}
			want := "remotes.staging.deploy." + key
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want it to name the nested replacement %q", err, want)
			}
		})
	}
}

// A list contributes one argument per entry, so an entry with a space in it
// cannot mean what its author meant. It was joined raw before, which is how
// `["--exclude-theme Magento/luma"]` became two words on the command line; now it
// renders as one quoted word, and the configuration is refused with the fix. The
// string form is the documented way to pass a multi-word value verbatim.
func TestValidateSettingsRefusesATwoWordListEntry(t *testing.T) {
	err := deploy.ValidateSettings(magento2.DeployRecipe(), map[string]any{
		"static_deploy_options": []string{"--exclude-theme Magento/luma"},
	})
	if !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
	}
	for _, want := range []string{"separate entries", "static_deploy_options"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must mention %q, got %q", want, err.Error())
		}
	}

	if err := deploy.ValidateSettings(magento2.DeployRecipe(), map[string]any{
		"static_deploy_options": "--exclude-theme Magento/luma",
	}); err != nil {
		t.Fatalf("the string form passes a multi-word value verbatim: %v", err)
	}

	// The normal shape keeps working, including a map key with a slash.
	if err := deploy.ValidateSettings(magento2.DeployRecipe(), map[string]any{
		"magento_themes": map[string][]string{"Magento/luma": {"en_US"}},
	}); err != nil {
		t.Fatalf("a theme map must validate: %v", err)
	}
}

// `php_bin` and `composer_bin` are command words, not one word: a documented
// wrapper (`php -d memory_limit=-1`, `docker exec app php`) has to reach the
// command as several words, and shell syntax in the value must stay inert rather
// than executable. One helper renders it for the recipe *and* for the probes, so
// `deploy:check` answers for the command the recipe will actually run.
func TestDeployVarsHonourAPHPWrapper(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	options := deploy.Options{Settings: map[string]any{"php_bin": "php -d memory_limit=-1"}}
	vars := cmd.DeployVarsForTest(host, options)

	expanded, err := vars.Expand("{{php_bin}} -v")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if expanded != "'php' '-d' 'memory_limit=-1' -v" {
		t.Fatalf("php_bin expanded to %q, want the words quoted one by one", expanded)
	}

	// The rendered words have to reach a real program as words. The wrapper here
	// is one whose contract a test can check without PHP installed: `/bin/sh -c`
	// followed by the words of the command.
	marker := filepath.Join(t.TempDir(), "wrapper-ran")
	shell := cmd.DeployVarsForTest(host, deploy.Options{Settings: map[string]any{"php_bin": "/bin/sh -c"}})
	command, err := shell.Expand("{{php_bin}} 'touch " + marker + "'")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("the rendered wrapper must execute: %v\n%s", err, command)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the wrapper did not run: %v", err)
	}
}

// A value that tries to add a command of its own is inert in the probes too: this
// is the check that used to interpolate `php_bin` raw.
func TestPHPProbeQuotesTheConfiguredBinary(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "probe-injected")
	origin, _ := seedGitRepo(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{
		Repository: origin,
		Branch:     "main",
		Publish:    deploy.PublishSymlink,
		Settings:   map[string]any{"php_bin": "php; touch " + marker},
	})
	_ = deploy.CoreCheck(context.Background(), sc)

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a php_bin value must not be able to run a command of its own")
	}
}
