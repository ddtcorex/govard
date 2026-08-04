package custom

import "govard/internal/frameworks/types"

// Spec declares Custom as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
