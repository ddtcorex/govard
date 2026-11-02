package try

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
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
