package tests

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"govard/internal/try"
)

// newProxyStandIn serves TLS with a leaf for shop.test issued by ca, the way
// the Govard proxy serves a project domain from its local CA.
func newProxyStandIn(t *testing.T, ca testCA) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "shop.test" {
			t.Errorf("Host = %q, want shop.test", r.Host)
		}
		if r.TLS == nil || r.TLS.ServerName != "shop.test" {
			t.Errorf("SNI = %v, want shop.test", r.TLS)
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{ca.Leaf(t, "shop.test")}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return srv
}

func TestProbeClientReachesTheProxyByAddressAndServerName(t *testing.T) {
	ca := newTestCA(t)
	srv := newProxyStandIn(t, ca)

	client := try.ProbeClient(srv.Listener.Addr().String(), "shop.test", ca.Pool())

	resp, err := client.Get("https://shop.test/")
	if err != nil {
		t.Fatalf("probe request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestProbeClientRejectsACertificateFromAnUntrustedCA(t *testing.T) {
	srv := newProxyStandIn(t, newTestCA(t))

	for name, pool := range map[string]*x509.CertPool{
		"empty pool":     x509.NewCertPool(),
		"unrelated CA":   newTestCA(t).Pool(),
		"no pool at all": nil,
	} {
		t.Run(name, func(t *testing.T) {
			client := try.ProbeClient(srv.Listener.Addr().String(), "shop.test", pool)

			resp, err := client.Get("https://shop.test/")
			if err == nil {
				_ = resp.Body.Close()
				t.Fatal("probe succeeded against an untrusted certificate, want a verification error")
			}
			var unknown x509.UnknownAuthorityError
			if !errors.As(err, &unknown) {
				t.Fatalf("err = %v, want x509.UnknownAuthorityError", err)
			}
		})
	}
}

func TestProbeClientChecksTheServerName(t *testing.T) {
	ca := newTestCA(t)
	srv := newProxyStandIn(t, ca)

	client := try.ProbeClient(srv.Listener.Addr().String(), "other.test", ca.Pool())

	resp, err := client.Get("https://other.test/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("probe succeeded for a name the certificate does not cover, want a hostname error")
	}
	var hostname x509.HostnameError
	if !errors.As(err, &hostname) {
		t.Fatalf("err = %v, want x509.HostnameError", err)
	}
}
