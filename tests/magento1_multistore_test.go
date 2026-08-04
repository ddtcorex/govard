package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"
)

func TestBuildMagento1CommandsStoreDomains(t *testing.T) {
	config := engine.Config{
		ProjectName: "testproject",
		Framework:   "magento1",
		Domain:      "main.test",
		StoreDomains: engine.StoreDomainMappings{
			"brand-b.test": {
				Code: "brand_b",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("testproject", config)

	foundWebsiteScopedSQL := false
	foundStoreScopedSQL := false

	for _, cmd := range commands {
		cmdStr := strings.Join(cmd.Args, " ")
		if strings.Contains(cmdStr, "bin/magento") {
			t.Fatalf("magento1 config auto should not plan bin/magento commands: %s", cmdStr)
		}
		if strings.Contains(cmdStr, "core_website") && strings.Contains(cmdStr, "brand_b") && strings.Contains(cmdStr, "brand-b.test") {
			foundWebsiteScopedSQL = true
		}
		if strings.Contains(cmdStr, "core_store") && strings.Contains(cmdStr, "brand_b") && strings.Contains(cmdStr, "brand-b.test") {
			foundStoreScopedSQL = true
		}
	}

	if !foundWebsiteScopedSQL {
		t.Fatal("expected Magento 1 config auto plan to include website-scoped base URL SQL")
	}
	if !foundStoreScopedSQL {
		t.Fatal("expected Magento 1 config auto plan to include store-scoped base URL SQL")
	}
}

func TestBuildMagento1CommandsTypedStoreDomains(t *testing.T) {
	config := engine.Config{
		ProjectName: "testproject",
		Framework:   "magento1",
		Domain:      "main.test",
		StoreDomains: engine.StoreDomainMappings{
			"brand-a.test": {
				Code: "brand_a",
				Type: "website",
			},
			"brand-b.test": {
				Code: "brand_b",
				Type: "store",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("testproject", config)

	foundWebsiteOnly := false
	foundStoreOnly := false
	foundWrongWebsiteScope := false
	foundWrongStoreScope := false

	for _, cmd := range commands {
		cmdStr := strings.Join(cmd.Args, " ")
		if strings.Contains(cmdStr, "core_website") && strings.Contains(cmdStr, "brand_a") && strings.Contains(cmdStr, "brand-a.test") {
			foundWebsiteOnly = true
		}
		if strings.Contains(cmdStr, "core_store") && strings.Contains(cmdStr, "brand_a") && strings.Contains(cmdStr, "brand-a.test") {
			foundWrongStoreScope = true
		}
		if strings.Contains(cmdStr, "core_store") && strings.Contains(cmdStr, "brand_b") && strings.Contains(cmdStr, "brand-b.test") {
			foundStoreOnly = true
		}
		if strings.Contains(cmdStr, "core_website") && strings.Contains(cmdStr, "brand_b") && strings.Contains(cmdStr, "brand-b.test") {
			foundWrongWebsiteScope = true
		}
	}

	if !foundWebsiteOnly {
		t.Fatal("expected website-typed Magento 1 mapping to emit website-scoped SQL")
	}
	if !foundStoreOnly {
		t.Fatal("expected store-typed Magento 1 mapping to emit store-scoped SQL")
	}
	if foundWrongStoreScope {
		t.Fatal("did not expect website-typed Magento 1 mapping to emit store-scoped SQL")
	}
	if foundWrongWebsiteScope {
		t.Fatal("did not expect store-typed Magento 1 mapping to emit website-scoped SQL")
	}
}

func TestBuildMagento1CommandsUseTablePrefix(t *testing.T) {
	config := engine.Config{
		ProjectName: "testproject",
		Framework:   "magento1",
		Domain:      "main.test",
		TablePrefix: "demo_",
		StoreDomains: engine.StoreDomainMappings{
			"brand-b.test": {
				Code: "brand_b",
				Type: "store",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("testproject", config)
	all := make([]string, 0, len(commands))
	for _, cmd := range commands {
		all = append(all, strings.Join(cmd.Args, " "))
	}
	joined := strings.Join(all, "\n")

	for _, expected := range []string{
		"demo_core_config_data",
		"demo_core_store",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected Magento 1 commands to contain %q, got:\n%s", expected, joined)
		}
	}
	if strings.Contains(joined, " core_config_data") || strings.Contains(joined, " core_store") {
		t.Fatalf("expected Magento 1 commands to use prefixed tables only, got:\n%s", joined)
	}
}
