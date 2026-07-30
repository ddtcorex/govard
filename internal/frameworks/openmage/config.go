package openmage

import "govard/internal/frameworks/magento1"

// config is OpenMage's FrameworkConfig - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go. Built from magento1.BuildConfig,
// shared with Magento 1.
var config = magento1.BuildConfig("openmage", "openmage", "8.2", "latest")
