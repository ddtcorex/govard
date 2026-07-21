package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
)

// allFrameworkNames mirrors the keys of engine.FrameworkConfigs, excluding
// "custom" (a user-defined escape hatch, not a real framework identity).
var allFrameworkNames = []string{
	"magento2", "magento1", "openmage", "mageos",
	"laravel", "symfony", "wordpress", "drupal",
	"nextjs", "emdash", "shopware", "cakephp", "prestashop",
}

// TestAllFrameworkNamesMatchesRegistry ensures that allFrameworkNames stays in
// sync with the actual keys in engine.FrameworkConfigs. If a 13th framework is
// added to engine.FrameworkConfigs in the future, this test will fail with a
// clear message, preventing silent snapshot-coverage gaps.
func TestAllFrameworkNamesMatchesRegistry(t *testing.T) {
	got := make(map[string]bool, len(allFrameworkNames))
	for _, name := range allFrameworkNames {
		got[name] = true
	}

	want := make(map[string]bool)
	for name := range engine.FrameworkConfigs {
		if name == "custom" {
			continue
		}
		want[name] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("engine.FrameworkConfigs has framework %q but allFrameworkNames is missing it - add it so this framework gets snapshot coverage", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("allFrameworkNames has framework %q but engine.FrameworkConfigs does not - remove it or check for a typo", name)
		}
	}
}

// compareOrUpdateGolden compares actual against the committed golden file at
// goldenPath. Set UPDATE_GOLDEN=1 to (re)write the golden file instead of
// comparing - the standard Go golden-file record/verify pattern.
func compareOrUpdateGolden(t *testing.T, goldenPath string, actual []byte) {
	t.Helper()

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("failed to create golden dir for %s: %v", goldenPath, err)
		}
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
		}
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file %s not found - run with UPDATE_GOLDEN=1 to create it: %v", goldenPath, err)
	}

	if string(expected) != string(actual) {
		t.Errorf("snapshot mismatch for %s\n--- expected ---\n%s\n--- actual ---\n%s", goldenPath, expected, actual)
	}
}

// normalizeSnapshotPaths replaces the two absolute-path roots that vary
// between test runs (the per-test temp project dir, and GOVARD_HOME_DIR)
// with stable placeholders, so golden files are deterministic.
func normalizeSnapshotPaths(content []byte, projectDir string, govardHome string) []byte {
	s := string(content)
	s = strings.ReplaceAll(s, projectDir, "<PROJECT_DIR>")
	s = strings.ReplaceAll(s, govardHome, "<GOVARD_HOME>")
	return []byte(s)
}

func testDataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to resolve test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata", "framework_snapshots")
}

func TestFrameworkSnapshotBlueprintRendering(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "internal", "blueprints", "files")
	goldenRoot := testDataDir(t)

	for _, framework := range allFrameworkNames {
		framework := framework
		t.Run(framework, func(t *testing.T) {
			tempDir := t.TempDir()
			fakeHome := filepath.Join(tempDir, "fake-home")
			if err := os.MkdirAll(fakeHome, 0o755); err != nil {
				t.Fatalf("failed to create fake home dir: %v", err)
			}
			t.Setenv("HOME", fakeHome)
			t.Setenv("SSH_AUTH_SOCK", "")

			destBlueprintsDir := filepath.Join(tempDir, "blueprints")
			if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
				t.Fatalf("failed to copy blueprints: %v", err)
			}

			projectName := "snapshot-" + framework
			config := engine.Config{
				ProjectName: projectName,
				Framework:   framework,
				Domain:      projectName + ".test",
				Stack: engine.Stack{
					UserID:  1000,
					GroupID: 1000,
				},
			}
			engine.NormalizeConfig(&config, tempDir)

			if err := engine.RenderBlueprint(tempDir, config); err != nil {
				t.Fatalf("RenderBlueprint failed for %s: %v", framework, err)
			}

			composePath := engine.ComposeFilePath(tempDir, projectName)
			composeContent, err := os.ReadFile(composePath)
			if err != nil {
				t.Fatalf("failed to read rendered compose file for %s: %v", framework, err)
			}
			goldenComposePath := filepath.Join(goldenRoot, framework, "compose.yml")
			compareOrUpdateGolden(t, goldenComposePath, normalizeSnapshotPaths(composeContent, tempDir, engine.GovardHomeDir()))

			nginxPath := filepath.Join(engine.GovardHomeDir(), "nginx", projectName, "default.conf")
			nginxContent := []byte("<NO NGINX CONFIG RENDERED>")
			if data, err := os.ReadFile(nginxPath); err == nil {
				nginxContent = data
			}
			goldenNginxPath := filepath.Join(goldenRoot, framework, "nginx.conf")
			compareOrUpdateGolden(t, goldenNginxPath, normalizeSnapshotPaths(nginxContent, tempDir, engine.GovardHomeDir()))
		})
	}
}

