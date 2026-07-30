package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestSyncCommandFromAndToAliasesAffectPlanEndpoints(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--from", "local", "--to", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Source:      local") {
		t.Fatalf("expected source from --from alias, got: %s", out)
	}
	if !strings.Contains(out, "Destination: dev") {
		t.Fatalf("expected destination from --to alias, got: %s", out)
	}
}

func TestSyncCommandLegacyEnvironmentAliasStillResolvesSource(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--environment", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Source:      dev") {
		t.Fatalf("expected source from legacy --environment alias, got: %s", out)
	}
}

func TestSyncCommandSourceWinsOverLegacyEnvironmentAlias(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--environment", "dev", "--source", "staging"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Source:      staging") {
		t.Fatalf("expected --source to take precedence over --environment, got: %s", out)
	}
	if strings.Contains(out, "Source:      dev") {
		t.Fatalf("did not expect legacy --environment to override --source, got: %s", out)
	}
}

func TestSyncCommandBareMediaFlagDoesNotConsumeEnvironmentFlag(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--media", "-e", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Source:      dev") {
		t.Fatalf("expected bare --media to keep -e bound to dev, got: %s", out)
	}
	if !strings.Contains(out, "Scopes:      media (optimized)") {
		t.Fatalf("expected media scope when bare --media is used, got: %s", out)
	}
}

func TestSyncCommandExplicitMediaModeStillParsesWithSpaceSeparatedValue(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--media", "all", "-e", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Source:      dev") {
		t.Fatalf("expected source to remain dev when --media all is used, got: %s", out)
	}
	if !strings.Contains(out, "Scopes:      media (all)") {
		t.Fatalf("expected explicit media mode all, got: %s", out)
	}
}

func TestSyncCommandWarnsWhenPathMissingForFileSync(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "-e", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No --path specified: the ENTIRE project root will be synced, not a subfolder.") {
		t.Fatalf("expected empty-path warning, got: %s", out)
	}
}

func TestSyncCommandWarnsHarderWhenPathMissingAndDeleteEnabled(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--delete", "-e", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No --path specified: the ENTIRE project root will be synced, and --delete is enabled") {
		t.Fatalf("expected delete-aware empty-path warning, got: %s", out)
	}
}

func TestSyncCommandNoEmptyPathWarningWhenPathGiven(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--path", "app/etc/config.php", "-e", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "No --path specified") {
		t.Fatalf("did not expect empty-path warning when --path is set, got: %s", out)
	}
}

func TestSyncCommandBareTrailingArgBecomesPath(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "-e", "dev", "var/log/"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Path Filter: var/log/") {
		t.Fatalf("expected bare trailing arg to become --path, got: %s", out)
	}
	if strings.Contains(out, "No --path specified") {
		t.Fatalf("did not expect empty-path warning once positional arg supplies a path, got: %s", out)
	}
}

func TestSyncCommandExplicitMediaModePositionalIsNotReusedAsPath(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--media", "all", "-e", "dev"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Path Filter: (none)") {
		t.Fatalf("expected 'all' consumed by --media, not reused as path, got: %s", out)
	}
}

func TestSyncCommandExplicitPathWinsOverPositionalArg(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	tempDir := t.TempDir()
	writeSyncAliasConfig(t, tempDir)
	chdirForTest(t, tempDir)

	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--plan", "--file", "--path", "app/etc/config.php", "-e", "dev", "ignored-positional"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Path Filter: app/etc/config.php") {
		t.Fatalf("expected explicit --path to win over positional arg, got: %s", out)
	}
}

func TestResetSyncFlagsForTestClearsStringArrayFlags(t *testing.T) {
	resetSyncFlagsForAliasTest(t)

	syncCmd := cmd.SyncCommand()
	if err := syncCmd.Flags().Set("include", "app/*"); err != nil {
		t.Fatalf("set include: %v", err)
	}
	if err := syncCmd.Flags().Set("exclude", "vendor/"); err != nil {
		t.Fatalf("set exclude: %v", err)
	}

	cmd.ResetSyncFlagsForTest()

	include, err := syncCmd.Flags().GetStringArray("include")
	if err != nil {
		t.Fatalf("get include: %v", err)
	}
	exclude, err := syncCmd.Flags().GetStringArray("exclude")
	if err != nil {
		t.Fatalf("get exclude: %v", err)
	}
	if len(include) != 0 {
		t.Fatalf("expected include flags to reset cleanly, got: %#v", include)
	}
	if len(exclude) != 0 {
		t.Fatalf("expected exclude flags to reset cleanly, got: %#v", exclude)
	}
}

func resetSyncFlagsForAliasTest(t *testing.T) {
	t.Helper()
	cmd.ResetSyncFlagsForTest()
	t.Cleanup(cmd.ResetSyncFlagsForTest)
}

func writeSyncAliasConfig(t *testing.T, tempDir string) {
	t.Helper()

	configPath := filepath.Join(tempDir, ".govard.yml")
	config := `project_name: sample-project
domain: sample.test
framework: laravel
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /srv/www/staging
  dev:
    host: dev.example.com
    user: deploy
    path: /srv/www/dev
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}
