package tests

import (
	"os"
	"path/filepath"
	"testing"

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
	if !opts.Verify || !opts.Lock {
		t.Fatalf("verify/lock must default to true, got verify=%v lock=%v", opts.Verify, opts.Lock)
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
