package tests

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"

	"github.com/pterm/pterm"
)

// restoreNotice is the fragment of the wrapper's single report line. Counting
// it keeps the end-to-end cases below independent of the surrounding
// ConfigureMagento chatter.
const restoreNotice = "the original was restored"

// The fixtures use one module per line, which is the shape a real
// `app/etc/config.php` has and what makes a reorder a permutation of lines:
// the sorted contents are identical and no module was added or removed.
const configPHPFixture = `<?php
return [
    'modules' => [
        'Magento_Store' => 1,
        'Magento_Backend' => 1,
    ],
    'scopes' => [
        'alpha' => 1,
        'beta' => 2,
    ],
];
`

// configPHPReordered reorders only the 'scopes' entries, which Magento writes
// in a nondeterministic order and which are keyed, so order carries no meaning.
const configPHPReordered = `<?php
return [
    'modules' => [
        'Magento_Store' => 1,
        'Magento_Backend' => 1,
    ],
    'scopes' => [
        'beta' => 2,
        'alpha' => 1,
    ],
];
`

// configPHPModulesReordered is what setup:upgrade writes when a new
// <sequence> dependency moves a module: same entries, different load order.
const configPHPModulesReordered = `<?php
return [
    'modules' => [
        'Magento_Backend' => 1,
        'Magento_Store' => 1,
    ],
    'scopes' => [
        'alpha' => 1,
        'beta' => 2,
    ],
];
`

const configPHPModulesAndScopesReordered = `<?php
return [
    'modules' => [
        'Magento_Backend' => 1,
        'Magento_Store' => 1,
    ],
    'scopes' => [
        'beta' => 2,
        'alpha' => 1,
    ],
];
`

const configPHPBackendDisabled = `<?php
return [
    'modules' => [
        'Magento_Store' => 1,
        'Magento_Backend' => 0,
    ],
    'scopes' => [
        'alpha' => 1,
        'beta' => 2,
    ],
];
`

const configPHPExtraModule = `<?php
return [
    'modules' => [
        'Magento_Store' => 1,
        'Magento_Backend' => 1,
        'Magento_Cms' => 1,
    ],
    'scopes' => [
        'alpha' => 1,
        'beta' => 2,
    ],
];
`

// writeConfigPHPFixture lays down a project root holding the fixture
// config.php and returns the root and the file path a stub command rewrites.
func writeConfigPHPFixture(t *testing.T) (projectRoot string, configPath string) {
	t.Helper()
	projectRoot = t.TempDir()
	configPath = filepath.Join(projectRoot, "app", "etc", "config.php")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir app/etc: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(configPHPFixture), 0o644); err != nil {
		t.Fatalf("write config.php: %v", err)
	}
	return projectRoot, configPath
}

