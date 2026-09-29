package desktop

import (
	"context"
	"sync"
	"time"
)

// operationWatcher polls operation events and emits operations:notification.
// It is registered as a Wails service only for its lifecycle; it has no
// methods the frontend can call.
type operationWatcher struct {
	platform Platform
	onEvent  func() // optional; the tray refresh hooks in here

	// pollInterval overrides the production poll cadence. Zero means "use
	// operationNotificationsPollInterval". It exists because the only honest way
	// to test a poller is to let it poll: a test that waits out the real
	// interval proves the same thing far more slowly, and a test that stubs the
	// clock out entirely proves nothing about the cadence.
	pollInterval time.Duration

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
	go watchOperationNotifications(watchCtx, w.platform, w.onEvent, w.interval())
}

// interval is the cadence the watcher actually polls at.
func (w *operationWatcher) interval() time.Duration {
	if w.pollInterval > 0 {
		return w.pollInterval
	}
	return operationNotificationsPollInterval
}

func (w *operationWatcher) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}
