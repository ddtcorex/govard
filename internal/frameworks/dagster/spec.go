package dagster

import "govard/internal/frameworks/types"

// Spec declares Dagster as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
