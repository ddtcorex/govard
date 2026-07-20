package tests

import (
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/magento1"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/openmage"
)

func TestMagentoFamilyDefinitions(t *testing.T) {
	m2 := magento2.Definition()
	if m2.Name != "magento2" {
		t.Errorf("magento2 Name = %q, want %q", m2.Name, "magento2")
	}
	if m2.Config.NGINXTemplate != "magento2.conf" {
		t.Errorf("magento2 Config.NGINXTemplate = %q, want %q", m2.Config.NGINXTemplate, "magento2.conf")
	}
	if m2.Bootstrap != nil {
		t.Error("magento2 Bootstrap should be nil (magento2 has no FrameworkBootstrap implementation yet)")
	}

	m1 := magento1.Definition()
	if m1.Name != "magento1" {
		t.Errorf("magento1 Name = %q, want %q", m1.Name, "magento1")
	}
	if m1.Bootstrap == nil {
		t.Fatal("magento1 Bootstrap should not be nil")
	}
	// magento1's FreshCommands() legitimately returns an empty slice with
	// zero-value Options (SupportsFreshInstall() is false - confirmed in
	// Plan 1's golden snapshot tests/testdata/framework_snapshots/magento1/
	// bootstrap_fresh_commands.json), so only confirm the call doesn't panic.
	_ = m1.Bootstrap(bootstrap.Options{}).FreshCommands()

	om := openmage.Definition()
	if om.Name != "openmage" {
		t.Errorf("openmage Name = %q, want %q", om.Name, "openmage")
	}
	if om.Bootstrap == nil {
		t.Fatal("openmage Bootstrap should not be nil")
	}
	if cmds := om.Bootstrap(bootstrap.Options{}).FreshCommands(); len(cmds) == 0 {
		t.Error("openmage Bootstrap factory should produce at least one fresh command")
	}
}
