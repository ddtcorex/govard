package tests

import (
	"fmt"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"
)

// Primary source for the search-option gate (checked 2026-10-02): the
// magento2 repository file
// setup/src/Magento/Setup/Model/SearchConfigOptionsList.php, which declares
// the setup:install --search-engine and --elasticsearch-* options, returns
// 404 at tags 2.3.5, 2.3.7, 2.3.7-p4 and 2.4.0-beta1 and 200 from 2.4.0. So
// 2.0 to 2.3 take no search install arguments at all (2.3 uses the mysql
// search by default), and 2.4.0 to 2.4.3 accept elasticsearch5/6/7 only.

// installProfileLines is every version line profiles.json defines, one
// representative per patch or patch range.
func installProfileLines() []string {
	lines := []string{
		"2.0.0", "2.0.18",
		"2.1.0", "2.1.18",
		"2.2.0", "2.2.1", "2.2.11",
		"2.3.0", "2.3.1", "2.3.4", "2.3.5", "2.3.6", "2.3.7", "2.3.7-p4",
	}
	for patch := 0; patch <= 9; patch++ {
		lines = append(lines, fmt.Sprintf("2.4.%d", patch))
		lines = append(lines, fmt.Sprintf("2.4.%d-p3", patch))
	}
	return lines
}

func TestSetupInstallSearchOptionsAreValidForEveryProfileLine(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, version := range installProfileLines() {
		args := strings.Join(magento2.BuildSetupInstallArgs(magento2.Variant, version, "admin@sample.test", ""), " ")
		hasSearch := strings.Contains(args, "--search-engine=") ||
			strings.Contains(args, "--elasticsearch-") || strings.Contains(args, "--opensearch-")

		if !engine.IsNumericDotVersionAtLeast(version, "2.4.0") {
			if hasSearch {
				t.Errorf("%s: setup:install has no search options before 2.4.0, got %s", version, args)
			}
			continue
		}
		wantEngine := "elasticsearch7"
		if engine.IsNumericDotVersionAtLeast(version, "2.4.6") {
			wantEngine = "opensearch"
		}
		if !strings.Contains(args, "--search-engine="+wantEngine+" ") {
			t.Errorf("%s: want --search-engine=%s, got %s", version, wantEngine, args)
		}
		other := map[string]string{"opensearch": "--elasticsearch-", "elasticsearch7": "--opensearch-"}[wantEngine]
		if strings.Contains(args, other) {
			t.Errorf("%s: %s options do not belong with %s: %s", version, other, wantEngine, args)
		}
	}
}

// Composer evidence, measured live 2026-10-02 against repo.magento.com with
// `composer create-project --no-install --ignore-platform-reqs` followed by
// `composer update --dry-run --ignore-platform-reqs` for 2.0.18, 2.1.18,
// 2.2.11, 2.3.0, 2.3.7, 2.4.0, 2.4.1 and 2.4.3 (exit codes in the commit
// message):
//   - Composer 2.2.30 (composer:2.2 image) resolves every one, exit 0.
//   - Composer 2.10.3 (composer:2 image) exits 2 on every one only because
//     security-advisory blocking hides old symfony/process, phpunit and
//     composer/composer releases; with the blocking off (govard writes
//     audit.block-insecure=false globally in ensureComposerConfig) it also
//     resolves every one, exit 0.
//
// So no line is unresolvable and Composer 1 stays impossible (Packagist shut
// it down on 2025-09-01). Pins below 2.4.0 are therefore not required and
// stay unset on purpose; pins only exist where Adobe lists Composer 2.2.
// 2.4.7 and later follow Adobe's newest-Composer listing.
type composerLine struct {
	line   string
	pin    string // "" means no pin, resolved as "latest"
	reason string // required whenever pin is ""
}

var composerPinTable = []composerLine{
	{"2.0", "", "no pin needed: Composer 2.2.30 and 2.10.3 (blocking off) both resolve 2.0.18, measured 2026-10-02"},
	{"2.1", "", "no pin needed: both resolve 2.1.18, measured 2026-10-02"},
	{"2.2", "", "no pin needed: both resolve 2.2.11, measured 2026-10-02"},
	{"2.3", "", "no pin needed: both resolve 2.3.0 and 2.3.7, measured 2026-10-02; Adobe lists Composer 1 up to 2.3.6, impossible since 2025-09-01"},
	{"2.4.0", "2.2", ""},
	{"2.4.1", "2.2", ""},
	{"2.4.2", "2.2", ""},
	{"2.4.3", "2.2", ""},
	{"2.4.4", "2.2", ""},
	{"2.4.5", "2.2", ""},
	{"2.4.6", "2.2", ""},
	{"2.4.7", "", "Adobe system requirements list Composer 2.9.3+ or 2.10, newest Composer"},
	{"2.4.8", "", "Adobe system requirements list Composer 2.9.3+ or 2.10, newest Composer"},
	{"2.4.9", "", "Adobe system requirements list Composer 2.9.3+ or 2.10, newest Composer"},
}

func composerRowFor(version string) (composerLine, bool) {
	for _, row := range composerPinTable {
		if version == row.line || strings.HasPrefix(version, row.line+".") || strings.HasPrefix(version, row.line+"-") {
			return row, true
		}
	}
	return composerLine{}, false
}

func TestEveryProfileLineHasAComposerPinOrDocumentedReason(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, version := range installProfileLines() {
		row, ok := composerRowFor(version)
		if !ok {
			t.Errorf("%s: no entry in the composer pin table", version)
			continue
		}
		if row.pin == "" && strings.TrimSpace(row.reason) == "" {
			t.Errorf("%s: no pin and no documented reason", version)
		}
		want := row.pin
		if want == "" {
			want = "latest"
		}
		got := engine.ResolveComposerVersion(engine.Config{Framework: "magento2", FrameworkVersion: version})
		if got != want {
			t.Errorf("%s: composer = %q, want %q (%s)", version, got, want, row.line)
		}
	}
}
