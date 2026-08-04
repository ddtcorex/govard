package drupal

import "govard/internal/frameworks/types"

// Spec declares Drupal as a root framework.
func Spec() types.FrameworkSpec { return types.FrameworkSpec{Definition: Definition()} }
