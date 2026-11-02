package try

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// ProbeClient returns a client that reaches the Govard proxy on addr while
// presenting serverName as the TLS name, and verifies the proxy's certificate
// against roots only.
//
// The domain may not resolve in this process's resolver, so the client dials
// the loopback address directly with an explicit server name. The proxy's
// certificate is issued by the Govard local CA, which may not be in this
// process's system trust store, so the caller passes that CA as roots instead
// (engine.LocalRootCAPool). Verification, including the server name, is never
// skipped: a nil roots is treated as an empty pool rather than as "use the
// system roots", so nothing but the given CA can vouch for the proxy.
func ProbeClient(addr, serverName string, roots *x509.CertPool) *http.Client {
	if roots == nil {
		roots = x509.NewCertPool()
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}

	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
			TLSClientConfig: &tls.Config{
				ServerName: serverName,
				RootCAs:    roots,
				MinVersion: tls.VersionTLS12,
			},
		},
	}
}

// RootsLoader returns the CA pool the probe trusts. refresh asks for a fresh
// copy from the source of truth rather than a cached one.
type RootsLoader func(refresh bool) (*x509.CertPool, error)

// staleCAHint tells the user how to recover when even a freshly extracted CA
// does not verify the proxy certificate.
const staleCAHint = "the Govard proxy certificate is not signed by the Govard local CA; " +
	"restart the proxy and re-trust its CA with: govard svc restart"

// RefreshingProbeClient is ProbeClient with one recovery step for a stale CA.
//
// The exported CA can outlive the proxy's own: recreating the proxy's data
// volume makes Caddy mint a new one, and every probe would then fail with an
// unknown authority until the readiness timeout. On the first such failure the
// client reloads the roots with refresh set, rebuilds its transport and
// retries the request once. If the fresh roots still do not verify the proxy,
// that and every later unknown authority error carries a recovery hint. The
// client never falls back to skipping verification or to the system roots.
func RefreshingProbeClient(addr, serverName string, load RootsLoader) (*http.Client, error) {
	roots, err := load(false)
	if err != nil {
		return nil, err
	}

	base := ProbeClient(addr, serverName, roots)
	rt := &refreshingTransport{
		current: base.Transport,
		rebuild: func(roots *x509.CertPool) http.RoundTripper {
			return ProbeClient(addr, serverName, roots).Transport
		},
		load: load,
	}
	base.Transport = rt
	return base, nil
}

type refreshingTransport struct {
	mu        sync.Mutex
	current   http.RoundTripper
	refreshed bool
	rebuild   func(*x509.CertPool) http.RoundTripper
	load      RootsLoader
}

func (t *refreshingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	current, refreshed := t.current, t.refreshed
	t.mu.Unlock()

	resp, err := current.RoundTrip(req)
	if !isUnknownAuthority(err) {
		return resp, err
	}
	if refreshed {
		return nil, fmt.Errorf("%s: %w", staleCAHint, err)
	}

	next, err := t.refresh(current)
	if err != nil {
		return nil, err
	}
	// Readiness probes are bodiless GETs, so replaying the request is safe.
	resp, err = next.RoundTrip(req.Clone(req.Context()))
	if isUnknownAuthority(err) {
		return nil, fmt.Errorf("%s: %w", staleCAHint, err)
	}
	return resp, err
}

// refresh swaps in a transport built from freshly loaded roots, once.
func (t *refreshingTransport) refresh(stale http.RoundTripper) (http.RoundTripper, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.refreshed {
		return t.current, nil
	}

	roots, err := t.load(true)
	if err != nil {
		// Leave refreshed false: a transient export failure must not
		// permanently downgrade every later probe to the hint error.
		return nil, fmt.Errorf("%s: reload the CA: %w", staleCAHint, err)
	}
	t.refreshed = true
	if closer, ok := stale.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	t.current = t.rebuild(roots)
	return t.current, nil
}

func isUnknownAuthority(err error) bool {
	var unknown x509.UnknownAuthorityError
	return err != nil && errors.As(err, &unknown)
}
