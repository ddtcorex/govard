package cmd

import (
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"govard/internal/engine"
	"govard/internal/updater"
)

func stubStorefrontProbe(t *testing.T, probe func(string) error) {
	t.Helper()
	originalProbe := upStorefrontProbe
	originalInterval := upStorefrontProbeInterval
	originalTimeout := upStorefrontTimeout
	originalSleep := upReadinessSleep
	t.Cleanup(func() {
		upStorefrontProbe = originalProbe
		upStorefrontProbeInterval = originalInterval
		upStorefrontTimeout = originalTimeout
		upReadinessSleep = originalSleep
	})
	upStorefrontProbe = probe
	upStorefrontProbeInterval = 10 * time.Millisecond
	upReadinessSleep = func(time.Duration) {}
}

func TestUpStorefrontReadinessWaitsForTheFirstAnswer(t *testing.T) {
	var calls int32
	stubStorefrontProbe(t, func(url string) error {
		if url != "https://sample-project.test/" {
			t.Fatalf("probed %q, want the project domain", url)
		}
		if atomic.AddInt32(&calls, 1) < 3 {
			return errors.New("dial tcp: connection refused")
		}
		return nil
	})

	if err := waitForUpStorefrontReadiness(engine.Config{Domain: "sample-project.test"}); err != nil {
		t.Fatalf("waitForUpStorefrontReadiness: %v", err)
	}
	if calls != 3 {
		t.Fatalf("probed %d times, want 3", calls)
	}
}

func TestUpStorefrontReadinessIsBoundedAndNonFatal(t *testing.T) {
	stubStorefrontProbe(t, func(string) error { return errors.New("storefront answered HTTP 503") })
	upStorefrontTimeout = 50 * time.Millisecond

	err := waitForUpStorefrontReadiness(engine.Config{Domain: "sample-project.test"})
	if err == nil {
		t.Fatal("readiness reported success although the storefront never answered")
	}
	if !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("error %q does not describe the timeout", err)
	}
}

func TestUpStorefrontReadinessSkipsProjectsWithoutADomain(t *testing.T) {
	probed := false
	stubStorefrontProbe(t, func(string) error { probed = true; return nil })

	if err := waitForUpStorefrontReadiness(engine.Config{}); err != nil {
		t.Fatalf("waitForUpStorefrontReadiness: %v", err)
	}
	if probed {
		t.Fatal("probed a project that has no domain")
	}
}

// TestStartUpUpdateCheckIsSilentForJSONOutput pins the --error-json contract: the
// machine-readable mode must not gain a background notice on stdout.
func TestStartUpUpdateCheckIsSilentForJSONOutput(t *testing.T) {
	notifications := int32(0)
	restore := updater.SetUpdateCheckNotifierForTest(func(string, string) { atomic.AddInt32(&notifications, 1) })
	defer restore()

	finish := startUpUpdateCheck(true)
	finish()

	if got := atomic.LoadInt32(&notifications); got != 0 {
		t.Fatalf("%d notifications in JSON mode, want none", got)
	}
}

// TestStartUpUpdateCheckStillRunsForHumanOutput is the other half of the branch:
// suppressing the notice for JSON must not disable the check itself.
func TestStartUpUpdateCheckStillRunsForHumanOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v99.0.0","body":"notes"}`))
	}))
	defer server.Close()
	t.Setenv("GOVARD_UPDATE_CHECK_URL", server.URL)
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	notifications := int32(0)
	restore := updater.SetUpdateCheckNotifierForTest(func(string, string) { atomic.AddInt32(&notifications, 1) })
	defer restore()

	finish := startUpUpdateCheck(false)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&notifications) == 0 {
		finish()
		time.Sleep(5 * time.Millisecond)
	}

	if got := atomic.LoadInt32(&notifications); got != 1 {
		t.Fatalf("%d notifications for a newer release, want one", got)
	}
}

// TestStorefrontProbeVerifiesAgainstTheGovardCA pins the real probe: it trusts the
// certificate the proxy issues, and it does not accept a certificate from an
// authority that pool does not hold.
func TestStorefrontProbeVerifiesAgainstTheGovardCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	trusted := x509.NewCertPool()
	trusted.AddCert(server.Certificate())

	originalPool := upStorefrontRootCAs
	originalProbe := upStorefrontProbe
	t.Cleanup(func() {
		upStorefrontRootCAs = originalPool
		upStorefrontProbe = originalProbe
	})
	upStorefrontRootCAs = func() (*x509.CertPool, error) { return trusted, nil }

	if err := upStorefrontProbe(server.URL + "/"); err != nil {
		t.Fatalf("probe rejected a certificate signed by the trusted authority: %v", err)
	}

	untrusted := x509.NewCertPool()
	upStorefrontRootCAs = func() (*x509.CertPool, error) { return untrusted, nil }
	if err := upStorefrontProbe(server.URL + "/"); err == nil {
		t.Fatal("probe accepted a certificate from an authority outside the pool")
	}
}
