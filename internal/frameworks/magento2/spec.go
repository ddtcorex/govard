package magento2

import "govard/internal/frameworks/types"

// Spec declares Magento 2 as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
