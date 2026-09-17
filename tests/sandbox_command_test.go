package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/runtime"
)

func TestSandboxCommandIsTopLevel(t *testing.T) {
	root := cmd.RootCommandForTest()
	found, _, err := root.Find([]string{"sandbox", "up"})
	if err != nil {
		t.Fatalf("govard sandbox up must exist at the root: %v", err)
	}
	if found.Use != "up" {
		t.Fatalf("found command %q, want the sandbox up command", found.Use)
	}

	// cobra's Find only errors for an unknown command at the root: under
	// `deploy`, a stray `sandbox` validates as its one positional arg and
	// returns the deploy command itself. Assert on identity instead: the
	// found command must be deploy, never a sandbox subcommand.
	if found, _, _ := root.Find([]string{"deploy", "sandbox"}); found.Name() == "sandbox" {
		t.Fatal("govard deploy sandbox must no longer exist")
	}
}

func TestSandboxCommandRequiresDocker(t *testing.T) {
	root := cmd.RootCommandForTest()
	found, _, err := root.Find([]string{"sandbox"})
	if err != nil {
		t.Fatalf("find sandbox: %v", err)
	}
	if got := runtime.Requires(found); len(got) != 1 || got[0] != runtime.CapDocker {
		t.Fatalf("requires = %v, want [docker]", got)
	}
}
