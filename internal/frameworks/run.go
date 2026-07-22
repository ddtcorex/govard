package frameworks

import (
	"fmt"
	"strings"

	"govard/internal/engine/bootstrap"
)

// RunBootstrap dispatches to framework's registered Bootstrap factory
// instead of a per-framework switch, so adding a framework here doesn't
// require touching a separate dispatch table.
func RunBootstrap(framework string, opts bootstrap.Options) error {
	def, ok := Get(strings.TrimSpace(framework))
	if !ok || def.Bootstrap == nil {
		return fmt.Errorf("unsupported framework: %s", framework)
	}
	_ = def.Bootstrap(opts).FreshCommands()
	return nil
}
