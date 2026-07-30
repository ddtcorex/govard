package magento2

import "govard/internal/engine/bootstrap"

// manifest is Magento 2's FrameworkManifestConfig - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "magento2" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object. Shared
// with Mage-OS (bootstrap.Magento2FamilyManifest) since the two
// distributions' sync/exclude/sensitive-table shape is identical.
var manifest = bootstrap.Magento2FamilyManifest
