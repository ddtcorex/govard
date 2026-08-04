package prestashop

import "govard/internal/frameworks/types"

// Spec declares PrestaShop as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
