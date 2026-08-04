package wordpress

import "govard/internal/frameworks/types"

// Spec declares WordPress as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
