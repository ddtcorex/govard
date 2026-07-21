package tests

import (
	"testing"

	"govard/internal/engine"
	_ "govard/internal/frameworks" // trigger all.go's init(), which registers detection data
)

// expectedDetection mirrors, framework-by-framework, the exact literal data
// in the pre-migration internal/engine/discovery.go. Two entries preserve
// quirks that look like bugs but are NOT fixed here (see Task 2's comment
// block for why): "openmage/magento-lts" and
// "magento-hackathon/magento-composer-installer" both currently detect as
// "magento1", not "openmage" - and "openmage" itself has NO composer/file
// heuristic of its own today, so it is never auto-detected.
var expectedDetection = map[string]engine.DetectionSpec{
	"magento2": {
		ComposerPackages: []string{"magento/product-community-edition", "magento/product-enterprise-edition", "magento/framework"},
		AuthJSONHosts:    []string{"repo.magento.com"},
	},
	"magento1": {
		ComposerPackages: []string{"openmage/magento-lts", "magento-hackathon/magento-composer-installer"},
		FilePaths:        []string{"app/Mage.php", "app/etc/local.xml"},
	},
	"openmage": {},
	"laravel": {
		ComposerPackages: []string{"laravel/framework"},
	},
	"drupal": {
		ComposerPackages: []string{"drupal/core"},
	},
	"symfony": {
		ComposerPackages: []string{"symfony/framework-bundle", "symfony/symfony"},
	},
	"shopware": {
		ComposerPackages: []string{"shopware/core", "shopware/platform"},
	},
	"cakephp": {
		ComposerPackages: []string{"cakephp/cakephp"},
	},
	"wordpress": {
		ComposerPackages: []string{"johnpbloch/wordpress", "roots/wordpress", "wordpress/wordpress"},
	},
	"prestashop": {
		FilePaths: []string{"config/defines.inc.php"},
	},
	"nextjs": {
		PackageJSONDeps: []string{"next"},
	},
	"emdash": {
		PackageJSONDeps: []string{"emdash"},
	},
}

func TestDetectionRegistryPopulatedForAllTwelveFrameworks(t *testing.T) {
	for name, want := range expectedDetection {
		name, want := name, want
		t.Run(name, func(t *testing.T) {
			got, ok := engine.GetRegisteredDetectionForTest(name)
			if !ok {
				t.Fatalf("no detection spec registered for %q", name)
			}
			if !stringSlicesEqualUnordered(got.ComposerPackages, want.ComposerPackages) {
				t.Errorf("%s ComposerPackages = %v, want %v", name, got.ComposerPackages, want.ComposerPackages)
			}
			if !stringSlicesEqualUnordered(got.PackageJSONDeps, want.PackageJSONDeps) {
				t.Errorf("%s PackageJSONDeps = %v, want %v", name, got.PackageJSONDeps, want.PackageJSONDeps)
			}
			if !stringSlicesEqualUnordered(got.AuthJSONHosts, want.AuthJSONHosts) {
				t.Errorf("%s AuthJSONHosts = %v, want %v", name, got.AuthJSONHosts, want.AuthJSONHosts)
			}
			if !stringSlicesEqualUnordered(got.FilePaths, want.FilePaths) {
				t.Errorf("%s FilePaths = %v, want %v", name, got.FilePaths, want.FilePaths)
			}
		})
	}
}

func stringSlicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}
