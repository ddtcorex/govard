package mageos

import "govard/internal/engine/bootstrap"

// config is Mage-OS's FrameworkConfig - registered into engine's runtime
// store by Definition() (via frameworks.Register), replacing the old
// static entry that used to live in internal/engine/framework_config.go.
// Built from bootstrap.BuildMagento2FamilyConfig, shared with Magento 2.
var config = bootstrap.BuildMagento2FamilyConfig("mageos", "mageos", "8.4")
