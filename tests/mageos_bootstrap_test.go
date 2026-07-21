package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
)

func TestMageOSFreshCreateProjectUsesMageOSRepository(t *testing.T) {
	config := engine.Config{Framework: "mageos"}
	commandLine := cmd.RunBootstrapFreshCreateProjectCommandLineForTest(config, "mage-os/project-community-edition", "")
	if !containsSubstring(commandLine, "https://repo.mage-os.org") {
		t.Errorf("expected mageos fresh-install command to reference https://repo.mage-os.org, got: %s", commandLine)
	}
	if containsSubstring(commandLine, "repo.magento.com") {
		t.Errorf("expected mageos fresh-install command to NOT reference repo.magento.com, got: %s", commandLine)
	}
}

func TestMagento2FreshCreateProjectStillUsesMagentoRepository(t *testing.T) {
	config := engine.Config{Framework: "magento2"}
	commandLine := cmd.RunBootstrapFreshCreateProjectCommandLineForTest(config, "magento/project-community-edition", "")
	if !containsSubstring(commandLine, "https://repo.magento.com") {
		t.Errorf("expected magento2 fresh-install command to still reference https://repo.magento.com, got: %s", commandLine)
	}
}

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
