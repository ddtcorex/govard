package tests

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/mageos"
)

// magentoSetupInstallHead and magentoSetupInstallTail are the literal
// setup:install arguments around the search block for the magento2 variant
// with admin email "admin@sample.test" and table prefix "pre_". They are
// written out by hand so the golden table below does not reuse the code
// under test.
var magentoSetupInstallHead = []string{
	"setup:install",
	"--backend-frontname=admin",
	"--db-host=db",
	"--db-name=magento",
	"--db-user=magento",
	"--db-password=magento",
	"--db-prefix=pre_",
}

var magentoSetupInstallTail = []string{
	"--admin-user=admin",
	"--admin-password=Admin12345678$",
	"--admin-firstname=Admin",
	"--admin-lastname=User",
	"--admin-email=admin@sample.test",
}

var magentoElasticsearch7Args = []string{
	"--search-engine=elasticsearch7",
	"--elasticsearch-host=elasticsearch",
	"--elasticsearch-port=9200",
	"--elasticsearch-index-prefix=magento2",
	"--elasticsearch-enable-auth=0",
	"--elasticsearch-timeout=15",
}

var magentoOpenSearchArgs = []string{
	"--search-engine=opensearch",
	"--opensearch-host=elasticsearch",
	"--opensearch-port=9200",
	"--opensearch-index-prefix=magento2",
	"--opensearch-enable-auth=0",
	"--opensearch-timeout=15",
}

func expectedMagentoSetupInstallArgs(search []string) []string {
	out := append([]string{}, magentoSetupInstallHead...)
	out = append(out, search...)
	return append(out, magentoSetupInstallTail...)
}

// TestSetupInstallArgsGoldenPerLine pins the full setup:install argument
// list for every Magento 2.4 line govard ships a profile for, so a change to
// BuildSetupInstallArgs that alters the output for any line fails here.
func TestSetupInstallArgsGoldenPerLine(t *testing.T) {
	cases := []struct {
		version string
		search  []string
	}{
		{"2.4.0", magentoElasticsearch7Args},
		{"2.4.1", magentoElasticsearch7Args},
		{"2.4.2", magentoElasticsearch7Args},
		{"2.4.3", magentoElasticsearch7Args},
		{"2.4.3-p2", magentoElasticsearch7Args},
		{"2.4.4", magentoElasticsearch7Args},
		{"2.4.4-p17", magentoElasticsearch7Args},
		{"2.4.5", magentoElasticsearch7Args},
		{"2.4.5-p16", magentoElasticsearch7Args},
		{"2.4.6", magentoElasticsearch7Args},
		{"2.4.6-p5", magentoElasticsearch7Args},
		{"2.4.6-p11", magentoElasticsearch7Args},
		{"2.4.7", magentoElasticsearch7Args},
		{"2.4.7-p3", magentoElasticsearch7Args},
		{"2.4.7-p7", magentoElasticsearch7Args},
		{"2.4.8", magentoOpenSearchArgs},
		{"2.4.8-p2", magentoOpenSearchArgs},
		{"2.4.9", magentoOpenSearchArgs},
		{"", magentoOpenSearchArgs},
	}
	for _, tc := range cases {
		got := magento2.BuildSetupInstallArgs(magento2.Variant, tc.version, "admin@sample.test", "pre_")
		want := expectedMagentoSetupInstallArgs(tc.search)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("version %q:\n got  %q\n want %q", tc.version, got, want)
		}
	}
}

// TestSetupInstallArgsGoldenForMageOS pins that Mage-OS keeps OpenSearch for
// every version, including ones that would select elasticsearch7 on magento2.
func TestSetupInstallArgsGoldenForMageOS(t *testing.T) {
	for _, version := range []string{"", "1.0.0", "2.4.7"} {
		got := magento2.BuildSetupInstallArgs(mageos.Variant, version, "admin@sample.test", "")
		joined := strings.Join(got, " ")
		if !strings.Contains(joined, "--search-engine=opensearch --opensearch-host=elasticsearch") {
			t.Errorf("mageos %q: expected OpenSearch args, got %q", version, joined)
		}
	}
}

