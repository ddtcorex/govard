package desktop

import (
	"context"
	"sync"
)

// operationWatcher polls operation events and emits operations:notification.
// It is registered as a Wails service only for its lifecycle; it has no
// methods the frontend can call.
type operationWatcher struct {
	platform Platform
	onEvent  func() // optional; the tray refresh hooks in here

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (w *operationWatcher) start(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
	watchCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go watchOperationNotifications(watchCtx, w.platform, w.onEvent)
}

func (w *operationWatcher) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}
