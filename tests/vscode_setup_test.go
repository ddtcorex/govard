package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestNormalizeIntelephensePHPVersion(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{in: "8.2", want: "8.2.0"},
		{in: "8.2.5", want: "8.2.5"},
		{in: "", want: ""},
		{in: "none", want: ""},
		{in: "  8.1  ", want: "8.1.0"},
	} {
		if got := cmd.NormalizeIntelephensePHPVersionForTest(tt.in); got != tt.want {
			t.Errorf("NormalizeIntelephensePHPVersionForTest(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPhpstanDefaultOptionsMagento2(t *testing.T) {
	got := cmd.PhpstanDefaultOptionsForTest("magento2")
	want := []string{"--level=0", "--autoload-file=vendor/autoload.php", "app/code", "app/design"}
	if !slicesEqual(got, want) {
		t.Errorf("PhpstanDefaultOptionsForTest(magento2) = %v, want %v", got, want)
	}
}

func TestPhpstanDefaultOptionsNonMagento(t *testing.T) {
	got := cmd.PhpstanDefaultOptionsForTest("laravel")
	want := []string{"--level=0", "--autoload-file=vendor/autoload.php", "app", "src"}
	if !slicesEqual(got, want) {
		t.Errorf("PhpstanDefaultOptionsForTest(laravel) = %v, want %v", got, want)
	}
}

func TestHasPHPStanConfig(t *testing.T) {
	root := t.TempDir()
	if cmd.HasPHPStanConfigForTest(root) {
		t.Error("expected no config to be found in an empty directory")
	}

	for _, name := range []string{"phpstan.neon", "phpstan.neon.dist", "phpstan.dist.neon"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, name), []byte("parameters: {}\n"), 0o644); err != nil {
				t.Fatalf("seed %s: %v", name, err)
			}
			if !cmd.HasPHPStanConfigForTest(dir) {
				t.Errorf("expected %s to be recognized as an existing PHPStan config", name)
			}
		})
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestExtensionInstalledInDirs(t *testing.T) {
	dir := t.TempDir()
	manifest := `[
		{"identifier": {"id": "shevaua.phpcs"}, "version": "1.0.8"},
		{"identifier": {"id": "Sanderronde.phpstan-vscode"}, "version": "4.0.17"}
	]`
	if err := os.WriteFile(filepath.Join(dir, "extensions.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write fake extensions.json: %v", err)
	}
	// A same-named folder on disk without a manifest entry must NOT count as
	// installed — this is exactly the false positive that let `setup` wire
	// up settings for extensions VSCode never actually loaded.
	if err := os.MkdirAll(filepath.Join(dir, "junstyle.php-cs-fixer-0.3.21"), 0o755); err != nil {
		t.Fatalf("mkdir orphaned extension folder: %v", err)
	}

	if !cmd.ExtensionInstalledInDirsForTest("shevaua.phpcs", []string{dir}) {
		t.Error("expected shevaua.phpcs to be detected as installed")
	}
	if !cmd.ExtensionInstalledInDirsForTest("sanderronde.phpstan-vscode", []string{dir}) {
		t.Error("expected extension ID matching to be case-insensitive")
	}
	if cmd.ExtensionInstalledInDirsForTest("junstyle.php-cs-fixer", []string{dir}) {
		t.Error("expected an orphaned folder with no extensions.json entry to be reported as not installed")
	}
	if cmd.ExtensionInstalledInDirsForTest("xdebug.php-debug", []string{dir}) {
		t.Error("expected xdebug.php-debug to be reported as not installed")
	}
	if cmd.ExtensionInstalledInDirsForTest("shevaua.phpcs", []string{filepath.Join(dir, "does-not-exist")}) {
		t.Error("expected a nonexistent directory to be treated as no match, not an error")
	}
}

