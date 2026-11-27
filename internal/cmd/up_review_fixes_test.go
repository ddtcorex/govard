package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"govard/internal/engine"
)

// A container that shows `exited` on the first inspection is kickstarted and
// given the abort window; it must not be failed on the very next poll. The poll
// cadence is left at its production default on purpose: setting it to 1 is what
// let the stale-state double count pass review once.
func TestUpReadinessToleratesAnExitedInspectionAtTheDefaultCadence(t *testing.T) {
	var (
		mu    sync.Mutex
		seen  = map[string]int{}
		kicks int
	)
	stubUpReadiness(t, func(string, []string) error { return nil }, func(container string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		seen[container]++
		if seen[container] == 1 {
			return "exited|1|false||", nil
		}
		return "running|0|false||", nil
	})
	upReadinessContainerStart = func(string) {
		mu.Lock()
		kicks++
		mu.Unlock()
	}

	if upReadinessStatePollEvery < 2 {
		t.Fatalf("this test pins the production cadence, got %d", upReadinessStatePollEvery)
	}
	if err := waitForUpRuntimeReadiness(t.TempDir(), upReadinessTestConfig(), nil, 30*time.Second); err != nil {
		t.Fatalf("a container that exited once and then ran was failed: %v", err)
	}
	if kicks != 3 {
		t.Fatalf("%d kickstarts, want one per check", kicks)
	}
}

// A container that stays exited is still a failure, and it is reported after the
// kickstart's re-inspection, not silently retried forever.
func TestUpReadinessStillFailsAContainerThatStaysExited(t *testing.T) {
	stubUpReadiness(t, func(string, []string) error { return nil }, func(string) (string, error) {
		return "exited|1|false||", nil
	})

	err := waitForUpRuntimeReadiness(t.TempDir(), upReadinessTestConfig(), nil, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("want an error naming the exited container, got %v", err)
	}
}

// With the Govard CA unavailable the probe falls back to plain HTTP, where the
// proxy answers with a redirect to HTTPS. The redirect is the answer: following
// it lands on a certificate this fallback has no CA for and fails.
func TestUpStorefrontProbeTreatsARedirectAsAnAnswer(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer tlsServer.Close()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, tlsServer.URL+"/", http.StatusPermanentRedirect)
	}))
	defer plain.Close()

	original := upStorefrontRootCAs
	upStorefrontRootCAs = func() (*x509.CertPool, error) { return nil, errors.New("no Govard CA") }
	t.Cleanup(func() { upStorefrontRootCAs = original })

	host := strings.TrimPrefix(plain.URL, "http://")
	if err := upStorefrontProbe("https://" + host + "/"); err != nil {
		t.Fatalf("a redirect from the proxy is an answer, got %v", err)
	}
}

// `env up redis` starts one service; the project's own domain is not part of it,
// so waiting for it would only burn the timeout.
func TestUpStorefrontReadinessIsSkippedForAPartialStart(t *testing.T) {
	stubStorefrontProbe(t, func(string) error {
		t.Fatal("the storefront was probed during a partial start")
		return nil
	})
	if err := waitForUpStorefrontReadiness(engine.Config{Domain: "sample-project.test"}, []string{"redis"}); err != nil {
		t.Fatalf("a partial start must not wait for the storefront: %v", err)
	}
}

func certVerificationError() error {
	return &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
}

// The exported root CA goes stale when the proxy's data volume is recreated and
// Caddy mints a new one. Every probe then fails verification, so the wait would
// run its whole timeout. It refreshes the CA once and retries; if the
// certificate is still untrusted it stops at once instead of waiting.
func TestUpStorefrontReadinessRefreshesAStaleCAOnceThenStops(t *testing.T) {
	var probes, refreshes int32
	stubStorefrontProbe(t, func(string) error {
		atomic.AddInt32(&probes, 1)
		return certVerificationError()
	})
	upStorefrontTimeout = time.Hour
	originalRefresh := upStorefrontRefreshCA
	upStorefrontRefreshCA = func() error { atomic.AddInt32(&refreshes, 1); return nil }
	t.Cleanup(func() { upStorefrontRefreshCA = originalRefresh })

	err := waitForUpStorefrontReadiness(engine.Config{Domain: "sample-project.test"}, nil)
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("want an error about the untrusted certificate, got %v", err)
	}
	if refreshes != 1 {
		t.Fatalf("refreshed the CA %d times, want once", refreshes)
	}
	if probes != 2 {
		t.Fatalf("probed %d times, want 2 (before and after the refresh)", probes)
	}
}

func TestUpStorefrontReadinessSucceedsOnceTheRefreshedCAIsTrusted(t *testing.T) {
	var refreshed int32
	stubStorefrontProbe(t, func(string) error {
		if atomic.LoadInt32(&refreshed) == 0 {
			return certVerificationError()
		}
		return nil
	})
	originalRefresh := upStorefrontRefreshCA
	upStorefrontRefreshCA = func() error { atomic.StoreInt32(&refreshed, 1); return nil }
	t.Cleanup(func() { upStorefrontRefreshCA = originalRefresh })

	if err := waitForUpStorefrontReadiness(engine.Config{Domain: "sample-project.test"}, nil); err != nil {
		t.Fatalf("the refreshed CA should have fixed the probe: %v", err)
	}
}
