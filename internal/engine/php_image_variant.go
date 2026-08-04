package engine

import "strings"

// phpImageVariantsByFramework maps a framework name to the Docker image
// variant suffix its PHP container needs (e.g. "magento1", "magento2");
// frameworks with no entry (or an empty variant) use the plain "php" image.
var phpImageVariantsByFramework = map[string]string{}

// RegisterPHPImageVariant registers variant as the PHP image suffix for
// framework, keyed the same way PHPImageVariantForFramework looks it up.
// Called from frameworks.Register (alongside RegisterDetection/
// RegisterFrameworkConfig/RegisterFrameworkManifest) so a framework package
// can declare its own image variant instead of a literal case in
// RequiredRuntimeImages/local_image_fallback.go's switch. A blank variant
// is a no-op (nothing to register). Not safe for concurrent calls; intended
// usage is registration during package init(), before RequiredRuntimeImages
// is ever called.
func RegisterPHPImageVariant(framework string, variant string) {
	framework = strings.ToLower(strings.TrimSpace(framework))
	variant = strings.TrimSpace(variant)
	if variant == "" {
		return
	}
	phpImageVariantsByFramework[framework] = variant
}

// PHPImageVariantForFramework returns the registered PHP image variant
// suffix for framework (e.g. "magento1"), or "" if framework uses the plain
// "php" image.
func PHPImageVariantForFramework(framework string) string {
	return phpImageVariantsByFramework[strings.ToLower(strings.TrimSpace(framework))]
}

// registeredPHPImageVariants returns the distinct set of registered PHP
// image variant suffixes (e.g. {"magento1", "magento2"}), used by
// local_image_fallback.go to recognize a "php-<variant>" service name
// generically instead of one literal case per variant.
func registeredPHPImageVariants() []string {
	seen := make(map[string]struct{}, len(phpImageVariantsByFramework))
	variants := make([]string, 0, len(phpImageVariantsByFramework))
	for _, v := range phpImageVariantsByFramework {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		variants = append(variants, v)
	}
	return variants
}

// dbDriverCategoriesByFramework maps a framework name to its phpMyAdmin
// DB-driver category label (e.g. "magento"); frameworks with no entry fall
// back to "app" in the generated PHP (see buildPMAConfigContent).
var dbDriverCategoriesByFramework = map[string]string{}

// RegisterDBDriverCategory registers category as the phpMyAdmin DB-driver
// category for framework, keyed the same way DBDriverCategoryForFramework
// looks it up. Called from frameworks.Register (alongside
// RegisterDetection/RegisterFrameworkConfig/RegisterFrameworkManifest/
// RegisterPHPImageVariant) so a framework package can declare its own
// category instead of a literal entry in proxy.go's $dbMap PHP array. A
// blank category is a no-op (nothing to register). Not safe for concurrent
// calls; intended usage is registration during package init(), before
// buildPMAConfigContent is ever called.
func RegisterDBDriverCategory(framework string, category string) {
	framework = strings.ToLower(strings.TrimSpace(framework))
	category = strings.TrimSpace(category)
	if category == "" {
		return
	}
	dbDriverCategoriesByFramework[framework] = category
}

// DBDriverCategoryForFramework returns the registered phpMyAdmin DB-driver
// category for framework, or "" if it has none (the generated PHP treats
// that the same as an absent $dbMap entry - falls back to "app").
func DBDriverCategoryForFramework(framework string) string {
	return dbDriverCategoriesByFramework[strings.ToLower(strings.TrimSpace(framework))]
}
