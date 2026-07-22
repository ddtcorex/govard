package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestMagentoDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"magento/product-community-edition": "2.4.7",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento2" {
		t.Errorf("Expected framework magento2, got %s", metadata.Framework)
	}
}

func TestComposerDetectionUsesFrameworkPriority(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"magento/product-community-edition": "2.4.8",
			"mage-os/project-community-edition": "1.3.1",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento2" {
		t.Errorf("Expected Magento 2 to win the documented detection priority, got %s", metadata.Framework)
	}
}

func TestMagento1Discovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"app/Mage.php": "",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento1" {
		t.Errorf("Expected framework magento1, got %s", metadata.Framework)
	}
}

func TestPrestaShopDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"config/defines.inc.php": "<?php\ndefine('_PS_VERSION_', '8.1.5');\n",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "prestashop" {
		t.Errorf("Expected framework prestashop, got %s", metadata.Framework)
	}
}

func TestLaravelDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"laravel/framework": "11.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "laravel" {
		t.Errorf("Expected framework laravel, got %s", metadata.Framework)
	}
	if metadata.Version != "11.0.0" {
		t.Errorf("Expected version 11.0.0, got %s", metadata.Version)
	}
}

func TestNextjsDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"package.json": packageJSON(t, map[string]string{
			"next": "14.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "nextjs" {
		t.Errorf("Expected framework nextjs, got %s", metadata.Framework)
	}
	if metadata.Version != "14.0.0" {
		t.Errorf("Expected version 14.0.0, got %s", metadata.Version)
	}
}

func TestNextjsDiscoveryWithMalformedComposerFallback(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": `{"name":"broken","require":{invalid json`,
		"package.json": packageJSON(t, map[string]string{
			"next": "14.2.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "nextjs" {
		t.Errorf("Expected framework nextjs, got %s", metadata.Framework)
	}
	if metadata.Version != "14.2.0" {
		t.Errorf("Expected version 14.2.0, got %s", metadata.Version)
	}
}

func TestEmdashDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"package.json": packageJSON(t, map[string]string{
			"astro":  "^6.1.2",
			"emdash": "^0.1.0",
		}),
		"astro.config.mjs": `import emdash from "emdash/astro";`,
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "emdash" {
		t.Errorf("Expected framework emdash, got %s", metadata.Framework)
	}
	if metadata.Version != "^0.1.0" {
		t.Errorf("Expected version ^0.1.0, got %s", metadata.Version)
	}
}

func TestEmdashTakesPrecedenceOverNextJS(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"package.json": packageJSON(t, map[string]string{
			"emdash": "^0.1.0",
			"next":   "15.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "emdash" {
		t.Errorf("Expected framework emdash to retain its legacy priority over nextjs, got %s", metadata.Framework)
	}
}

func TestDrupalDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"drupal/core": "10.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "drupal" {
		t.Errorf("Expected framework drupal, got %s", metadata.Framework)
	}
}

func TestSymfonyDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"symfony/framework-bundle": "7.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "symfony" {
		t.Errorf("Expected framework symfony, got %s", metadata.Framework)
	}
}

func TestShopwareDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"shopware/core": "6.6.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "shopware" {
		t.Errorf("Expected framework shopware, got %s", metadata.Framework)
	}
}

func TestCakephpDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"cakephp/cakephp": "5.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "cakephp" {
		t.Errorf("Expected framework cakephp, got %s", metadata.Framework)
	}
}

func TestWordpressDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"johnpbloch/wordpress": "6.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "wordpress" {
		t.Errorf("Expected framework wordpress, got %s", metadata.Framework)
	}
}

func TestOpenMagePackageDetectedAsMagento1(t *testing.T) {
	// This looks surprising but is today's real behavior: openmage/magento-lts
	// maps to "magento1", not "openmage" - openmage has no detection
	// heuristic of its own. Locking this in so Task 2's rewrite can't
	// accidentally "fix" it.
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"openmage/magento-lts": "20.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento1" {
		t.Errorf("Expected framework magento1 (current quirk), got %s", metadata.Framework)
	}
}

func TestMagentoHackathonPackageDetectedAsMagento1(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"magento-hackathon/magento-composer-installer": "3.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento1" {
		t.Errorf("Expected framework magento1, got %s", metadata.Framework)
	}
}

func TestMagento1LocalXMLDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"app/etc/local.xml": "<config></config>",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento1" {
		t.Errorf("Expected framework magento1, got %s", metadata.Framework)
	}
}

func TestMagento2AuthJSONDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"auth.json": `{"http-basic":{"repo.magento.com":{"username":"u","password":"p"}}}`,
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "magento2" {
		t.Errorf("Expected framework magento2, got %s", metadata.Framework)
	}
}

func tempProject(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", rel, err)
		}
	}
	return dir
}

func composerJSON(t *testing.T, require map[string]string) string {
	t.Helper()
	payload := map[string]map[string]string{"require": require}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to build composer.json: %v", err)
	}
	return string(data)
}

func packageJSON(t *testing.T, deps map[string]string) string {
	t.Helper()
	payload := map[string]map[string]string{"dependencies": deps}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to build package.json: %v", err)
	}
	return string(data)
}
