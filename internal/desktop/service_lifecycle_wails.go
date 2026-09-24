//go:build desktop

package desktop

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ServiceStartup is promoted into every service that embeds serviceBase; Wails
// v3 calls it with a context cancelled at shutdown. It lives in a
// desktop-tagged file because application.ServiceOptions is part of the GUI
// runtime the untagged CLI must not link.
func (b *serviceBase) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	b.ctx = ctx
	return nil
}

func (w *operationWatcher) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	w.start(ctx)
	return nil
}

func (w *operationWatcher) ServiceShutdown() error {
	w.stop()
	return nil
}
