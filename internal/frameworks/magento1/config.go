package magento1

import "govard/internal/engine/bootstrap"

// config is Magento 1's FrameworkConfig - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go. Built from
// bootstrap.BuildMagento1FamilyConfig, shared with OpenMage.
var config = bootstrap.BuildMagento1FamilyConfig("magento1", "magento", "8.1", "2.2")
