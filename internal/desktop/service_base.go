package desktop

import "context"

// serviceBase is embedded in every bound service. ctx is the lifecycle context
// used only for cancellation (docker log streams, background syncs, engine
// calls); it is never a handle to the GUI runtime. platform is.
type serviceBase struct {
	ctx      context.Context
	platform Platform
}

// lifecycleContext never returns nil: calls made before startup (tests, and the
// window between construction and ServiceStartup) get a background context.
func (b *serviceBase) lifecycleContext() context.Context {
	if b.ctx == nil {
		return context.Background()
	}
	return b.ctx
}
