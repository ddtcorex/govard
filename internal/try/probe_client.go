package try

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// ProbeClient returns a client that reaches the Govard proxy on addr while
// presenting serverName as the TLS name.
//
// Readiness is a local liveness question, not a trust question: the proxy's
// certificate is the local CA, which may or may not be in this process's trust
// store, and the domain may not resolve in this process's resolver. Dialing the
// loopback address directly with an explicit server name answers "is the app
// up?" without depending on either. The client is never used for anything but
// this probe, and certificate verification is intentionally skipped for it.
func ProbeClient(addr, serverName string) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}

	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
			//nolint:gosec // liveness probe for the local proxy, see the doc comment
			TLSClientConfig: &tls.Config{ServerName: serverName, InsecureSkipVerify: true},
		},
	}
}
