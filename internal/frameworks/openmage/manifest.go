package openmage

import "govard/internal/frameworks/magento1"

// manifest is OpenMage's FrameworkManifestConfig - registered into
// engine's runtime store by Definition() (via frameworks.Register),
// replacing the old "openmage" entry that used to live in
// internal/engine/framework_manifest.json's "frameworks" object. Shared
// with Magento 1 (internal/frameworks/magento1/manifest.go) since
// OpenMage shares Magento 1's database schema and sync/exclude shape
// byte-for-byte.
var manifest = magento1.Manifest
