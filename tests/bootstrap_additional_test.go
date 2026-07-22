package tests

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	bootstrapengine "govard/internal/engine/bootstrap"

	"github.com/spf13/cobra"
)

func TestRunBootstrapComposerPrepareForTestRunsVendorCleanup(t *testing.T) {
	var gotConfig engine.Config
	var gotCommandLine string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		gotConfig = config
		gotCommandLine = commandLine
		return nil
	})()

	err := cmd.RunBootstrapComposerPrepareForTest(engine.Config{ProjectName: "sample-project"})
	if err != nil {
		t.Fatalf("RunBootstrapComposerPrepareForTest() error = %v", err)
	}
	if gotConfig.ProjectName != "sample-project" {
		t.Fatalf("project name = %q, want %q", gotConfig.ProjectName, "sample-project")
	}
	if gotCommandLine != "rm -rf vendor" {
		t.Fatalf("command line = %q, want %q", gotCommandLine, "rm -rf vendor")
	}
}

func TestBootstrapComposerDumpAutoloadForTestRunsSubcommandWhenComposerJSONExists(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}

	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := make([]string, len(args))
		copy(captured, args)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.BootstrapComposerDumpAutoloadForTest(&cobra.Command{}, tempDir)
	if err != nil {
		t.Fatalf("BootstrapComposerDumpAutoloadForTest() error = %v", err)
	}

	want := []string{"tool", "composer", "dump-autoload", "-n"}
	if len(calls) != 1 {
		t.Fatalf("expected one subcommand call, got %d", len(calls))
	}
	if !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("subcommand args = %#v, want %#v", calls[0], want)
	}
}

func TestBootstrapComposerDumpAutoloadForTestSkipsWhenComposerJSONMissing(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	calls := 0
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls++
		return nil
	})()

	err := cmd.BootstrapComposerDumpAutoloadForTest(&cobra.Command{}, tempDir)
	if err != nil {
		t.Fatalf("BootstrapComposerDumpAutoloadForTest() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no subcommand calls when composer.json is missing, got %d", calls)
	}
}

func TestBootstrapComposerDumpAutoloadForTestAllowsVendorFallbackOnError(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write composer.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "vendor"), 0o755); err != nil {
		t.Fatalf("mkdir vendor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "vendor", "autoload.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write vendor/autoload.php: %v", err)
	}

	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		return errors.New("composer failed")
	})()

	err := cmd.BootstrapComposerDumpAutoloadForTest(&cobra.Command{}, tempDir)
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}
}

func TestEnsureBootstrapAuthJSONForTestCreatesAuthFromCredentials(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	t.Setenv("HOME", tempDir)

	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("/vendor\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	err := cmd.EnsureBootstrapAuthJSONForTest(
		engine.Config{Framework: "magento2"},
		"public-key",
		"private-key",
		true,
	)
	if err != nil {
		t.Fatalf("EnsureBootstrapAuthJSONForTest() error = %v", err)
	}

	// The new logic saves to global ~/.composer/auth.json (host)
	authContent, err := os.ReadFile(filepath.Join(tempDir, ".composer", "auth.json"))
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	authText := string(authContent)
	if !strings.Contains(authText, `"username": "public-key"`) {
		t.Fatalf("auth.json missing username; content:\n%s", authText)
	}
	if !strings.Contains(authText, `"password": "private-key"`) {
		t.Fatalf("auth.json missing password; content:\n%s", authText)
	}

	gitignoreContent, err := os.ReadFile(filepath.Join(tempDir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if strings.Contains(string(gitignoreContent), "/auth.json") {
		t.Fatalf(".gitignore should NOT contain /auth.json entry anymore: %s", string(gitignoreContent))
	}
}

func TestRunBootstrapFreshCreateProjectForTestBuildsExpectedCommand(t *testing.T) {
	var gotCommandLine string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		gotCommandLine = commandLine
		return nil
	})()

	err := cmd.RunBootstrapFreshCreateProjectForTest(
		&cobra.Command{},
		engine.Config{ProjectName: "sample-project"},
		"magento/project-community-edition",
		"2.4.8",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFreshCreateProjectForTest() error = %v", err)
	}

	if !strings.Contains(gotCommandLine, "composer create-project -n --ignore-platform-reqs --repository-url=https://repo.magento.com 'magento/project-community-edition' /tmp/govard-create-project '2.4.8'") {
		t.Fatalf("unexpected create-project command: %s", gotCommandLine)
	}
	if !strings.Contains(gotCommandLine, "rm -rf /tmp/govard-create-project") {
		t.Fatalf("expected cleanup commands in shell line: %s", gotCommandLine)
	}
}

