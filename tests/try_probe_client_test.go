package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"govard/internal/try"
)

func TestProbeClientReachesTheProxyByAddressAndServerName(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "shop.test" {
			t.Errorf("Host = %q, want shop.test", r.Host)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := try.ProbeClient(srv.Listener.Addr().String(), "shop.test")

	resp, err := client.Get("https://shop.test/")
	if err != nil {
		t.Fatalf("probe request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
