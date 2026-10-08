package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"govard/internal/deploy"
)

func renderSandboxState(state *deploy.SandboxState) string {
	command := &cobra.Command{}
	var out bytes.Buffer
	command.SetOut(&out)
	printSandboxState(command, state, "Sandbox")
	return out.String()
}

// After `down --purge` there is no container, and `running: false` read like a
// stopped one that could be started. The state is named instead.
func TestPrintSandboxStateNamesAnAbsentSandbox(t *testing.T) {
	text := renderSandboxState(&deploy.SandboxState{Container: "govard-x-sandbox-1", RemoteName: "sandbox"})
	if !bytes.Contains([]byte(text), []byte("state:      absent")) {
		t.Fatalf("an absent sandbox must say so:\n%s", text)
	}
	if bytes.Contains([]byte(text), []byte("running:")) {
		t.Fatalf("an absent sandbox is not running:false:\n%s", text)
	}
}

func TestPrintSandboxStateKeepsRunningForAContainerThatExists(t *testing.T) {
	text := renderSandboxState(&deploy.SandboxState{Container: "c", Exists: true, Running: true, RemoteName: "sandbox"})
	for _, want := range []string{"state:      running", "running:    true"} {
		if !bytes.Contains([]byte(text), []byte(want)) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	stopped := renderSandboxState(&deploy.SandboxState{Container: "c", Exists: true, RemoteName: "sandbox"})
	if !bytes.Contains([]byte(stopped), []byte("state:      stopped")) {
		t.Errorf("a stopped container is stopped:\n%s", stopped)
	}
}
