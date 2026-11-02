package engine

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// rootCAExporter copies the proxy's root CA to the host and returns the path
// it wrote. Tests replace it so no container is touched.
var rootCAExporter = func() (string, error) {
	info, err := exportRootCA()
	return info.Path, err
}

// SetRootCAExporterForTest overrides how LocalRootCAPool extracts the CA from
// the proxy when the exported copy is unusable. It returns a restore func.
func SetRootCAExporterForTest(fn func() (string, error)) func() {
	previous := rootCAExporter
	rootCAExporter = fn
	return func() { rootCAExporter = previous }
}

// LocalRootCAPool returns a pool holding only the Govard local root CA, the
// authority the Govard proxy issues every project certificate from.
//
// It reads the copy exported to <govard home>/ssl/root.crt and, when that file
// is missing or does not hold a certificate, extracts the CA from the running
// proxy container the same way `govard doctor trust` does. System roots are
// deliberately left out: a caller holding this pool only ever talks to the
// Govard proxy, so no other authority should be able to vouch for it.
func LocalRootCAPool() (*x509.CertPool, error) {
	localPath := filepath.Join(GovardHomeDir(), "ssl", "root.crt")
	pool, localErr := rootCAPoolFromFile(localPath)
	if localErr == nil {
		return pool, nil
	}

	exportedPath, exportErr := rootCAExporter()
	if exportErr == nil {
		pool, exportErr = rootCAPoolFromFile(exportedPath)
		if exportErr == nil {
			return pool, nil
		}
	}

	return nil, fmt.Errorf("load the Govard root CA: %s: %w; extracting it from the proxy failed: %w",
		localPath, localErr, exportErr)
}

// RefreshLocalRootCAPool re-extracts the root CA from the running proxy even
// when the exported copy parses, for when that copy is stale: the proxy's data
// volume was recreated and Caddy minted a new CA. The fresh copy replaces the
// exported one, and the returned pool holds only it.
func RefreshLocalRootCAPool() (*x509.CertPool, error) {
	exportedPath, err := rootCAExporter()
	if err != nil {
		return nil, fmt.Errorf("re-extract the Govard root CA from the proxy: %w", err)
	}
	return rootCAPoolFromFile(exportedPath)
}

func rootCAPoolFromFile(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	der := data
	if block, _ := pem.Decode(data); block != nil {
		der = block.Bytes
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if !cert.IsCA {
		return nil, errors.New(path + " is not a CA certificate")
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return pool, nil
}
