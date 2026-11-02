package try

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrNotReady marks a timeout: the stack answered, with something other than
// a ready status, for the whole wait.
var ErrNotReady = errors.New("origin did not become ready")

// WaitReady polls origin until the stack answers with a 2xx or 3xx status.
//
// A 502 or a refused connection means the proxy is up but the app is not, which
// is the normal state during a first boot. The caller's context wins over the
// timeout, so Ctrl-C during the wait returns promptly instead of at the deadline.
func WaitReady(ctx context.Context, client *http.Client, o Origin, timeout, interval time.Duration) error {
	if client == nil {
		client = http.DefaultClient
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	deadline := time.Now().Add(timeout)
	var last string

	for {
		status, err := probeOnce(ctx, client, o)
		if err == nil && status >= http.StatusOK && status < http.StatusBadRequest {
			return nil
		}
		if err != nil {
			last = err.Error()
		} else {
			last = fmt.Sprintf("status %d", status)
		}

		if ctx.Err() != nil {
			return fmt.Errorf("wait for %s: %w (last %s)", o.URL, ctx.Err(), last)
		}
		if time.Now().Add(interval).After(deadline) {
			return fmt.Errorf("%w: timed out after %s (last %s)", ErrNotReady, timeout, last)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %s: %w (last %s)", o.URL, ctx.Err(), last)
		case <-time.After(interval):
		}
	}
}

// probeOnce issues one readiness request and returns the status code.
func probeOnce(ctx context.Context, client *http.Client, o Origin) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("build readiness request: %w", err)
	}
	req.Host = o.HostHeader

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}
