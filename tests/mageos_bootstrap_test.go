package tests

import (
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/mageos"
)

func TestMageOSBootstrapDispatcherFreshCommands(t *testing.T) {
	cmds := mageos.FreshCommands(bootstrap.Options{})
	if len(cmds) == 0 {
		t.Fatal("expected at least one fresh command for mageos")
	}
	if !containsSubstring(cmds[0], "mage-os/project-community-edition") {
		t.Errorf("expected command to reference mage-os/project-community-edition, got %q", cmds[0])
	}
	if !containsSubstring(cmds[0], "repo.mage-os.org") {
		t.Errorf("expected command to reference repo.mage-os.org, got %q", cmds[0])
	}
}

func TestMageOSFreshCommandsUsesExplicitVersion(t *testing.T) {
	cmds := mageos.FreshCommands(bootstrap.Options{Version: "1.3.1"})
	if len(cmds) != 1 {
		t.Fatalf("expected one command, got %d", len(cmds))
	}
	if !containsSubstring(cmds[0], "mage-os/project-community-edition:1.3.1") || !containsSubstring(cmds[0], "https://repo.mage-os.org") {
		t.Fatalf("unexpected Mage-OS create-project command: %q", cmds[0])
	}
}
