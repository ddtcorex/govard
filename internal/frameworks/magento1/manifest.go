package magento1

import "govard/internal/engine/bootstrap"

// manifest is Magento 1's FrameworkManifestConfig - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "magento1" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object. Shared
// with OpenMage (bootstrap.Magento1FamilyManifest) since OpenMage shares
// Magento 1's database schema and sync/exclude shape byte-for-byte.
var manifest = bootstrap.Magento1FamilyManifest
