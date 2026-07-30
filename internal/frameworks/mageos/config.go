package mageos

import "govard/internal/frameworks/magento2"

// config is Mage-OS's FrameworkConfig - registered into engine's runtime
// store by Definition() (via frameworks.Register), replacing the old
// static entry that used to live in internal/engine/framework_config.go.
// Built from magento2.BuildConfig, shared with Magento 2.
var config = magento2.BuildConfig(Variant.Name, Variant.DBName, "8.4")
