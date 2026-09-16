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

	out := &bytes.Buffer{}
	command := cmd.RootCommandForTest()
	command.SetArgs([]string{"deploy", "check", "local"})
	command.SetOut(out)
	command.SetErr(io.Discard)
	if err := command.Execute(); err != nil {
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
