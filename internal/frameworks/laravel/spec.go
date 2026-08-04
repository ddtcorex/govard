package laravel

import "govard/internal/frameworks/types"

// Spec declares Laravel as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