// capturePterm redirects the report printers for the duration of the test.
func capturePterm(t *testing.T) *bytes.Buffer {
	t.Helper()
	var captured bytes.Buffer
	pterm.SetDefaultOutput(&captured)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
	return &captured
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// reportLines is the operator-visible output: escape codes stripped, blanks
// dropped, so a prefix can be matched without depending on colour support.
func reportLines(raw string) []string {
	var lines []string
	for _, line := range strings.Split(ansiEscape.ReplaceAllString(raw, ""), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// A `govard config auto` run reaches `app:config:import`, whose importer
// rewrites the module list in its own order. On an unchanged project that
// left a tracked file dirty (26 insertions / 26 deletions, no module added or
// removed) and poisoned the git_dirty signal an audit run reports on.
func TestRunPreservingUnchangedConfigPHPRestoresAReorderedFile(t *testing.T) {
	projectRoot, configPath := writeConfigPHPFixture(t)
	captured := capturePterm(t)

	err := magento2.RunPreservingUnchangedConfigPHPForTest(projectRoot, func() error {
		return os.WriteFile(configPath, []byte(configPHPReordered), 0o644)
	})
	if err != nil {
		t.Fatalf("run the wrapped command: %v", err)
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config.php: %v", readErr)
	}
	if string(got) != configPHPFixture {
		t.Fatalf("config.php was not restored to its original bytes, got:\n%s", got)
	}

	lines := reportLines(captured.String())
	if len(lines) != 1 {
		t.Fatalf("expected exactly one report line, got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "config.php") {
		t.Fatalf("the report line does not name the file it restored: %q", lines[0])
	}
	if !strings.Contains(lines[0], "INFO") {
		t.Fatalf("a restored reorder is a note, not a warning: %q", lines[0])
	}
}

// When Magento genuinely changes the module set, its output stands: the helper
// must not undo a real change, and must stay quiet about doing nothing.
func TestRunPreservingUnchangedConfigPHPKeepsAGenuineChange(t *testing.T) {
	cases := []struct {
		name  string
		after string
	}{
		{name: "a module is disabled", after: configPHPBackendDisabled},
		{name: "a module is added", after: configPHPExtraModule},
		{name: "the module load order changes", after: configPHPModulesReordered},
		{name: "the module load order and scopes both change", after: configPHPModulesAndScopesReordered},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot, configPath := writeConfigPHPFixture(t)
			captured := capturePterm(t)

			err := magento2.RunPreservingUnchangedConfigPHPForTest(projectRoot, func() error {
				return os.WriteFile(configPath, []byte(tc.after), 0o644)
			})
			if err != nil {
				t.Fatalf("run the wrapped command: %v", err)
			}

			got, readErr := os.ReadFile(configPath)
			if readErr != nil {
				t.Fatalf("read config.php: %v", readErr)
			}
			if string(got) != tc.after {
				t.Fatalf("Magento's real change was reverted, got:\n%s", got)
			}
			if lines := reportLines(captured.String()); len(lines) != 0 {
				t.Fatalf("expected nothing printed for a genuine change, got %q", lines)
			}
		})
	}
}

// A project with no config.php at all (the file has never been written, or the
// command failed before touching it) still runs, and still answers with the
// command's own result.
func TestRunPreservingUnchangedConfigPHPRunsWithNoConfigPHP(t *testing.T) {
	captured := capturePterm(t)
	calls := 0

	err := magento2.RunPreservingUnchangedConfigPHPForTest(t.TempDir(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("run the wrapped command: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected the command to run once, it ran %d times", calls)
	}
	if lines := reportLines(captured.String()); len(lines) != 0 {
		t.Fatalf("expected nothing printed, got %q", lines)
	}
}

// A run that leaves config.php byte-identical has nothing to report. Printing
// a restore line there would claim a reorder that never happened, so this case
// is what keeps the report line honest.
func TestRunPreservingUnchangedConfigPHPSaysNothingWhenMagentoRewritesNothing(t *testing.T) {
	projectRoot, configPath := writeConfigPHPFixture(t)
	captured := capturePterm(t)

	err := magento2.RunPreservingUnchangedConfigPHPForTest(projectRoot, func() error {
		// The same bytes the file already had: the importer's order matched the
		// operator's this time.
		return os.WriteFile(configPath, []byte(configPHPFixture), 0o644)
	})
	if err != nil {
		t.Fatalf("run the wrapped command: %v", err)
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config.php: %v", readErr)
	}
	if string(got) != configPHPFixture {
		t.Fatalf("expected config.php to be untouched, got:\n%s", got)
	}
	if lines := reportLines(captured.String()); len(lines) != 0 {
		t.Fatalf("a run that changed nothing must say nothing, got %q", lines)
	}
}

// An empty project root has nothing to compare against, and guessing one is
// worse than doing nothing: substituting the working directory here would let a
// govard process started anywhere restore a file it has no business touching.
func TestRunPreservingUnchangedConfigPHPDoesNotGuessAProjectRoot(t *testing.T) {
	elsewhere := t.TempDir()
	configPath := filepath.Join(elsewhere, "app", "etc", "config.php")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir app/etc: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(configPHPFixture), 0o644); err != nil {
		t.Fatalf("write config.php: %v", err)
	}
	chdirForTest(t, elsewhere)
	captured := capturePterm(t)
	calls := 0

	err := magento2.RunPreservingUnchangedConfigPHPForTest("", func() error {
		calls++
		return os.WriteFile(configPath, []byte(configPHPReordered), 0o644)
	})
	if err != nil {
		t.Fatalf("run the wrapped command: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected the command to run once, it ran %d times", calls)
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config.php: %v", readErr)
	}
	if string(got) != configPHPReordered {
		t.Fatalf("an empty project root was resolved to %s instead of being left alone", elsewhere)
	}
	if lines := reportLines(captured.String()); len(lines) != 0 {
		t.Fatalf("expected nothing printed, got %q", lines)
	}
}

// The call site branches on this error to fall back to setup:upgrade, so the
// helper hands back exactly what the command returned — unwrapped, and with
// the restore still done.
func TestRunPreservingUnchangedConfigPHPReturnsTheCommandsOwnError(t *testing.T) {
	projectRoot, configPath := writeConfigPHPFixture(t)
	capturePterm(t)
	commandFailure := errors.New("app:config:import failed: exit status 1")

	err := magento2.RunPreservingUnchangedConfigPHPForTest(projectRoot, func() error {
		if writeErr := os.WriteFile(configPath, []byte(configPHPReordered), 0o644); writeErr != nil {
			return writeErr
		}
		return commandFailure
	})
	if err != commandFailure {
		t.Fatalf("expected the command's own error, got %v", err)
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config.php: %v", readErr)
	}
	if string(got) != configPHPFixture {
		t.Fatalf("a failing command must still get its reorder restored, got:\n%s", got)
	}
}

// A checkout the operator cannot write back — a read-only mount, or a
// config.php the container owns — is real. run()'s error is the command's
// answer and the restore is housekeeping, so an impossible restore is warned
// about rather than reported as a Magento failure.
func TestRunPreservingUnchangedConfigPHPWarnsWhenTheRestoreCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions, so there is nothing to fail here")
	}
	projectRoot, configPath := writeConfigPHPFixture(t)
	captured := capturePterm(t)

	// The importer runs inside the container, as a user govard cannot become.
	// A self-chmod stands in for that ownership and fails the restore the same
	// way a container-owned file does.
	err := magento2.RunPreservingUnchangedConfigPHPForTest(projectRoot, func() error {
		if writeErr := os.WriteFile(configPath, []byte(configPHPReordered), 0o644); writeErr != nil {
			return writeErr
		}
		return os.Chmod(configPath, 0o444)
	})
	if err != nil {
		t.Fatalf("a failed restore must not be reported as a failed command, got: %v", err)
	}

	lines := reportLines(captured.String())
	if len(lines) != 1 {
		t.Fatalf("expected exactly one report line, got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "WARNING") {
		t.Fatalf("an impossible restore must warn, not pass quietly: %q", lines[0])
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config.php: %v", readErr)
	}
	if string(got) != configPHPReordered {
		t.Fatalf("expected the reorder to survive an impossible restore, got:\n%s", got)
	}
}

// The cases above exercise the helper directly, so they would all still pass
// with the helper left unwrapped at both call sites. The cases below drive the
// real ConfigureMagento behind a fake docker, because the wrapping is the fix:
// a helper nobody calls reorders config.php exactly as before.
func TestConfigureMagentoRestoresTheConfigRewriteTheRepairStepMade(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shim targets POSIX sh")
	}

	cases := []struct {
		name string
		// importExit and upgradeExit are what the fake docker answers with for
		// app:config:import and setup:upgrade; both commands rewrite
		// app/etc/config.php into the reordered copy whenever they are reached.
		importExit    int
		upgradeExit   int
		upgradeOutput string
		// retry describes the second setup:upgrade, the one made after the
		// search index was unblocked. It only exists when the first attempt
		// failed with a search-block error (upgradeOutput).
		retry retryStep
		// wantRan are the repair commands the fake docker must have been asked
		// for; without them the case would pass on a run that never reached the
		// repair path at all.
		wantRan []string
		// wantRestored is what the file must hold at the end, and wantRestores
		// how many times a restore should have been reported.
		wantRestores int
		wantRestored string
	}{
		{
			// The call site under the needsConfigImport branch.
			name:         "app:config:import reorders it",
			importExit:   0,
			wantRan:      []string{"app:config:import"},
			wantRestores: 1,
			wantRestored: configPHPFixture,
		},
		{
			// The call site inside the import-failed fallback. The import ran
			// and reordered it too, so this case restores twice.
			name:         "the setup:upgrade fallback reorders it",
			importExit:   1,
			upgradeExit:  0,
			wantRan:      []string{"app:config:import", "setup:upgrade"},
			wantRestores: 2,
			wantRestored: configPHPFixture,
		},
		{
			// The search-index retry. Every earlier step reorders the file and
			// is restored; the retry then reorders it once more and must be
			// restored too (#512), so the file is byte-identical at the end.
			name:          "the search-index retry reorders it and it is restored",
			importExit:    1,
			upgradeExit:   1,
			upgradeOutput: "cluster_block_exception: index is read-only",
			retry:         retryStep{exit: 0, writes: configPHPReordered},
			wantRan:       []string{"app:config:import", "setup:upgrade"},
			wantRestores:  3,
			wantRestored:  configPHPFixture,
		},
		{
			// A retry that still fails after reordering is restored as well:
			// the wrapper returns the command's own error and the caller
			// reports it.
			name:          "a failing search-index retry that reorders it is restored",
			importExit:    1,
			upgradeExit:   1,
			upgradeOutput: "cluster_block_exception: index is read-only",
			retry:         retryStep{exit: 1, writes: configPHPReordered},
			wantRan:       []string{"app:config:import", "setup:upgrade"},
			wantRestores:  3,
			wantRestored:  configPHPFixture,
		},
		{
			// The retry genuinely adds a module: Magento's output stands.
			name:          "the search-index retry that changes the module set keeps Magento's output",
			importExit:    1,
			upgradeExit:   1,
			upgradeOutput: "cluster_block_exception: index is read-only",
			retry:         retryStep{exit: 0, writes: configPHPExtraModule},
			wantRan:       []string{"app:config:import", "setup:upgrade"},
			wantRestores:  2,
			wantRestored:  configPHPExtraModule,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, configPath, dockerLog := configureMagentoFixture(t, tc.importExit, tc.upgradeExit, tc.upgradeOutput, tc.retry)
			captured := capturePterm(t)

			// The shim keeps failing setup:config:set, so the run ends in the
			// "command failed" error. What this case is about is the file it
			// leaves behind, not how the run ends.
			_ = magento2.ConfigureMagento("sample", configureMagentoConfig(), true, nil)

			commands, readErr := os.ReadFile(dockerLog)
			if readErr != nil {
				t.Fatalf("read the fake docker log: %v", readErr)
			}
			for _, want := range tc.wantRan {
				if !strings.Contains(string(commands), want) {
					t.Fatalf("expected the repair path to run %q, docker log:\n%s", want, commands)
				}
			}

			if got := strings.Count(captured.String(), restoreNotice); got != tc.wantRestores {
				t.Fatalf("expected %d restore notices, got %d in:\n%s", tc.wantRestores, got, captured.String())
			}

			raw, readErr := os.ReadFile(configPath)
			if readErr != nil {
				t.Fatalf("read config.php: %v", readErr)
			}
			if string(raw) != tc.wantRestored {
				t.Fatalf("expected config.php to hold:\n%s\ngot:\n%s", tc.wantRestored, raw)
			}
		})
	}
}