func TestDetectPHPCSStandard(t *testing.T) {
	for _, tt := range []struct {
		name     string
		composer string
		want     string
	}{
		{
			name:     "magento coding standard",
			composer: `{"require": {"magento/magento-coding-standard": "*"}}`,
			want:     "Magento2",
		},
		{
			name:     "wordpress coding standard in require-dev",
			composer: `{"require-dev": {"wp-coding-standards/wpcs": "^3.0"}}`,
			want:     "WordPress",
		},
		{
			name:     "no known package falls back to PSR12",
			composer: `{"require": {"squizlabs/php_codesniffer": "^3.7"}}`,
			want:     "PSR12",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(tt.composer), 0o644); err != nil {
				t.Fatalf("write composer.json: %v", err)
			}
			if got := cmd.DetectPHPCSStandardForTest(root); got != tt.want {
				t.Errorf("DetectPHPCSStandardForTest() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("missing composer.json falls back to PSR12", func(t *testing.T) {
		root := t.TempDir()
		if got := cmd.DetectPHPCSStandardForTest(root); got != "PSR12" {
			t.Errorf("DetectPHPCSStandardForTest() = %q, want PSR12", got)
		}
	})
}

func TestMergeJSONObjectFilePreservesUnrelatedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"editor.tabSize": 4, "phpstan.binPath": "old"}`), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := cmd.MergeJSONObjectFileForTest(path,
		map[string]interface{}{"php.validate.executablePath": "/wrapper"},
		[]string{"phpstan.binPath"},
	)
	if err != nil {
		t.Fatalf("MergeJSONObjectFileForTest: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	if obj["editor.tabSize"] != float64(4) {
		t.Errorf("expected unrelated key editor.tabSize to survive, got %v", obj["editor.tabSize"])
	}
	if _, exists := obj["phpstan.binPath"]; exists {
		t.Errorf("expected phpstan.binPath to be removed, still present: %v", obj["phpstan.binPath"])
	}
	if obj["php.validate.executablePath"] != "/wrapper" {
		t.Errorf("expected php.validate.executablePath to be set, got %v", obj["php.validate.executablePath"])
	}
}

func TestMergeJSONObjectFileCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")

	if err := cmd.MergeJSONObjectFileForTest(path, map[string]interface{}{"a": "b"}, nil); err != nil {
		t.Fatalf("MergeJSONObjectFileForTest on missing file: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to be created: %v", err)
	}
}

func TestMergeLaunchConfigReplacesExistingEntryAndKeepsOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch.json")
	seed := `{
		"version": "0.2.0",
		"configurations": [
			{"name": "Some Other Config", "type": "node", "request": "launch"},
			{"name": "Listen for Xdebug (Govard)", "type": "php", "request": "launch", "port": 9000}
		]
	}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := cmd.MergeLaunchConfigForTest(path, "/var/www/html"); err != nil {
		t.Fatalf("MergeLaunchConfigForTest: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	var launch struct {
		Configurations []map[string]interface{} `json:"configurations"`
	}
	if err := json.Unmarshal(data, &launch); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(launch.Configurations) != 2 {
		t.Fatalf("expected 2 configurations, got %d", len(launch.Configurations))
	}

	var xdebug map[string]interface{}
	for _, c := range launch.Configurations {
		if c["name"] == "Some Other Config" {
			continue
		}
		xdebug = c
	}
	if xdebug == nil {
		t.Fatal("expected the Govard Xdebug configuration to still be present")
	}
	if xdebug["port"] != float64(9003) {
		t.Errorf("expected port to be updated to 9003, got %v", xdebug["port"])
	}
}

func TestVSCodeIndexSettingsKeepVendorSearchable(t *testing.T) {
	for _, framework := range []string{"magento2", "mageos", "laravel", "symfony", "wordpress"} {
		t.Run(framework, func(t *testing.T) {
			set := cmd.VSCodeIndexSettingsForTest(framework)

			if v, ok := set["search.useIgnoreFiles"]; !ok || v != false {
				t.Errorf("search.useIgnoreFiles = %v, want false so a gitignored vendor/ stays searchable", v)
			}
			if v, ok := set["search.useParentIgnoreFiles"]; !ok || v != false {
				t.Errorf("search.useParentIgnoreFiles = %v, want false", v)
			}
			searchExclude, _ := set["search.exclude"].(map[string]interface{})
			for key := range searchExclude {
				if strings.Contains(key, "vendor") && !strings.Contains(key, "vendor/**/") {
					t.Errorf("search.exclude must not hide vendor/, found %q", key)
				}
			}
			watcher, _ := set["files.watcherExclude"].(map[string]interface{})
			if _, ok := watcher["**/vendor/**"]; ok {
				t.Error("files.watcherExclude must not drop vendor/: Intelephense needs it to see composer updates")
			}
			index, _ := set["intelephense.files.exclude"].([]string)
			if !containsString(index, "**/vendor/**/vendor/**") || !containsString(index, "**/vendor/**/{Tests,tests}/**") {
				t.Errorf("intelephense.files.exclude must keep Intelephense's own defaults, got %v", index)
			}
			for _, glob := range index {
				if glob == "**/vendor/**" {
					t.Error("intelephense.files.exclude must not exclude vendor/ itself")
				}
			}
		})
	}
}

func TestVSCodeIndexSettingsFrameworkNoise(t *testing.T) {
	for _, tt := range []struct {
		framework string
		search    string
		watcher   string
		index     string
	}{
		{"magento2", "**/var/cache", "**/var/**", "var/**"},
		{"mageos", "**/pub/static", "**/pub/static/**", "pub/static/**"},
		{"laravel", "**/storage/framework", "**/storage/framework/**", "storage/framework/**"},
		{"symfony", "**/var/cache", "**/var/cache/**", "var/cache/**"},
		{"wordpress", "**/wp-content/uploads", "**/wp-content/uploads/**", "wp-content/uploads/**"},
	} {
		t.Run(tt.framework, func(t *testing.T) {
			set := cmd.VSCodeIndexSettingsForTest(tt.framework)
			if _, ok := set["search.exclude"].(map[string]interface{})[tt.search]; !ok {
				t.Errorf("search.exclude missing %q: %v", tt.search, set["search.exclude"])
			}
			if _, ok := set["files.watcherExclude"].(map[string]interface{})[tt.watcher]; !ok {
				t.Errorf("files.watcherExclude missing %q: %v", tt.watcher, set["files.watcherExclude"])
			}
			if !containsString(set["intelephense.files.exclude"].([]string), "**/"+tt.index) {
				t.Errorf("intelephense.files.exclude missing **/%s: %v", tt.index, set["intelephense.files.exclude"])
			}
		})
	}
}

