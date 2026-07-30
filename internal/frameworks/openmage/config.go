package openmage

import "govard/internal/engine/bootstrap"

// config is OpenMage's FrameworkConfig - registered into engine's
// runtime store by Definition() (via frameworks.Register), replacing the
// old static entry that used to live in
// internal/engine/framework_config.go. Built from
// bootstrap.BuildMagento1FamilyConfig, shared with Magento 1.
var config = bootstrap.BuildMagento1FamilyConfig("openmage", "openmage", "8.2", "latest")
