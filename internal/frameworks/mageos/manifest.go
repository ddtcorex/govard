package mageos

import "govard/internal/frameworks/magento2"

// manifest is Mage-OS's FrameworkManifestConfig - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old "mageos" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object. Shared
// with Magento 2 (internal/frameworks/magento2/manifest.go) since the two
// distributions' sync/exclude/sensitive-table shape is identical.
var manifest = magento2.Manifest
