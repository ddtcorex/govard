package tests

import (
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestMageOSBootstrapDispatcherFreshCommands(t *testing.T) {
	cmds := bootstrap.MageOSFreshCommands(bootstrap.Options{})
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
