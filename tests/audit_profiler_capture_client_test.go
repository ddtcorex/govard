package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"govard/internal/cmd"
)

// Stock Magento only enables the CSV profiler when HTTP_ACCEPT contains
// "text/html" (app/bootstrap.php). The capture client must therefore send an
// explicit HTML Accept header; Go's net/http sends none by default.
func TestAuditProfilerCaptureClientSendsHTMLAcceptHeader(t *testing.T) {
	var accept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpGet := cmd.AuditProfilerHTTPGetForTest()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, err := httpGet(ctx, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(accept, "text/html") {
		t.Fatalf("Accept header = %q, want it to contain text/html so stock Magento enables the CSV profiler", accept)
	}
}
