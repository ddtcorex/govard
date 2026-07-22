package frameworks

import (
	"strings"

	"govard/internal/engine/tunnel"
)

// NewBaseURLManager resolves framework's registered BaseURLManager
// factory, falling back to tunnel.NoopManager for frameworks that don't
// need specialized base-URL rewriting.
func NewBaseURLManager(framework string) tunnel.BaseURLManager {
	def, ok := Get(strings.TrimSpace(framework))
	if !ok || def.BaseURLManager == nil {
		return &tunnel.NoopManager{}
	}
	return def.BaseURLManager()
}
