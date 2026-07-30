package mageos

import "govard/internal/engine/bootstrap"

// manifest is Mage-OS's FrameworkManifestConfig - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old "mageos" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object. Shared
// with Magento 2 (bootstrap.Magento2FamilyManifest) since the two
// distributions' sync/exclude/sensitive-table shape is identical.
var manifest = bootstrap.Magento2FamilyManifest
