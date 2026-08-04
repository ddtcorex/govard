package symfony

import "govard/internal/frameworks/types"

// Spec declares Symfony as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
