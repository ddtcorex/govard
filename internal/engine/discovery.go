package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type ProjectMetadata struct {
	Framework string
	Version   string
}

func DetectWebRoot(root string, framework string) string {
	return DetectFrameworkWebRoot(root, framework)
}

func DetectFramework(root string) ProjectMetadata {
	metadata := ProjectMetadata{Framework: "generic"}

	// Check composer.json
	composerPath := filepath.Join(root, "composer.json")
	if _, err := os.Stat(composerPath); err == nil {
		if require, ok := readComposerRequirements(composerPath); ok {
			frameworkMap := map[string]string{
				"magento/product-community-edition":            "magento2",
				"magento/product-enterprise-edition":           "magento2",
				"magento/framework":                            "magento2",
				"openmage/magento-lts":                         "magento1",
				"magento-hackathon/magento-composer-installer": "magento1",
				"laravel/framework":                            "laravel",
				"drupal/core":                                  "drupal",
				"symfony/framework-bundle":                     "symfony",
				"symfony/symfony":                              "symfony",
				"shopware/core":                                "shopware",
				"shopware/platform":                            "shopware",
				"cakephp/cakephp":                              "cakephp",
				"johnpbloch/wordpress":                         "wordpress",
				"roots/wordpress":                              "wordpress",
				"wordpress/wordpress":                          "wordpress",
			}

			for pkg, raw := range require {
				if fw, exists := frameworkMap[pkg]; exists {
					metadata.Framework = fw
					metadata.Version = dependencyVersionString(raw)
					return metadata
				}
			}
		}
	}

	// Check package.json
	packagePath := filepath.Join(root, "package.json")
	if _, err := os.Stat(packagePath); err == nil {
		if deps, ok := readPackageDependencies(packagePath); ok {
			if raw, ok := deps["emdash"]; ok {
				metadata.Framework = "emdash"
				metadata.Version = dependencyVersionString(raw)
				return metadata
			}
			if raw, ok := deps["next"]; ok {
				metadata.Framework = "nextjs"
				metadata.Version = dependencyVersionString(raw)
				return metadata
			}
		}
	}

	// Heuristic: auth.json with Magento repo credentials
	authPath := filepath.Join(root, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		data, _ := os.ReadFile(authPath)
		var auth map[string]interface{}
		if err := json.Unmarshal(data, &auth); err == nil {
			if basic, ok := auth["http-basic"].(map[string]interface{}); ok {
				if _, ok := basic["repo.magento.com"]; ok {
					metadata.Framework = "magento2"
					return metadata
				}
			}
		}
	}

	// Heuristic: Magento 1 files
	if _, err := os.Stat(filepath.Join(root, "app", "Mage.php")); err == nil {
		metadata.Framework = "magento1"
		return metadata
	}
	if _, err := os.Stat(filepath.Join(root, "app", "etc", "local.xml")); err == nil {
		metadata.Framework = "magento1"
		return metadata
	}

	// Heuristic: PrestaShop files. config/defines.inc.php is present in every
	// PrestaShop version (1.6 through 9.x) and PrestaShop isn't reliably a composer
	// "require" entry since it's the root package, not a dependency.
	if _, err := os.Stat(filepath.Join(root, "config", "defines.inc.php")); err == nil {
		metadata.Framework = "prestashop"
		return metadata
	}

	return metadata
}

func dependencyVersionString(raw interface{}) string {
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func readComposerRequirements(path string) (map[string]interface{}, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var composer struct {
		Require map[string]interface{} `json:"require"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return nil, false
	}
	if composer.Require == nil {
		return nil, false
	}
	return composer.Require, true
}

func readPackageDependencies(path string) (map[string]interface{}, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var pkg struct {
		Dependencies    map[string]interface{} `json:"dependencies"`
		DevDependencies map[string]interface{} `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, false
	}
	if pkg.Dependencies == nil && pkg.DevDependencies == nil {
		return nil, false
	}

	deps := make(map[string]interface{}, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for key, value := range pkg.Dependencies {
		deps[key] = value
	}
	for key, value := range pkg.DevDependencies {
		if _, exists := deps[key]; !exists {
			deps[key] = value
		}
	}
	return deps, true
}
