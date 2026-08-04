package tests

import (
	"context"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"
)

func TestMagentoOperationalHooksAreInheritedByMageOS(t *testing.T) {
	magento, ok := frameworks.Get("magento2")
	if !ok || magento.PostSync == nil || magento.UnblockSearchIndex == nil || magento.BuildSearchHostFixSQL == nil || magento.RenderBootstrapEnvironment == nil || magento.ComposerAuth.Repository == "" {
		t.Fatal("expected Magento 2 to own operational and bootstrap-environment hooks")
	}
	if len(magento.DefaultChownDirectories) == 0 {
		t.Fatal("expected Magento 2 to own its default chown directories")
	}
	if magento.MinimumBootstrapVersion == "" || magento.DefaultFreshMetaPackage == "" {
		t.Fatal("expected Magento 2 to own fresh-install version and package defaults")
	}
	if magento.DetectLocalAdminMetadata == nil || magento.BuildLocalAdminSettingsQuery == nil || magento.ResolveLocalAdminURL == nil {
		t.Fatal("expected Magento 2 to own local admin metadata, query, and URL policy")
	}

	mageOS, ok := frameworks.Get("mageos")
	if !ok || mageOS.PostSync == nil || mageOS.UnblockSearchIndex == nil || mageOS.BuildSearchHostFixSQL == nil || mageOS.RenderBootstrapEnvironment == nil {
		t.Fatal("expected Mage-OS to inherit Magento operational hooks")
	}
	if len(mageOS.DefaultChownDirectories) == 0 {
		t.Fatal("expected Mage-OS to inherit Magento default chown directories")
	}
	if mageOS.DefaultFreshMetaPackage != "mage-os/project-community-edition" {
		t.Fatalf("expected Mage-OS package override, got %q", mageOS.DefaultFreshMetaPackage)
	}
}

func TestUpgradeRegistryUsesRegisteredFrameworkAliases(t *testing.T) {
	engine.RegisterFrameworkAlias("upgrade-fixture", "upgrade-canonical")
	engine.RegisterUpgrader("upgrade-canonical", func(context.Context, engine.Config, engine.UpgradeOptions) error {
		return nil
	})

	if _, ok := engine.GetUpgrader("upgrade-fixture"); !ok {
		t.Fatal("expected upgrader lookup to use the registered alias")
	}
}

func TestFrameworkMigrationTypesAreOwnedByDefinitions(t *testing.T) {
	magento, ok := frameworks.Get("magento2")
	if !ok || len(magento.MigrationTypes.DDEV) == 0 || len(magento.MigrationTypes.Warden) == 0 {
		t.Fatal("expected Magento 2 to declare its DDEV and Warden migration types")
	}

	wordpress, ok := frameworks.Get("wordpress")
	if !ok || len(wordpress.MigrationTypes.DDEV) == 0 || len(wordpress.MigrationTypes.Warden) == 0 {
		t.Fatal("expected WordPress to declare its DDEV and Warden migration types")
	}
}

func TestFrameworkRuntimeImageAndTemplatePoliciesAreOwnedByDefinitions(t *testing.T) {
	emdash, ok := frameworks.Get("emdash")
	if !ok || emdash.NodeImageFlavor != "standard" {
		t.Fatal("expected Emdash to own its standard Node image policy")
	}

	mageOS, ok := frameworks.Get("mageos")
	if !ok || mageOS.VarnishTemplateFramework != "magento2" {
		t.Fatal("expected Mage-OS to own its Magento 2 Varnish-template inheritance")
	}
}

func TestFrameworkPostCloneErrorPoliciesAreOwnedByDefinitions(t *testing.T) {
	wordpress, ok := frameworks.Get("wordpress")
	if !ok || wordpress.IgnorePostCloneError == nil {
		t.Fatal("expected WordPress to own its post-clone error policy")
	}

	prestashop, ok := frameworks.Get("prestashop")
	if !ok || prestashop.IgnorePostCloneError == nil {
		t.Fatal("expected PrestaShop to own its post-clone error policy")
	}
}
