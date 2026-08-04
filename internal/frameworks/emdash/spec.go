package emdash

import "govard/internal/frameworks/types"

// Spec declares Emdash as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
