package frameworks

import (
	"fmt"
	"strings"

	"govard/internal/deploy"
	"govard/internal/engine/bootstrap"
)

// DeployRecipe returns the deploy recipe this framework registered. The
// command layer calls it and hands the result to internal/deploy, which must
// never import the framework registry itself (that would close an import cycle
// through the framework types).
func DeployRecipe(framework string) (deploy.Recipe, bool) {
	def, ok := Get(strings.TrimSpace(framework))
	if !ok || def.DeployRecipe == nil {
		return deploy.Recipe{}, false
	}
	return def.DeployRecipe(), true
}

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
