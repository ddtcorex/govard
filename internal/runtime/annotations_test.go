package runtime

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func newCmd(use string, runnable bool, annotation string) *cobra.Command {
	cmd := &cobra.Command{Use: use}
	if runnable {
		cmd.RunE = func(*cobra.Command, []string) error { return nil }
	}
	if annotation != "" {
		cmd.Annotations = map[string]string{AnnotationRequires: annotation}
	}
	return cmd
}

func TestRequiresReadsOwnAnnotation(t *testing.T) {
	cmd := newCmd("init", true, "none")
	if got, want := Requires(cmd), []Capability{CapNone}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Requires = %#v, want %#v", got, want)
	}
}

func TestRequiresInheritsNearestParent(t *testing.T) {
	parent := newCmd("db", false, "docker")
	child := newCmd("query", true, "")
	parent.AddCommand(child)
	if got, want := Requires(child), []Capability{CapDocker}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Requires = %#v, want %#v", got, want)
	}
}

func TestRequiresDefaultsToDockerForRunnableCommands(t *testing.T) {
	cmd := newCmd("mystery", true, "")
	if got, want := Requires(cmd), []Capability{CapDocker}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Requires = %#v, want %#v", got, want)
	}
}

func TestRequiresIsNilForPureGroups(t *testing.T) {
	cmd := newCmd("group", false, "")
	if got := Requires(cmd); len(got) != 0 {
		t.Fatalf("Requires = %#v, want empty", got)
	}
}

func TestRequiresSupportsMultipleCapabilities(t *testing.T) {
	cmd := newCmd("sync", true, "ssh,rsync")
	want := []Capability{CapSSH, CapRsync}
	if got := Requires(cmd); !reflect.DeepEqual(got, want) {
		t.Fatalf("Requires = %#v, want %#v", got, want)
	}
}

func TestRequiresTreatsUnknownCapabilityAsDocker(t *testing.T) {
	cmd := newCmd("odd", true, "bogus")
	if got, want := Requires(cmd), []Capability{CapDocker}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Requires = %#v, want %#v", got, want)
	}
}

// TestAlwaysRunnableMatchesTopLevelCommand pins the always-runnable set to the
// top-level entry points. Matching the leaf name let `desktop doctor` skip the
// gate although it declares docker, and gated `completion bash` although
// `completion` is on the list.
func TestAlwaysRunnableMatchesTopLevelCommand(t *testing.T) {
	root := newCmd("govard", false, "")
	desktop := newCmd("desktop", true, "docker")
	doctor := newCmd("doctor", true, "")
	completion := newCmd("completion", true, "")
	bash := newCmd("bash", true, "")
	root.AddCommand(desktop, completion)
	desktop.AddCommand(doctor)
	completion.AddCommand(bash)

	if AlwaysRunnable(doctor) {
		t.Error("desktop doctor must not be always-runnable")
	}
	if !AlwaysRunnable(bash) {
		t.Error("completion bash must be always-runnable")
	}
	if !AlwaysRunnable(newCmd("doctor", true, "")) {
		t.Error("the top-level doctor must be always-runnable")
	}
}

// TestRequiresReportsNoneForAlwaysRunnableCommands keeps the manifest honest:
// these commands carry no runtime requirement, so `govard capabilities` must not
// report one the gate never enforces.
func TestRequiresReportsNoneForAlwaysRunnableCommands(t *testing.T) {
	root := newCmd("govard", false, "")
	completion := newCmd("completion", true, "")
	bash := newCmd("bash", true, "")
	root.AddCommand(completion)
	completion.AddCommand(bash)

	if got, want := Requires(bash), []Capability{CapNone}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Requires(completion bash) = %#v, want %#v", got, want)
	}
	if got, want := Requires(root), []Capability(nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("Requires(govard) = %#v, want %#v", got, want)
	}
}