// Magento's generated/code holds the Factory and Interceptor classes that
// application code references constantly, so it must stay indexed even though
// it is noise in search results.
func TestVSCodeIndexSettingsMagentoKeepsGeneratedIndexed(t *testing.T) {
	set := cmd.VSCodeIndexSettingsForTest("magento2")
	if _, ok := set["search.exclude"].(map[string]interface{})["**/generated/code"]; !ok {
		t.Error("generated/code should be excluded from search")
	}
	for _, glob := range set["intelephense.files.exclude"].([]string) {
		if strings.Contains(glob, "generated") {
			t.Errorf("intelephense.files.exclude must keep generated/ indexed, found %q", glob)
		}
	}
}

func TestVSCodeIndexSettingsUnknownFrameworkStillSearchesVendor(t *testing.T) {
	set := cmd.VSCodeIndexSettingsForTest("no-such-framework")
	if set["search.useIgnoreFiles"] != false {
		t.Error("generic fallback must still disable search.useIgnoreFiles")
	}
}

func TestMergeJSONObjectFileMergesMapKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"search.exclude": {"**/custom": true, "**/var/cache": false}, "editor.tabSize": 2}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	set := map[string]interface{}{
		"search.exclude": map[string]interface{}{"**/var/cache": true, "**/pub/static": true},
	}
	if err := cmd.MergeJSONObjectFileMergingForTest(path, set, nil, []string{"search.exclude"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	exclude := got["search.exclude"].(map[string]interface{})
	if exclude["**/custom"] != true {
		t.Error("user's own search.exclude entry was dropped")
	}
	if exclude["**/var/cache"] != true || exclude["**/pub/static"] != true {
		t.Errorf("managed entries not applied: %v", exclude)
	}
	if got["editor.tabSize"] != float64(2) {
		t.Error("unrelated key was not preserved")
	}
}

func TestMergeJSONObjectFileUnionsListKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"intelephense.files.exclude": ["**/mine/**", "**/.git/**"]}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	set := map[string]interface{}{"intelephense.files.exclude": []string{"**/.git/**", "**/var/**"}}
	for i := 0; i < 2; i++ { // second run proves idempotence
		if err := cmd.MergeJSONObjectFileMergingForTest(path, set, nil, []string{"intelephense.files.exclude"}); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	var got map[string][]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"**/mine/**", "**/.git/**", "**/var/**"}
	if !slicesEqual(got["intelephense.files.exclude"], want) {
		t.Errorf("got %v, want %v", got["intelephense.files.exclude"], want)
	}
}

func TestPruneVendorExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{
		"search.exclude": {"**/vendor": true, "vendor/**": true, "/vendor/": true, "**/vendor/**": true, "**/var": true, "**/vendor/**/Test": true, "**/other/vendor": false},
		"files.exclude": {"**/vendor": true, "**/generated": true},
		"files.watcherExclude": {"**/vendor/**": true}
	}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := cmd.PruneVendorExcludesForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 5 {
		t.Errorf("removed = %v, want 5 entries (4 in search.exclude, 1 in files.exclude)", removed)
	}
	data, _ := os.ReadFile(path)
	var got map[string]map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	search := got["search.exclude"]
	for _, key := range []string{"**/vendor", "vendor/**", "/vendor/", "**/vendor/**"} {
		if _, ok := search[key]; ok {
			t.Errorf("search.exclude still hides vendor via %q", key)
		}
	}
	if search["**/var"] != true || search["**/vendor/**/Test"] != true {
		t.Errorf("unrelated search.exclude entries must survive: %v", search)
	}
	if _, ok := got["files.exclude"]["**/vendor"]; ok || got["files.exclude"]["**/generated"] != true {
		t.Errorf("files.exclude not pruned correctly: %v", got["files.exclude"])
	}
	if got["files.watcherExclude"]["**/vendor/**"] != true {
		t.Error("files.watcherExclude is the user's call and must not be touched")
	}
}

func TestPruneVendorExcludesMissingFile(t *testing.T) {
	removed, err := cmd.PruneVendorExcludesForTest(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || len(removed) != 0 {
		t.Errorf("missing file: removed=%v err=%v, want none", removed, err)
	}
}
