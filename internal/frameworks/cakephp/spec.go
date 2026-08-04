package cakephp

import "govard/internal/frameworks/types"

// Spec declares CakePHP as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
