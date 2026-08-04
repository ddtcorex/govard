package magento1

import "govard/internal/frameworks/types"

// Spec declares Magento 1 as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
