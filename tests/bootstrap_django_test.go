package tests

import (
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestDjangoBootstrapCapabilities(t *testing.T) {
	b := bootstrap.NewDjangoBootstrap(bootstrap.Options{})
	if b.Name() != "django" {
		t.Errorf("Name() = %q, want %q", b.Name(), "django")
	}
	if b.SupportsFreshInstall() {
		t.Error("expected SupportsFreshInstall() to be false")
	}
	if !b.SupportsClone() {
		t.Error("expected SupportsClone() to be true")
	}
	if err := b.CreateProject(t.TempDir()); err == nil {
		t.Error("expected CreateProject to return an error (fresh-install unsupported)")
	}
}

func TestDjangoBootstrapFreshCommandsEmpty(t *testing.T) {
	b := bootstrap.NewDjangoBootstrap(bootstrap.Options{})
	if cmds := b.FreshCommands(); len(cmds) != 0 {
		t.Errorf("FreshCommands() = %v, want empty (fresh-install unsupported)", cmds)
	}
}
