package tests

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

// rootsSequence is a probe roots loader: the first load returns initial, every
// refresh returns refreshed and is counted.
type rootsSequence struct {
	initial, refreshed *x509.CertPool
	refreshes          int
}

func (s *rootsSequence) load(refresh bool) (*x509.CertPool, error) {
	if refresh {
		s.refreshes++
		return s.refreshed, nil
	}
	return s.initial, nil
}

func TestRefreshingProbeClientReExportsAStaleCAOnceAndSucceeds(t *testing.T) {
	ca := newTestCA(t)
	srv := newProxyStandIn(t, ca)
	roots := &rootsSequence{initial: newTestCA(t).Pool(), refreshed: ca.Pool()}

	client, err := try.RefreshingProbeClient(srv.Listener.Addr().String(), "shop.test", roots.load)
	if err != nil {
		t.Fatalf("RefreshingProbeClient: %v", err)
	}

	for i := 0; i < 2; i++ {
		resp, err := client.Get("https://shop.test/")
		if err != nil {
			t.Fatalf("probe %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("probe %d status = %d, want 200", i, resp.StatusCode)
		}
	}
	if roots.refreshes != 1 {
		t.Fatalf("refreshes = %d, want exactly 1", roots.refreshes)
	}
}

func TestRefreshingProbeClientFailsWithAHintWhenTheFreshCAIsStillWrong(t *testing.T) {
	srv := newProxyStandIn(t, newTestCA(t))
	roots := &rootsSequence{initial: newTestCA(t).Pool(), refreshed: newTestCA(t).Pool()}

	client, err := try.RefreshingProbeClient(srv.Listener.Addr().String(), "shop.test", roots.load)
	if err != nil {
		t.Fatalf("RefreshingProbeClient: %v", err)
	}

	for i := 0; i < 2; i++ {
		resp, err := client.Get("https://shop.test/")
		if err == nil {
			_ = resp.Body.Close()
			t.Fatalf("probe %d succeeded against an untrusted certificate", i)
		}
		var unknown x509.UnknownAuthorityError
		if !errors.As(err, &unknown) {
			t.Fatalf("probe %d err = %v, want x509.UnknownAuthorityError", i, err)
		}
		if !strings.Contains(err.Error(), "govard svc restart") {
			t.Fatalf("probe %d err = %v, want a hint naming govard svc restart", i, err)
		}
	}
	if roots.refreshes != 1 {
		t.Fatalf("refreshes = %d, want exactly 1", roots.refreshes)
	}
}

func TestRefreshingProbeClientNeverRefreshesForOtherErrors(t *testing.T) {
	ca := newTestCA(t)
	srv := newProxyStandIn(t, ca)
	roots := &rootsSequence{initial: ca.Pool(), refreshed: ca.Pool()}

	client, err := try.RefreshingProbeClient(srv.Listener.Addr().String(), "other.test", roots.load)
	if err != nil {
		t.Fatalf("RefreshingProbeClient: %v", err)
	}
	resp, err := client.Get("https://other.test/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("probe succeeded for a name the certificate does not cover")
	}
	if roots.refreshes != 0 {
		t.Fatalf("refreshes = %d, want 0 for a hostname mismatch", roots.refreshes)
	}
}
