package tests

import (
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

const composerLockFixture = `{
  "packages": [
    {"name": "psr/log", "version": "1.1.4"},
    {"name": "magento/framework", "version": "103.0.5"}
  ],
  "packages-dev": [
    {"name": "phpunit/phpunit", "version": "9.6.19"}
  ]
}`

func writeComposerLockFixture(t *testing.T, projectRoot string, content string) {
	t.Helper()
	if content == "" {
		return
	}
	if err := osWriteFile(filepath.Join(projectRoot, "composer.lock"), []byte(content)); err != nil {
		t.Fatalf("write composer.lock fixture: %v", err)
	}
}

func writeInstalledJSONFixture(t *testing.T, projectRoot string, content string) {
	t.Helper()
	if content == "" {
		return
	}
	path := filepath.Join(projectRoot, "vendor", "composer", "installed.json")
	if err := osWriteFile(path, []byte(content)); err != nil {
		t.Fatalf("write installed.json fixture: %v", err)
	}
}

func TestVendorSatisfiesComposerLockExactMatchComposerV2(t *testing.T) {
	projectRoot := t.TempDir()
	writeComposerLockFixture(t, projectRoot, composerLockFixture)
	writeInstalledJSONFixture(t, projectRoot, `{
  "packages": [
    {"name": "psr/log", "version": "1.1.4"},
    {"name": "magento/framework", "version": "103.0.5"},
    {"name": "phpunit/phpunit", "version": "9.6.19"}
  ]
}`)

	satisfied, err := engine.VendorSatisfiesComposerLock(projectRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !satisfied {
		t.Fatal("expected vendor to satisfy composer.lock, got false")
	}
}

func TestVendorSatisfiesComposerLockExactMatchComposerV1BareArray(t *testing.T) {
	projectRoot := t.TempDir()
	writeComposerLockFixture(t, projectRoot, composerLockFixture)
	writeInstalledJSONFixture(t, projectRoot, `[
    {"name": "psr/log", "version": "1.1.4"},
    {"name": "magento/framework", "version": "103.0.5"},
    {"name": "phpunit/phpunit", "version": "9.6.19"}
  ]`)

	satisfied, err := engine.VendorSatisfiesComposerLock(projectRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !satisfied {
		t.Fatal("expected vendor to satisfy composer.lock (composer 1.x bare-array format), got false")
	}
}

func TestVendorSatisfiesComposerLockVersionMismatch(t *testing.T) {
	projectRoot := t.TempDir()
	writeComposerLockFixture(t, projectRoot, composerLockFixture)
	writeInstalledJSONFixture(t, projectRoot, `{
  "packages": [
    {"name": "psr/log", "version": "1.1.3"},
    {"name": "magento/framework", "version": "103.0.5"},
    {"name": "phpunit/phpunit", "version": "9.6.19"}
  ]
}`)

	satisfied, err := engine.VendorSatisfiesComposerLock(projectRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if satisfied {
		t.Fatal("expected version mismatch to yield false, got true")
	}
}

func TestVendorSatisfiesComposerLockMissingPackage(t *testing.T) {
	projectRoot := t.TempDir()
	writeComposerLockFixture(t, projectRoot, composerLockFixture)
	writeInstalledJSONFixture(t, projectRoot, `{
  "packages": [
    {"name": "psr/log", "version": "1.1.4"}
  ]
}`)

	satisfied, err := engine.VendorSatisfiesComposerLock(projectRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if satisfied {
		t.Fatal("expected missing package to yield false, got true")
	}
}

func TestVendorSatisfiesComposerLockMissingComposerLock(t *testing.T) {
	projectRoot := t.TempDir()
	writeInstalledJSONFixture(t, projectRoot, `{"packages": []}`)

	satisfied, err := engine.VendorSatisfiesComposerLock(projectRoot)
	if err != nil {
		t.Fatalf("expected no error when composer.lock is missing, got: %v", err)
	}
	if satisfied {
		t.Fatal("expected false when composer.lock is missing, got true")
	}
}

func TestVendorSatisfiesComposerLockMissingInstalledJSON(t *testing.T) {
	projectRoot := t.TempDir()
	writeComposerLockFixture(t, projectRoot, composerLockFixture)

	satisfied, err := engine.VendorSatisfiesComposerLock(projectRoot)
	if err != nil {
		t.Fatalf("expected no error when installed.json is missing, got: %v", err)
	}
	if satisfied {
		t.Fatal("expected false when installed.json is missing, got true")
	}
}

func TestVendorSatisfiesComposerLockMalformedJSON(t *testing.T) {
	projectRoot := t.TempDir()
	writeComposerLockFixture(t, projectRoot, `{not valid json`)
	writeInstalledJSONFixture(t, projectRoot, `{"packages": []}`)

	satisfied, err := engine.VendorSatisfiesComposerLock(projectRoot)
	if err != nil {
		t.Fatalf("expected no error for malformed composer.lock, got: %v", err)
	}
	if satisfied {
		t.Fatal("expected false for malformed composer.lock, got true")
	}
}