// freshCommandsFor calls each framework's bootstrap constructor directly,
// bypassing the production dispatcher (internal/engine/bootstrap/dispatcher.go's
// Run). This means it snapshots what each bootstrapper's FreshCommands()
// produces, but does NOT catch a dispatcher-routing regression (e.g. one
// framework's name accidentally mapped to the wrong bootstrapper) - Run's
// signature only returns an error today, with no way to observe which
// bootstrapper it actually resolved to. Known, accepted limitation.
func freshCommandsFor(framework string) []string {
	opts := bootstrap.Options{}
	switch framework {
	case "magento2":
		return bootstrap.Magento2FreshCommands(opts)
	case "mageos":
		return bootstrap.MageOSFreshCommands(opts)
	case "magento1":
		return bootstrap.NewMagento1Bootstrap(opts).FreshCommands()
	case "openmage":
		return bootstrap.NewOpenMageBootstrap(opts).FreshCommands()
	case "laravel":
		return bootstrap.NewLaravelBootstrap(opts).FreshCommands()
	case "symfony":
		return bootstrap.NewSymfonyBootstrap(opts).FreshCommands()
	case "drupal":
		return bootstrap.NewDrupalBootstrap(opts).FreshCommands()
	case "wordpress":
		return bootstrap.NewWordPressBootstrap(opts).FreshCommands()
	case "nextjs":
		return bootstrap.NewNextJSBootstrap(opts).FreshCommands()
	case "emdash":
		return bootstrap.NewEmdashBootstrap(opts).FreshCommands()
	case "shopware":
		return bootstrap.NewShopwareBootstrap(opts).FreshCommands()
	case "cakephp":
		return bootstrap.NewCakePHPBootstrap(opts).FreshCommands()
	case "prestashop":
		return bootstrap.NewPrestaShopBootstrap(opts).FreshCommands()
	default:
		return nil
	}
}

func TestFrameworkSnapshotBootstrapFreshCommands(t *testing.T) {
	goldenRoot := testDataDir(t)

	for _, framework := range allFrameworkNames {
		framework := framework
		t.Run(framework, func(t *testing.T) {
			cmds := freshCommandsFor(framework)
			data, err := json.MarshalIndent(cmds, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal fresh commands for %s: %v", framework, err)
			}

			goldenPath := filepath.Join(goldenRoot, framework, "bootstrap_fresh_commands.json")
			compareOrUpdateGolden(t, goldenPath, data)
		})
	}
}

func TestFrameworkSnapshotConfigAndProfile(t *testing.T) {
	goldenRoot := testDataDir(t)

	type snapshot struct {
		FrameworkConfig engine.FrameworkConfig      `json:"framework_config"`
		RuntimeProfile  engine.RuntimeProfileResult `json:"runtime_profile"`
	}

	for _, framework := range allFrameworkNames {
		framework := framework
		t.Run(framework, func(t *testing.T) {
			fwConfig, ok := engine.GetFrameworkConfig(framework)
			if !ok {
				t.Fatalf("GetFrameworkConfig(%q) returned ok=false", framework)
			}
			profileResult, err := engine.ResolveRuntimeProfile(framework, "")
			if err != nil {
				t.Fatalf("ResolveRuntimeProfile(%q, \"\") failed: %v", framework, err)
			}

			snap := snapshot{FrameworkConfig: fwConfig, RuntimeProfile: profileResult}
			data, err := json.MarshalIndent(snap, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal snapshot for %s: %v", framework, err)
			}

			goldenPath := filepath.Join(goldenRoot, framework, "config_profile.json")
			compareOrUpdateGolden(t, goldenPath, data)
		})
	}
}

func TestFrameworkSnapshotManifestAndDBCredentials(t *testing.T) {
	goldenRoot := testDataDir(t)

	type snapshot struct {
		SyncNoiseExcludes      []string `json:"sync_noise_excludes"`
		MediaExcludesAll       []string `json:"media_excludes_all"`
		MediaExcludesOptimized []string `json:"media_excludes_optimized"`
		MediaExcludesMinimal   []string `json:"media_excludes_minimal"`
		DBHost                 string   `json:"db_host"`
		DBPort                 int      `json:"db_port"`
		DBUsername             string   `json:"db_username"`
		DBDatabase             string   `json:"db_database"`
		DBTablePrefix          string   `json:"db_table_prefix"`
	}

	for _, framework := range allFrameworkNames {
		framework := framework
		t.Run(framework, func(t *testing.T) {
			creds := cmd.DefaultDBCredentialsForFrameworkForTest(framework)

			snap := snapshot{
				SyncNoiseExcludes:      engine.GetFrameworkSyncNoiseExcludes(framework),
				MediaExcludesAll:       engine.GetFrameworkMediaExcludes(framework, engine.FrameworkMediaModeAll),
				MediaExcludesOptimized: engine.GetFrameworkMediaExcludes(framework, engine.FrameworkMediaModeOptimized),
				MediaExcludesMinimal:   engine.GetFrameworkMediaExcludes(framework, engine.FrameworkMediaModeMinimal),
				DBHost:                 creds.Host,
				DBPort:                 creds.Port,
				DBUsername:             creds.Username,
				DBDatabase:             creds.Database,
				DBTablePrefix:          creds.TablePrefix,
			}
			// Password intentionally omitted from the snapshot per CLAUDE.md
			// ("Never log secrets, tokens, private keys, or DB passwords") -
			// default local dev passwords are low-sensitivity but there is no
			// reason to commit them to a golden fixture file either.

			data, err := json.MarshalIndent(snap, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal snapshot for %s: %v", framework, err)
			}

			goldenPath := filepath.Join(goldenRoot, framework, "manifest_db.json")
			compareOrUpdateGolden(t, goldenPath, data)
		})
	}
}
