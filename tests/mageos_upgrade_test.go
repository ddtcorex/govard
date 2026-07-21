package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestMageOSComposerCleanupUsesMageOSPrefixNotMagentoPrefix(t *testing.T) {
	current := map[string]interface{}{
		"require": map[string]interface{}{
			"mage-os/product-community-edition": "1.3.0",
			"mage-os/module-catalog":            "1.3.0",
			"third-party/unrelated":             "2.0.0",
		},
	}
	target := map[string]interface{}{
		"require": map[string]interface{}{
			"mage-os/product-community-edition": "1.3.1",
		},
	}

	engine.MergeComposerMapKeysWithPrefixForTest(current, target, "require", "mage-os/")

	requireMap := current["require"].(map[string]interface{})
	if _, ok := requireMap["mage-os/module-catalog"]; ok {
		t.Error("expected stale mage-os/module-catalog to be removed (not present in target)")
	}
	if v, ok := requireMap["mage-os/product-community-edition"]; !ok || v != "1.3.1" {
		t.Errorf("expected mage-os/product-community-edition to be updated to 1.3.1, got %v", v)
	}
	if _, ok := requireMap["third-party/unrelated"]; !ok {
		t.Error("expected third-party/unrelated (non mage-os/ package) to be preserved")
	}
}

func TestMagentoComposerCleanupStillUsesMagentoPrefix(t *testing.T) {
	current := map[string]interface{}{
		"require": map[string]interface{}{
			"magento/product-community-edition": "2.4.7",
			"magento/module-catalog":            "2.4.7",
		},
	}
	target := map[string]interface{}{
		"require": map[string]interface{}{
			"magento/product-community-edition": "2.4.8",
		},
	}

	engine.MergeComposerMapKeysWithPrefixForTest(current, target, "require", "magento/")

	requireMap := current["require"].(map[string]interface{})
	if _, ok := requireMap["magento/module-catalog"]; ok {
		t.Error("expected stale magento/module-catalog to be removed (not present in target)")
	}
}

func TestUpgradeDispatcherRoutesMageOSToMagentoFamilyUpgrade(t *testing.T) {
	// upgradeMagento2 (unlike the dispatcher's default "not implemented yet"
	// fallback, which prints a warning and returns nil) requires a non-empty
	// TargetVersion and returns a specific error when it's missing. That
	// error is a reliable, observable signal that "mageos" is actually
	// routed to the Magento-family upgrade function - not silently falling
	// through to the unimplemented-framework path, which would return nil
	// here instead of an error. This assertion is deliberately about the
	// error VALUE, not just err != nil, since both code paths could
	// otherwise look identical from a bare nil-check.
	err := engine.UpgradeFrameworkForTest(engine.Config{Framework: "mageos"}, engine.UpgradeOptions{})
	if err == nil {
		t.Fatal("expected an error because TargetVersion is empty - if mageos fell through to the dispatcher's 'not implemented' default case instead of upgradeMagento2, this would be nil")
	}
	if !containsSubstring(err.Error(), "target version is required") {
		t.Errorf("expected the magento-family upgrade's 'target version is required' error, got: %v", err)
	}
}
