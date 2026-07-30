package magento2

import "govard/internal/engine/bootstrap"

// config is Magento 2's FrameworkConfig - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go. Built from
// bootstrap.BuildMagento2FamilyConfig, shared with Mage-OS.
var config = bootstrap.BuildMagento2FamilyConfig("magento2", "magento", "8.5")