const legacyFreshCreateProject = "set -e && rm -rf /tmp/govard-create-project && " +
	"composer create-project -n --ignore-platform-reqs --repository-url=https://repo.magento.com 'magento/project-community-edition' /tmp/govard-create-project '2.4.6' && " +
	"if command -v rsync >/dev/null 2>&1; then rsync -a /tmp/govard-create-project/ /var/www/html/; else cp -a /tmp/govard-create-project/. /var/www/html/; fi && " +
	"rm -rf /tmp/govard-create-project"

// TestFreshCreateProjectWithoutPHPVersionIsUnchanged pins that the
// create-project command is byte-identical to the pre-pin command when no
// runtime PHP version is known.
func TestFreshCreateProjectWithoutPHPVersionIsUnchanged(t *testing.T) {
	got := magento2.BuildFreshCreateProjectCommand(magento2.Variant, bootstrap.Options{
		MetaPackage: "magento/project-community-edition",
		Version:     "2.4.6",
	})
	if got != legacyFreshCreateProject {
		t.Fatalf("command changed without a PHP version:\n got  %s\n want %s", got, legacyFreshCreateProject)
	}
}

// TestFreshCreateProjectComposer1KeepsLegacyCommand pins that a Composer 1
// runtime keeps the pre-pin command: Composer 1 has no
// --ignore-platform-req=<name> option, and --ignore-platform-reqs would make
// a platform.php pin inert.
func TestFreshCreateProjectComposer1KeepsLegacyCommand(t *testing.T) {
	for _, composerVersion := range []string{"1", "1.10.27"} {
		got := magento2.BuildFreshCreateProjectCommand(magento2.Variant, bootstrap.Options{
			MetaPackage:     "magento/project-community-edition",
			Version:         "2.4.6",
			PHPVersion:      "7.4",
			ComposerVersion: composerVersion,
		})
		if got != legacyFreshCreateProject {
			t.Fatalf("composer %s: expected the legacy command, got %s", composerVersion, got)
		}
	}
}

// TestComposerPlatformPinMatchesProfilePHP drives every Magento 2.4 line
// through the runtime profile and asserts the create-project sequence pins
// config.platform.php to that line's PHP before dependencies are resolved,
// and that the dependency install no longer ignores the php requirement.
func TestComposerPlatformPinMatchesProfilePHP(t *testing.T) {
	versions := []string{"2.4.2", "2.4.3-p2", "2.4.4", "2.4.5-p8", "2.4.6", "2.4.6-p11", "2.4.7-p3", "2.4.8-p2", "2.4.9"}
	for _, version := range versions {
		result, err := engine.ResolveRuntimeProfile("magento2", version)
		if err != nil {
			t.Fatalf("%s: resolve profile: %v", version, err)
		}
		php := result.Profile.PHPVersion
		if php == "" {
			t.Fatalf("%s: profile has no PHP version", version)
		}
		got := magento2.BuildFreshCreateProjectCommand(magento2.Variant, bootstrap.Options{
			MetaPackage:     "magento/project-community-edition",
			Version:         version,
			PHPVersion:      php,
			ComposerVersion: result.Profile.ComposerVersion,
		})
		steps := strings.Split(got, " && ")
		want := []string{
			"set -e",
			"rm -rf /tmp/govard-create-project",
			"composer create-project -n --no-install --ignore-platform-reqs --add-repository --repository-url=https://repo.magento.com 'magento/project-community-edition' /tmp/govard-create-project '" + version + "'",
			"composer --working-dir=/tmp/govard-create-project config platform.php '" + php + "'",
			"composer --working-dir=/tmp/govard-create-project install -n --ignore-platform-req='ext-*' --ignore-platform-req='lib-*'",
			"if command -v rsync >/dev/null 2>&1; then rsync -a /tmp/govard-create-project/ /var/www/html/; else cp -a /tmp/govard-create-project/. /var/www/html/; fi",
			"rm -rf /tmp/govard-create-project",
		}
		if !reflect.DeepEqual(steps, want) {
			t.Errorf("%s (php %s):\n got  %q\n want %q", version, php, steps, want)
		}
	}
}

// TestProfileJSONHasNoUnknownFields decodes the embedded profiles.json with
// DisallowUnknownFields, so a key the Go structs do not model (and that
// json.Unmarshal would silently drop) fails the build.
func TestProfileJSONHasNoUnknownFields(t *testing.T) {
	if err := magento2.ValidateProfilesJSONStrictForTest(); err != nil {
		t.Fatalf("profiles.json has a field the loader does not model: %v", err)
	}
}
