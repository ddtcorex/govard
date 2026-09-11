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
