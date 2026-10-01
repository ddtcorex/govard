package tests

import (
	"fmt"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"
)

// Sources for the verified tables below (fetched 2026-10-01):
//
//   - OpenSearch version per patch, on-premises tab:
//     https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements
//   - Search engine value per line: "For versions earlier than 2.4.6, use the
//     elasticsearch7 value for the Elasticsearch 7 or OpenSearch engine. For
//     version 2.4.6 and later, use the opensearch value for the OpenSearch
//     engine."
//     https://experienceleague.adobe.com/en/docs/commerce-operations/upgrade-guide/prepare/prerequisites
//
// Lines that the system-requirements page no longer lists (2.4.0 to 2.4.3,
// 2.3, 2.2, 2.1, 2.0) are not in the search table: their values were not
// verified and are left as they are.

// verifiedOpenSearchRange is one run of patches on one line that Adobe lists
// with the same OpenSearch major.minor.
type verifiedOpenSearchRange struct {
	line       string // "2.4.6"
	fromP, toP int    // inclusive -pN range
	openSearch string // the profile's search_version Adobe lists
}

var verifiedOpenSearchTable = []verifiedOpenSearchRange{
	{"2.4.4", 0, 7, "1.2"},
	{"2.4.4", 8, 12, "1.3"},
	{"2.4.4", 13, 18, "2.19"},
	{"2.4.5", 0, 6, "1.2"},
	{"2.4.5", 7, 11, "1.3"},
	{"2.4.5", 12, 17, "2.19"},
	{"2.4.6", 0, 4, "2.5"},
	{"2.4.6", 5, 9, "2.12"},
	{"2.4.6", 10, 15, "2.19"},
	{"2.4.7", 0, 4, "2.12"},
	{"2.4.7", 5, 10, "2.19"},
	{"2.4.8", 0, 1, "2.19"},
	{"2.4.8", 2, 5, "3.0"},
	{"2.4.9", 0, 0, "3.0"},
}

// verifiedSetupSearchEngine is the --search-engine value Adobe documents for
// a line running OpenSearch.
func verifiedSetupSearchEngine(line string) string {
	if engine.IsNumericDotVersionAtLeast(line, "2.4.6") {
		return "opensearch"
	}
	return "elasticsearch7"
}

func patchVersion(line string, p int) string {
	if p == 0 {
		return line
	}
	return fmt.Sprintf("%s-p%d", line, p)
}

// TestEveryProfileLineHasAValidSearchPairing walks every patch Adobe lists for
// 2.4.4 to 2.4.9 and asserts the resolved profile runs the OpenSearch version
// Adobe lists for it, and that setup:install and the auto-configuration
// select the engine value Adobe documents for that line.
func TestEveryProfileLineHasAValidSearchPairing(t *testing.T) {
	t.Chdir(t.TempDir()) // keep the ElasticSuite probe off any real checkout
	for _, row := range verifiedOpenSearchTable {
		for p := row.fromP; p <= row.toP; p++ {
			version := patchVersion(row.line, p)
			result, err := engine.ResolveRuntimeProfile("magento2", version)
			if err != nil {
				t.Errorf("%s: resolve profile: %v", version, err)
				continue
			}
			profile := result.Profile
			if profile.Search != "opensearch" || profile.SearchVersion != row.openSearch {
				t.Errorf("%s: profile search = %s %s, Adobe lists opensearch %s", version, profile.Search, profile.SearchVersion, row.openSearch)
			}

			wantEngine := verifiedSetupSearchEngine(row.line)
			args := strings.Join(magento2.BuildSetupInstallArgs(magento2.Variant, version, "admin@sample.test", ""), " ")
			if !strings.Contains(args, "--search-engine="+wantEngine+" ") {
				t.Errorf("%s: setup:install should use --search-engine=%s, got %s", version, wantEngine, args)
			}

			config := engine.Config{Framework: "magento2", FrameworkVersion: version}
			config.Stack.Services.Search = profile.Search
			if got := magento2.ResolveMagentoSearchEngine(config); got != wantEngine {
				t.Errorf("%s: ResolveMagentoSearchEngine() = %q, want %q", version, got, wantEngine)
			}
		}
	}
}

// TestSetupInstallArgsAreValidForEachLine asserts the search option family in
// setup:install matches the engine value for every verified line: the
// --opensearch-* options only with opensearch, --elasticsearch-* only with
// elasticsearch7.
func TestSetupInstallArgsAreValidForEachLine(t *testing.T) {
	for _, line := range []string{"2.4.4", "2.4.5", "2.4.6", "2.4.7", "2.4.8", "2.4.9"} {
		engineName := verifiedSetupSearchEngine(line)
		other := map[string]string{"opensearch": "--elasticsearch-", "elasticsearch7": "--opensearch-"}[engineName]
		own := map[string]string{"opensearch": "--opensearch-host=", "elasticsearch7": "--elasticsearch-host="}[engineName]
		args := strings.Join(magento2.BuildSetupInstallArgs(magento2.Variant, line, "admin@sample.test", ""), " ")
		if !strings.Contains(args, "--search-engine="+engineName+" ") || !strings.Contains(args, own) {
			t.Errorf("%s: expected --search-engine=%s with %s options, got %s", line, engineName, own, args)
		}
		if strings.Contains(args, other) {
			t.Errorf("%s: %s options do not belong with --search-engine=%s: %s", line, other, engineName, args)
		}
	}
}

// TestComposerPinPerVerifiedLine asserts the Composer version each verified
// line resolves to: the 2.2 LTS where Adobe lists 2.2.26+, and the newest
// Composer where Adobe lists 2.9.3+ or 2.10. 2.4.0 and 2.4.1 are left out on
// purpose: Adobe documents Composer 1 for them, but Packagist shut down
// Composer 1 support on 2025-09-01, so their "2.2" pin is unverified.
func TestComposerPinPerVerifiedLine(t *testing.T) {
	cases := map[string]string{
		"2.4.4-p18": "2.2",
		"2.4.5-p17": "2.2",
		"2.4.6-p15": "2.2",
		"2.4.7-p10": "latest",
		"2.4.8-p5":  "latest",
		"2.4.9":     "latest",
	}
	for version, want := range cases {
		got := engine.ResolveComposerVersion(engine.Config{Framework: "magento2", FrameworkVersion: version})
		if got != want {
			t.Errorf("%s: composer = %q, want %q", version, got, want)
		}
	}
}
