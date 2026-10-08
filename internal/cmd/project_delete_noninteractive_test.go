package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"govard/internal/engine"
)

func withStdinTerminal(t *testing.T, terminal bool) {
	t.Helper()
	previous := stdinIsTerminalFn
	stdinIsTerminalFn = func() bool { return terminal }
	t.Cleanup(func() { stdinIsTerminalFn = previous })
}

func TestConfirmDestructiveRefusesWithoutATerminal(t *testing.T) {
	withStdinTerminal(t, false)
	confirmed, err := confirmDestructive("Are you sure you want to proceed?", "--force")
	if err == nil {
		t.Fatalf("expected an error, got confirmed=%v", confirmed)
	}
	if confirmed {
		t.Errorf("a refusal must not confirm")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the error must name --force, got %q", err)
	}
}

func TestProjectDeleteWithoutTerminalOrForceRemovesNothing(t *testing.T) {
	withStdinTerminal(t, false)
	previous := projectDeleteForce
	projectDeleteForce = false
	t.Cleanup(func() { projectDeleteForce = previous })
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	command := &cobra.Command{}
	art := engine.ProjectArtifacts{Name: "ghost-shop"}
	if err := runUnregisteredDelete(command, art); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("unregistered delete: want an error naming --force, got %v", err)
	}
	if err := runOrphanDelete(command, engine.OrphanProject{Name: "ghost-shop"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("orphan delete: want an error naming --force, got %v", err)
	}
	if err := runOrphanDelete(command, engine.OrphanProject{Name: "bad name/../x"}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("unsafe orphan name: want an error naming --force, got %v", err)
	}
}
