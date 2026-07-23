package tests

import (
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/cakephp"
	"govard/internal/frameworks/django"
	"govard/internal/frameworks/drupal"
	"govard/internal/frameworks/emdash"
	"govard/internal/frameworks/laravel"
	"govard/internal/frameworks/magento1"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/nextjs"
	"govard/internal/frameworks/openmage"
	"govard/internal/frameworks/prestashop"
	"govard/internal/frameworks/shopware"
	"govard/internal/frameworks/symfony"
	"govard/internal/frameworks/types"
	"govard/internal/frameworks/wordpress"
)

func TestMagentoFamilyDefinitions(t *testing.T) {
	m2 := magento2.Definition()
	if m2.Name != "magento2" {
		t.Errorf("magento2 Name = %q, want %q", m2.Name, "magento2")
	}
	if m2.Config.NGINXTemplate != "magento2.conf" {
		t.Errorf("magento2 Config.NGINXTemplate = %q, want %q", m2.Config.NGINXTemplate, "magento2.conf")
	}
	if m2.Bootstrap == nil {
		t.Fatal("magento2 Bootstrap should not be nil")
	}
	if got := m2.Bootstrap(bootstrap.Options{Version: "2.4.7"}).FreshCommands(); len(got) != 1 || !strings.Contains(got[0], "magento/project-community-edition:2.4.7") {
		t.Errorf("magento2 Bootstrap FreshCommands = %v, want a magento2 create-project command", got)
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

func TestMainstreamPHPFrameworkDefinitions(t *testing.T) {
	cases := []struct {
		name string
		def  types.FrameworkDefinition
	}{
		{"laravel", laravel.Definition()},
		{"symfony", symfony.Definition()},
		{"drupal", drupal.Definition()},
		{"wordpress", wordpress.Definition()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.def.Name != tc.name {
				t.Errorf("Name = %q, want %q", tc.def.Name, tc.name)
			}
			if tc.def.Bootstrap == nil {
				t.Fatalf("%s Bootstrap should not be nil", tc.name)
			}
			if cmds := tc.def.Bootstrap(bootstrap.Options{}).FreshCommands(); len(cmds) == 0 {
				t.Errorf("%s Bootstrap factory should produce at least one fresh command", tc.name)
			}
		})
	}

	if wordpress.Definition().Aliases[0] != "wp" {
		t.Errorf("wordpress Aliases = %v, want first alias %q", wordpress.Definition().Aliases, "wp")
	}
}

func TestRemainingPHPFrameworkDefinitions(t *testing.T) {
	cases := []struct {
		name string
		def  types.FrameworkDefinition
	}{
		{"shopware", shopware.Definition()},
		{"cakephp", cakephp.Definition()},
		{"prestashop", prestashop.Definition()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.def.Name != tc.name {
				t.Errorf("Name = %q, want %q", tc.def.Name, tc.name)
			}
			if tc.def.Bootstrap == nil {
				t.Fatalf("%s Bootstrap should not be nil", tc.name)
			}
			// prestashop's bootstrapper legitimately returns an empty
			// command list (SupportsFreshInstall() is false) - confirmed
			// in Plan 1's golden snapshot, so don't assert len > 0 here.
			_ = tc.def.Bootstrap(bootstrap.Options{}).FreshCommands()
		})
	}
}

func TestNodeFrameworkDefinitions(t *testing.T) {
	cases := []struct {
		name string
		def  types.FrameworkDefinition
	}{
		{"nextjs", nextjs.Definition()},
		{"emdash", emdash.Definition()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.def.Name != tc.name {
				t.Errorf("Name = %q, want %q", tc.def.Name, tc.name)
			}
			if tc.def.Config.Runtime != "node" {
				t.Errorf("%s Config.Runtime = %q, want %q", tc.name, tc.def.Config.Runtime, "node")
			}
			if tc.def.Bootstrap == nil {
				t.Fatalf("%s Bootstrap should not be nil", tc.name)
			}
			if cmds := tc.def.Bootstrap(bootstrap.Options{}).FreshCommands(); len(cmds) == 0 {
				t.Errorf("%s Bootstrap factory should produce at least one fresh command", tc.name)
			}
		})
	}
}

func TestDjangoFrameworkDefinition(t *testing.T) {
	def := django.Definition()
	if def.Name != "django" {
		t.Errorf("Name = %q, want %q", def.Name, "django")
	}
	if def.Config.Runtime != "python" {
		t.Errorf("Config.Runtime = %q, want %q", def.Config.Runtime, "python")
	}
	if def.Bootstrap == nil {
		t.Fatal("django Bootstrap should not be nil")
	}
	if !def.SupportsFreshInstall {
		t.Error("expected SupportsFreshInstall to be true")
	}
	if !def.SupportsBootstrap {
		t.Error("expected SupportsBootstrap (clone workflow) to be true")
	}
	if cmds := def.Bootstrap(bootstrap.Options{}).FreshCommands(); len(cmds) == 0 {
		t.Error("django Bootstrap factory should produce at least one fresh command now that fresh-install is supported")
	}
}
