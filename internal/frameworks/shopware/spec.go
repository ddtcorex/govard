package shopware

import "govard/internal/frameworks/types"

// Spec declares Shopware as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
