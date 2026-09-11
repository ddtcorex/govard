package cmd

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

// resolveCommandForTest resolves a command path from the live command tree.
func resolveCommandForTest(t *testing.T, path string) *cobra.Command {
	t.Helper()
	command, _, err := rootCmd.Find(strings.Fields(path))
	if err != nil {
		t.Fatalf("resolve %q: %v", path, err)
	}
	if want := "govard " + path; command.CommandPath() != want {
		t.Fatalf("resolve %q = %q, want %q", path, command.CommandPath(), want)
	}
	return command
}

// TestHostOnlyCommandsStayDockerFree pins the Docker-free surface to the work a
// command actually does. These all only read the project, the registry, or the
// local machine, so they must run on a host with no container runtime even
// though the group they live in orchestrates containers — a diagnostic that
// cannot run when the thing it diagnoses is broken is the case that matters.
func TestHostOnlyCommandsStayDockerFree(t *testing.T) {
	for _, path := range []string{
		"desktop doctor",
		"vscode setup",
		"domain list",
		"project list",
		"project open",
	} {
		command := resolveCommandForTest(t, path)
		if got, want := runtime.Requires(command), []runtime.Capability{runtime.CapNone}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s requires %v, want %v", command.CommandPath(), got, want)
		}
	}
}

// TestContainerBackedSiblingsKeepDocker is the other half of the same split: a
// child that overrides its group must not open a hole in the siblings that
// really do need the container runtime.
func TestContainerBackedSiblingsKeepDocker(t *testing.T) {
	for _, path := range []string{
		"desktop",
		"vscode php",
		"vscode phpcs",
		"domain add",
		"domain remove",
		"project delete",
		"project orphans",
		// The lock file records the resolved runtime environment — docker and
		// compose versions plus every service image digest — so all three lock
		// commands read the container runtime.
		"config auto",
		"lock",
		"lock generate",
		"lock check",
		"lock diff",
	} {
		command := resolveCommandForTest(t, path)
		if got, want := runtime.Requires(command), []runtime.Capability{runtime.CapDocker}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s requires %v, want %v", command.CommandPath(), got, want)
		}
	}
}
