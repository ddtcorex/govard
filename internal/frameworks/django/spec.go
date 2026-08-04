package django

import "govard/internal/frameworks/types"

// Spec declares Django as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
