package nextjs

import "govard/internal/frameworks/types"

// Spec declares Next.js as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
