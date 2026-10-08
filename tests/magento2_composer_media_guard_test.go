package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/frameworks"
	"govard/internal/frameworks/magento2"
)

func writeMediaFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// magento/sample-data-media maps `* -> pub/media`; the Magento composer
// installer then copies `catalog` INTO an existing pub/media/catalog, which is
// how a clone bootstrap (it wipes vendor/ and reinstalls everything) leaves
// pub/media/catalog/catalog/... behind.
func TestMagentoComposerInstallGuardRemovesOnlyNewInstallerNesting(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "pub", "media")
	pkg := filepath.Join(root, "vendor", "magento", "sample-data-media")
	writeMediaFixture(t, filepath.Join(media, "catalog", "product", "a.jpg"))
	writeMediaFixture(t, filepath.Join(media, "wysiwyg", "w.jpg"))
	// Present before the install: user or sample content, never touched.
	writeMediaFixture(t, filepath.Join(media, "downloadable", "downloadable", "files", "keep.bin"))
	writeMediaFixture(t, filepath.Join(media, "other", "other", "mine.txt"))
	writeMediaFixture(t, filepath.Join(pkg, "catalog", "product", "a.jpg"))
	writeMediaFixture(t, filepath.Join(pkg, "wysiwyg", "w.jpg"))
	writeMediaFixture(t, filepath.Join(pkg, "downloadable", "downloadable", "files", "keep.bin"))

	done := magento2.GuardComposerInstallMedia(root)
	// What the installer does: copy each package entry into the existing dir.
	writeMediaFixture(t, filepath.Join(media, "catalog", "catalog", "product", "a.jpg"))
	writeMediaFixture(t, filepath.Join(media, "wysiwyg", "wysiwyg", "w.jpg"))
	writeMediaFixture(t, filepath.Join(media, "downloadable", "downloadable", "downloadable", "files", "keep.bin"))
	// A new nested dir the package does not ship is not the installer's.
	writeMediaFixture(t, filepath.Join(media, "other", "other", "other", "new.txt"))
	done()

	for _, gone := range []string{
		filepath.Join(media, "catalog", "catalog"),
		filepath.Join(media, "wysiwyg", "wysiwyg"),
		filepath.Join(media, "downloadable", "downloadable", "downloadable"),
	} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s was created by the installer and should be removed", gone)
		}
	}
	for _, kept := range []string{
		filepath.Join(media, "catalog", "product", "a.jpg"),
		filepath.Join(media, "downloadable", "downloadable", "files", "keep.bin"),
		filepath.Join(media, "other", "other", "mine.txt"),
		filepath.Join(media, "other", "other", "other", "new.txt"),
	} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s must stay: %v", kept, err)
		}
	}
}

func TestMagentoComposerInstallGuardIsNoopWithoutSampleMedia(t *testing.T) {
	root := t.TempDir()
	writeMediaFixture(t, filepath.Join(root, "pub", "media", "catalog", "catalog", "x.jpg"))
	done := magento2.GuardComposerInstallMedia(root)
	done()
	if _, err := os.Stat(filepath.Join(root, "pub", "media", "catalog", "catalog", "x.jpg")); err != nil {
		t.Fatalf("nothing may be removed without the package: %v", err)
	}
}

func TestMagentoFamilyDefinitionsDeclareTheComposerInstallGuard(t *testing.T) {
	def, ok := frameworks.Get("magento2")
	if !ok || def.ComposerInstallGuard == nil {
		t.Fatal("magento2 must declare ComposerInstallGuard")
	}
}
