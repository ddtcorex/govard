package frameworks

import (
	"govard/internal/frameworks/cakephp"
	"govard/internal/frameworks/django"
	"govard/internal/frameworks/drupal"
	"govard/internal/frameworks/emdash"
	"govard/internal/frameworks/laravel"
	"govard/internal/frameworks/magento1"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/mageos"
	"govard/internal/frameworks/nextjs"
	"govard/internal/frameworks/openmage"
	"govard/internal/frameworks/prestashop"
	"govard/internal/frameworks/shopware"
	"govard/internal/frameworks/symfony"
	"govard/internal/frameworks/wordpress"
)

// init registers every known framework's definition into the package-level
// default registry, in detection-priority order. This is the one place
// that must be edited to register a new framework (e.g. mageos, added in a
// later step of this initiative). Order matters for ambiguous-match
// resolution in engine.DetectFramework - in particular, emdash must
// register before nextjs, since the pre-registry detector checked the
// emdash dependency first and a project with both deps present must keep
// resolving to emdash.
func init() {
	Register(magento2.Definition())
	Register(mageos.Definition())
	Register(magento1.Definition())
	Register(openmage.Definition())
	Register(laravel.Definition())
	Register(symfony.Definition())
	Register(drupal.Definition())
	Register(wordpress.Definition())
	Register(emdash.Definition())
	Register(nextjs.Definition())
	Register(shopware.Definition())
	Register(cakephp.Definition())
	Register(prestashop.Definition())
	Register(django.Definition())
}
