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

// newLocalDeployCheckProject seeds a checkout `govard deploy check` can run
// against: a magento2 project whose only remote is local, with the in-place
// layout the check resolves. It chdirs into the project for the rest of the
// test, because the check describes the checkout the process stands in.
func newLocalDeployCheckProject(t *testing.T) string {
	t.Helper()
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	// No deploy settings at all: everything below comes from the recipe.
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: magento2
domain: sample.test
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: `+filepath.Join(root, "public_html")+`
    local: true
    deploy:
      deploy_path: `+filepath.Join(root, ".deployer")+`
      branch: main
      repository: `+origin+`
`)
	// An in-place layout: the served path is a real directory, which is the one
	// the warning is about.
	for _, dir := range []string{"public_html", ".deployer"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return root
}

// runDeployCheckForTest runs the check command with stderr discarded, so what it
// returns is exactly what the operator reads on stdout.
func runDeployCheckForTest(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	out := &bytes.Buffer{}
	command := cmd.RootCommandForTest()
	command.SetArgs(append([]string{"deploy", "check"}, args...))
	command.SetOut(out)
	command.SetErr(io.Discard)
	return out, command.Execute()
}

// `govard deploy check` has to report on the deploy that will actually run.
//
// It resolved the project's settings without layering the recipe under them, so
// a project that configures nothing looked exactly like a project whose
// `sync_paths` were deliberately set to nothing — and the check warned about an
// in-place activation that copies no built files, which is not the activation
// this project gets. A preflight that describes a deploy nobody will run is
// worse than no preflight: it sends the operator to edit a file that is already
// right.
func TestDeployCheckReportsTheSettingsTheDeployWillUse(t *testing.T) {
	newLocalDeployCheckProject(t)

	out, err := runDeployCheckForTest(t, "local")
	if err != nil {
		t.Fatalf("deploy check: %v\n%s", err, out.String())
	}

	printed := out.String()
	if strings.Contains(printed, "sync_paths is empty") {
		t.Fatalf("the check described a deploy that will not run — it read the project's settings without the recipe's:\n%s", printed)
	}
	// And it did run: a check that printed nothing would pass the assertion above
	// for the wrong reason.
	if !strings.Contains(printed, "is deployable") {
		t.Fatalf("the check produced no verdict:\n%s", printed)
	}
}

// The prepare preflight refuses a checkout that carries submodules, because
// `git archive` cannot carry their content and a release with empty submodule
// directories would look like it worked. The check builds that same preflight,
// so it must reach the same verdict: an operator who was told "deployable" for
// such a commit only learned otherwise once a release directory already existed.
func TestDeployCheckRefusesACommitThatUsesSubmodules(t *testing.T) {
	root := newLocalDeployCheckProject(t)
	writeFile(t, filepath.Join(root, ".gitmodules"), "[submodule \"vendor/core\"]\n\tpath = vendor/core\n")

	out, err := runDeployCheckForTest(t, "local")
	if err == nil || !strings.Contains(err.Error(), ".gitmodules") {
		t.Fatalf("deploy check accepted a checkout with submodules: %v\n%s", err, out.String())
	}
}

// A project that declares a private Composer repository and has no credential to
// authenticate it is the most common reason a deploy fails on its first
// dependency install. The preflight computes that warning from the checkout on
// disk; the check has to print it, or the operator only meets it once the target
// is already being built.
func TestDeployCheckPrintsTheCredentialsWarning(t *testing.T) {
	t.Setenv("COMPOSER_AUTH", "")
	root := newLocalDeployCheckProject(t)
	writeFile(t, filepath.Join(root, "composer.json"),
		`{"repositories":[{"type":"vcs","url":"git@example.invalid:private/repo.git"}]}`)

	out, err := runDeployCheckForTest(t, "local")
	if err != nil {
		t.Fatalf("deploy check: %v\n%s", err, out.String())
	}

	printed := out.String()
	// The repository URL is the part only a reader of this checkout can know, so
	// it is what makes the assertion a check on the local probe rather than on
	// the warning's wording.
	if !strings.Contains(printed, "no credentials are available") || !strings.Contains(printed, "private/repo.git") {
		t.Fatalf("the credentials warning was not printed:\n%s", printed)
	}
	// The check still reached its verdict: a run that died before the warning
	// could print would otherwise satisfy the assertion for the wrong reason.
	if !strings.Contains(printed, "is deployable") {
		t.Fatalf("the check produced no verdict:\n%s", printed)
	}
}
