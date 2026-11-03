package tests

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

// writeHomeRootCA places data where Govard exports its root CA and returns
// that path.
func writeHomeRootCA(t *testing.T, data []byte) string {
	t.Helper()

	sslDir := filepath.Join(engine.GovardHomeDir(), "ssl")
	if err := os.MkdirAll(sslDir, 0o755); err != nil {
		t.Fatalf("mkdir ssl: %v", err)
	}
	path := filepath.Join(sslDir, "root.crt")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write root.crt: %v", err)
	}

	return path
}

func assertPoolTrusts(t *testing.T, pool *x509.CertPool, ca testCA) {
	t.Helper()

	if pool == nil {
		t.Fatal("pool is nil")
	}
	if _, err := ca.cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Fatalf("pool does not trust the Govard CA: %v", err)
	}
}

func TestLocalRootCAPoolLoadsTheExportedCertificate(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	ca := newTestCA(t)
	writeHomeRootCA(t, ca.PEM())

	restore := engine.SetRootCAExporterForTest(func() (string, error) {
		t.Error("exporter called although ssl/root.crt is present")
		return "", errors.New("unexpected export")
	})
	defer restore()

	pool, err := engine.LocalRootCAPool()
	if err != nil {
		t.Fatalf("LocalRootCAPool: %v", err)
	}
	assertPoolTrusts(t, pool, ca)

	if _, err := newTestCA(t).cert.Verify(x509.VerifyOptions{Roots: pool}); err == nil {
		t.Fatal("pool trusts an unrelated CA, want only the Govard CA")
	}
}

func TestLocalRootCAPoolExportsFromTheProxyWhenTheFileIsMissing(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	ca := newTestCA(t)
	exported := filepath.Join(t.TempDir(), "root.crt")

	calls := 0
	restore := engine.SetRootCAExporterForTest(func() (string, error) {
		calls++
		return exported, os.WriteFile(exported, ca.PEM(), 0o644)
	})
	defer restore()

	pool, err := engine.LocalRootCAPool()
	if err != nil {
		t.Fatalf("LocalRootCAPool: %v", err)
	}
	if calls != 1 {
		t.Fatalf("exporter calls = %d, want 1", calls)
	}
	assertPoolTrusts(t, pool, ca)
}

func TestLocalRootCAPoolReExportsWhenTheFileIsGarbage(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	writeHomeRootCA(t, []byte("not a certificate"))
	ca := newTestCA(t)
	exported := filepath.Join(t.TempDir(), "root.crt")

	restore := engine.SetRootCAExporterForTest(func() (string, error) {
		return exported, os.WriteFile(exported, ca.PEM(), 0o644)
	})
	defer restore()

	pool, err := engine.LocalRootCAPool()
	if err != nil {
		t.Fatalf("LocalRootCAPool: %v", err)
	}
	assertPoolTrusts(t, pool, ca)
}

func TestLocalRootCAPoolRejectsALeafCertificate(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	issuer := newTestCA(t)
	leaf := issuer.Leaf(t, "shop.test")
	writeHomeRootCA(t, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Certificate[0]}))
	ca := newTestCA(t)
	exported := filepath.Join(t.TempDir(), "root.crt")

	calls := 0
	restore := engine.SetRootCAExporterForTest(func() (string, error) {
		calls++
		return exported, os.WriteFile(exported, ca.PEM(), 0o644)
	})
	defer restore()

	pool, err := engine.LocalRootCAPool()
	if err != nil {
		t.Fatalf("LocalRootCAPool: %v", err)
	}
	if calls != 1 {
		t.Fatalf("exporter calls = %d, want 1 for a non CA root.crt", calls)
	}
	assertPoolTrusts(t, pool, ca)
}

func TestRefreshLocalRootCAPoolExportsEvenWhenTheFileParses(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	writeHomeRootCA(t, newTestCA(t).PEM())
	fresh := newTestCA(t)
	exported := filepath.Join(t.TempDir(), "root.crt")

	calls := 0
	restore := engine.SetRootCAExporterForTest(func() (string, error) {
		calls++
		return exported, os.WriteFile(exported, fresh.PEM(), 0o644)
	})
	defer restore()

	pool, err := engine.RefreshLocalRootCAPool()
	if err != nil {
		t.Fatalf("RefreshLocalRootCAPool: %v", err)
	}
	if calls != 1 {
		t.Fatalf("exporter calls = %d, want 1", calls)
	}
	assertPoolTrusts(t, pool, fresh)
}

func TestLocalRootCAPoolFailsWhenNeitherSourceWorks(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	writeHomeRootCA(t, []byte("not a certificate"))

	restore := engine.SetRootCAExporterForTest(func() (string, error) {
		return "", errors.New("tried containers govard-proxy-caddy: no such container")
	})
	defer restore()

	pool, err := engine.LocalRootCAPool()
	if err == nil {
		t.Fatalf("LocalRootCAPool returned a pool (%v), want an error", pool)
	}
	for _, want := range []string{"root.crt", "no such container"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