func TestBuildPHPContainerShellCommandForTestUsesRepoRootCDPrefix(t *testing.T) {
	got := cmd.BuildPHPContainerShellCommandForTest("composer install --no-interaction")
	want := "cd /var/www/html && composer install --no-interaction"
	if got != want {
		t.Fatalf("BuildPHPContainerShellCommandForTest() = %q, want %q", got, want)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestRejectsUnsupportedFramework(t *testing.T) {
	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{Framework: "custom"},
		"dev",
		"",
	)
	if err == nil {
		t.Fatal("expected unsupported framework error")
	}
	if !strings.Contains(err.Error(), "fresh install not supported for framework: custom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestNextJSUsesThrowawayContainer(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedProjectDir string
	var capturedCommand string
	defer cmd.SetNodeCreateProjectRunnerForTest(func(config engine.Config, projectDir string, commandLine string) error {
		capturedProjectDir = projectDir
		capturedCommand = commandLine
		stageDir := extractStageHostDir(t, commandLine)
		return os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"nextjs-app\"}\n"), 0o644)
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "nextjs",
			Domain:      "sample.test",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if capturedProjectDir != tempDir {
		t.Fatalf("expected runner to receive project dir %q, got %q", tempDir, capturedProjectDir)
	}
	if !strings.Contains(capturedCommand, "npx create-next-app@latest") {
		t.Fatalf("expected create-next-app invocation, got: %s", capturedCommand)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to land in the project dir: %v", err)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestWordPressDoesNotRestartEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	restoreDownloader := bootstrapengine.SetWordPressCoreDownloaderForTest(func(projectDir string) error {
		samplePath := filepath.Join(projectDir, "wp-config-sample.php")
		return os.WriteFile(samplePath, []byte("<?php\n"), 0o644)
	})
	defer restoreDownloader()

	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		return nil
	})()

	calls := make([][]string, 0, 2)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := append([]string{}, args...)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "wordpress",
			Domain:      "sample.test",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	want := [][]string{
		{"config", "auto"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("subcommand calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapHyvaInstallForTestRunsExpectedComposerCalls(t *testing.T) {
	calls := make([][]string, 0, 3)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := make([]string, len(args))
		copy(captured, args)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapHyvaInstallForTest(&cobra.Command{}, "token-123")
	if err != nil {
		t.Fatalf("RunBootstrapHyvaInstallForTest() error = %v", err)
	}

	want := [][]string{
		{"tool", "composer", "config", "http-basic.hyva-themes.repo.packagist.com", "token", "token-123"},
		{"tool", "composer", "config", "repositories.hyva-themes", "composer", "https://hyva-themes.repo.packagist.com/app-hyva-test-dv1dgx/"},
		{"tool", "composer", "require", "-n", "hyva-themes/magento2-default-theme"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("composer calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapMagentoSetupInstallForTestUsesElasticsearch7ForLegacyVersion(t *testing.T) {
	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := make([]string, len(args))
		copy(captured, args)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapMagentoSetupInstallForTest(
		&cobra.Command{},
		engine.Config{Framework: "magento2", Domain: "sample.test"},
		"staging",
		"2.4.7",
	)
	if err != nil {
		t.Fatalf("RunBootstrapMagentoSetupInstallForTest() error = %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected one setup call, got %d", len(calls))
	}
	joined := strings.Join(calls[0], " ")
	if !strings.Contains(joined, "--search-engine=elasticsearch7") {
		t.Fatalf("expected elasticsearch7 engine for legacy versions, args: %s", joined)
	}
	if strings.Contains(joined, "--search-engine=opensearch") {
		t.Fatalf("did not expect opensearch args for legacy version: %s", joined)
	}
}

func TestRunBootstrapMagentoSetupInstallForTestUsesOpenSearchForRecentVersion(t *testing.T) {
	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := make([]string, len(args))
		copy(captured, args)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapMagentoSetupInstallForTest(
		&cobra.Command{},
		engine.Config{Domain: "sample.test"},
		"staging",
		"2.4.8",
	)
	if err != nil {
		t.Fatalf("RunBootstrapMagentoSetupInstallForTest() error = %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected one setup call, got %d", len(calls))
	}
	joined := strings.Join(calls[0], " ")
	if !strings.Contains(joined, "--search-engine=opensearch") {
		t.Fatalf("expected opensearch args for 2.4.8+, got: %s", joined)
	}
}

func TestRunBootstrapSampleDataForTestRunsAllSteps(t *testing.T) {
	calls := make([][]string, 0, 4)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := make([]string, len(args))
		copy(captured, args)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapSampleDataForTest(&cobra.Command{})
	if err != nil {
		t.Fatalf("RunBootstrapSampleDataForTest() error = %v", err)
	}

	want := [][]string{
		{"tool", "magento", "sample:deploy"},
		{"tool", "magento", "setup:upgrade"},
		{"tool", "magento", "indexer:reindex"},
		{"tool", "magento", "cache:flush"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("sample data calls = %#v, want %#v", calls, want)
	}
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}
func TestRunBootstrapRemoteSyncsVendorWithTrailingSlash(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "magento2",
		Remotes: map[string]engine.RemoteConfig{
			"staging": {
				Host: "staging.example.com",
				Path: "/var/www/html",
			},
		},
	}

	opts := cmd.DefaultBootstrapRuntimeOptionsForTest()
	opts.Source = "staging"
	opts.ComposerInstall = true
	opts.AssumeYes = true

	// Mock composer.json existence
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	subcommandCalls := 0
	vendorSyncFound := false
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		subcommandCalls++
		// The call we are looking for is: govard sync --source staging --file --path vendor/ --yes
		if len(args) >= 6 && args[0] == "sync" && args[5] == "vendor/" {
			vendorSyncFound = true
		}

		// Simulate composer install failure to trigger vendor sync
		if len(args) >= 3 && args[0] == "tool" && args[1] == "composer" && args[2] == "install" {
			return errors.New("composer install failed")
		}
		return nil
	})()

	// Mock remote directory check
	defer cmd.SetBootstrapRemoteDirExistsForTest(func(remoteName string, remoteCfg engine.RemoteConfig, remotePath string) bool {
		return true
	})()

	err := cmd.RunBootstrapRemoteForTest(&cobra.Command{}, config, opts)
	if err != nil && !strings.Contains(err.Error(), "composer install failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	if !vendorSyncFound {
		t.Fatal("expected vendor sync with trailing slash 'vendor/', but it was not found in subcommand calls")
	}
}

func TestRunBootstrapRemoteSkipsComposerInstallWhenVendorSatisfiesLock(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	config := engine.Config{
		ProjectName: "sample-project",
		Framework:   "magento2",
	}

	opts := cmd.DefaultBootstrapRuntimeOptionsForTest()
	opts.ComposerInstall = true
	opts.AssumeYes = true
	// Avoid triggering the unrelated "remote is required" gate at the top of
	// runBootstrapRemote (requiresRemote), which is orthogonal to the
	// composer-install-skip behavior under test here.
	opts.DBImport = false
	opts.MediaSync = ""

	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "composer.lock"), []byte(`{
  "packages": [{"name": "psr/log", "version": "1.1.4"}],
  "packages-dev": []
}`), 0644); err != nil {
		t.Fatal(err)
	}
	installedDir := filepath.Join(tempDir, "vendor", "composer")
	if err := os.MkdirAll(installedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedDir, "installed.json"), []byte(`{
  "packages": [{"name": "psr/log", "version": "1.1.4"}]
}`), 0644); err != nil {
		t.Fatal(err)
	}

	composerInstallCalls := 0
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		if len(args) >= 3 && args[0] == "tool" && args[1] == "composer" && args[2] == "install" {
			composerInstallCalls++
		}
		return nil
	})()

	if err := cmd.RunBootstrapRemoteForTest(&cobra.Command{}, config, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if composerInstallCalls != 0 {
		t.Fatalf("expected composer install to be skipped, but it was called %d time(s)", composerInstallCalls)
	}
}