// retryStep scripts the setup:upgrade made after the search index was
// unblocked: the exit code the fake docker answers with and the config.php
// content it leaves behind (empty means the same reordered copy as the earlier
// steps). It applies only when the first attempt failed with upgradeOutput.
type retryStep struct {
	exit   int
	writes string
}

// configureMagentoFixture lays down a project root, puts a fake docker on PATH
// and makes it the working directory, because ConfigureMagento resolves the
// project root from the working directory exactly as `govard config auto` does.
// Every docker call is logged, and setup:config:set always fails with a
// setup:upgrade hint so the repair path is entered at all.
func configureMagentoFixture(t *testing.T, importExit, upgradeExit int, upgradeOutput string, retry retryStep) (projectRoot, configPath, dockerLog string) {
	t.Helper()

	projectRoot = t.TempDir()
	configPath = filepath.Join(projectRoot, "app", "etc", "config.php")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir app/etc: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(configPHPFixture), 0o644); err != nil {
		t.Fatalf("write config.php: %v", err)
	}
	// What the fake importer writes: the same lines, in another order.
	reordered := filepath.Join(projectRoot, "app", "etc", "reordered.php")
	if err := os.WriteFile(reordered, []byte(configPHPReordered), 0o644); err != nil {
		t.Fatalf("write reordered.php: %v", err)
	}

	retryTarget := reordered
	if retry.writes != "" {
		retryTarget = filepath.Join(projectRoot, "app", "etc", "retry.php")
		if err := os.WriteFile(retryTarget, []byte(retry.writes), 0o644); err != nil {
			t.Fatalf("write retry.php: %v", err)
		}
	}

	shimDir := t.TempDir()
	dockerLog = filepath.Join(shimDir, "docker.log")
	firstUpgradeMarker := filepath.Join(shimDir, "first-upgrade-done")
	script := fmt.Sprintf(`#!/bin/sh
echo "$*" >> %[1]s
case "$*" in
*setup:config:set*)
	echo "Please refer to the 'setup:upgrade' command to finish the installation."
	exit 1
	;;
*app:config:import*)
	cp %[2]s %[3]s
	exit %[4]d
	;;
*setup:upgrade*)
	# The first attempt answers as configured. A later one is the retry made
	# after the search index was unblocked and answers with the retry step.
	if [ -n "%[5]s" ] && [ -e %[7]s ]; then
		cp %[8]s %[3]s
		exit %[9]d
	fi
	touch %[7]s
	cp %[2]s %[3]s
	[ -n "%[5]s" ] && echo "%[5]s"
	exit %[6]d
	;;
esac
exit 0
`, dockerLog, reordered, configPath, importExit, upgradeOutput, upgradeExit, firstUpgradeMarker, retryTarget, retry.exit)

	dockerPath := filepath.Join(shimDir, "docker")
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	chdirForTest(t, projectRoot)

	// composer install would shell out to the fake docker for real; the case is
	// about config.php, not about dependencies.
	restore := magento2.SetMagentoComposerInstallRunnerForTest(func(string, engine.Config, io.Writer, io.Writer) error { return nil })
	t.Cleanup(restore)

	return projectRoot, configPath, dockerLog
}

// configureMagentoConfig is the project the end-to-end cases configure. The
// cache and search services are switched off deliberately: ConfigureMagento
// otherwise waits on those containers, and waiting out a container that is not
// there is the documented way a unit test blows its time budget.
func configureMagentoConfig() engine.Config {
	return engine.Config{
		ProjectName: "sample",
		Framework:   "magento2",
		Stack: engine.Stack{
			Services: engine.Services{Cache: "none", Search: "none"},
		},
	}
}
