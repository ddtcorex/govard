package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/frameworks/magento2"
)

func TestFrontendSyncDiscoveryReturnsNoThemesWhenThemeRootIsAbsent(t *testing.T) {
	root := t.TempDir()

	themes, err := magento2.DiscoverFrontendSyncThemes(root)
	if err != nil {
		t.Fatalf("discover frontend sync themes: %v", err)
	}
	if len(themes) != 0 {
		t.Fatalf("expected no themes, got %#v", themes)
	}
}

func TestFrontendSyncDiscoveryRecordsThemeRuntimePathsAndLockHash(t *testing.T) {
	root := t.TempDir()
	writeFrontendSyncTheme(t, root, "Hyva", "Default", `{"lockfileVersion":3}`)

	themes, err := magento2.DiscoverFrontendSyncThemes(root)
	if err != nil {
		t.Fatalf("discover frontend sync themes: %v", err)
	}
	want := []magento2.FrontendSyncTheme{{
		Vendor:            "Hyva",
		Theme:             "Default",
		TailwindDir:       "app/design/frontend/Hyva/Default/web/tailwind",
		CSSOutputDir:      "app/design/frontend/Hyva/Default/web/css",
		PackageLockHash:   "2086223f909ecaf4b063cfc7e8187ab73e8e8a24ec7029d54e0403dee55a3265",
		PackageJSONHash:   "b8fb9cc187f138a8a7f1f50a75d20b0bac9ba4e03dda5af92d946fd8f577c856",
		BrowserSyncScript: true,
	}}
	if !reflect.DeepEqual(themes, want) {
		t.Fatalf("unexpected themes:\n got: %#v\nwant: %#v", themes, want)
	}
}

func TestFrontendSyncDiscoverySortsMultipleThemesByVendorAndTheme(t *testing.T) {
	root := t.TempDir()
	writeFrontendSyncTheme(t, root, "Zeta", "Second", "zeta-lock")
	writeFrontendSyncTheme(t, root, "Alpha", "First", "alpha-lock")

	themes, err := magento2.DiscoverFrontendSyncThemes(root)
	if err != nil {
		t.Fatalf("discover frontend sync themes: %v", err)
	}
	want := []magento2.FrontendSyncTheme{
		{
			Vendor:            "Alpha",
			Theme:             "First",
			TailwindDir:       "app/design/frontend/Alpha/First/web/tailwind",
			CSSOutputDir:      "app/design/frontend/Alpha/First/web/css",
			PackageLockHash:   "544820a3b10cb4c1fd04d314010f75c6b16501486dd938c1859e41c974ee389b",
			PackageJSONHash:   "b8fb9cc187f138a8a7f1f50a75d20b0bac9ba4e03dda5af92d946fd8f577c856",
			BrowserSyncScript: true,
		},
		{
			Vendor:            "Zeta",
			Theme:             "Second",
			TailwindDir:       "app/design/frontend/Zeta/Second/web/tailwind",
			CSSOutputDir:      "app/design/frontend/Zeta/Second/web/css",
			PackageLockHash:   "d80b8c7d1068041c8ef28481a43c59e416e91dd34090f54af0824cd26b07bb65",
			PackageJSONHash:   "b8fb9cc187f138a8a7f1f50a75d20b0bac9ba4e03dda5af92d946fd8f577c856",
			BrowserSyncScript: true,
		},
	}
	if !reflect.DeepEqual(themes, want) {
		t.Fatalf("unexpected themes:\n got: %#v\nwant: %#v", themes, want)
	}
}

func TestDiscoverFrontendSyncRuntimeDiscoversLumaAtProjectRoot(t *testing.T) {
	root := t.TempDir()
	writeFrontendSyncLumaRoot(t, root)

	runtime, err := magento2.DiscoverFrontendSyncRuntime(root)
	if err != nil {
		t.Fatalf("discover Luma frontend runtime: %v", err)
	}
	want := magento2.FrontendSyncRuntime{
		Mode:              magento2.FrontendSyncModeLuma,
		WorkingDir:        ".",
		Command:           "npx grunt watch",
		PackageLockHash:   "e25b3e26894b0261fafd86d044f10b48e92e8cd8670493effd8207ff149019e5",
		PackageJSONHash:   "f726da3ad8351213bff3b92d7525e1565a4c4d7481c64fd7b751a48fc7d667d0",
		NodeModulesVolume: "frontend-sync-luma-node-modules-e25b3e26894b0261fafd86d044f10b48e92e8cd8670493effd8207ff149019e5",
	}
	if !reflect.DeepEqual(runtime, want) {
		t.Fatalf("unexpected Luma runtime:\n got: %#v\nwant: %#v", runtime, want)
	}
}

func TestDiscoverFrontendSyncRuntimeRejectsHyvaAndLuma(t *testing.T) {
	root := t.TempDir()
	writeFrontendSyncTheme(t, root, "Vendor", "Theme", "hyva-lock")
	writeFrontendSyncLumaRoot(t, root)

	_, err := magento2.DiscoverFrontendSyncRuntime(root)
	if err == nil {
		t.Fatal("expected a project with Hyva and Luma runtimes to be rejected")
	}
	for _, want := range []string{"Hyva", "Luma", "remove one frontend runtime"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected ambiguity remediation to contain %q, got %v", want, err)
		}
	}
}

func TestDiscoverFrontendSyncRuntimeRejectsLumaWithoutPackageLock(t *testing.T) {
	root := t.TempDir()
	writeFrontendSyncLumaRoot(t, root)
	if err := os.Remove(filepath.Join(root, "package-lock.json")); err != nil {
		t.Fatalf("remove Luma package lock: %v", err)
	}

	_, err := magento2.DiscoverFrontendSyncRuntime(root)
	if err == nil {
		t.Fatal("expected Luma runtime without a package lock to be rejected")
	}
	for _, want := range []string{"package-lock.json", "npm install"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected missing-lock remediation to contain %q, got %v", want, err)
		}
	}
}

func TestDiscoverFrontendSyncRuntimeRejectsHyvaAndIncompleteLumaWithMissingLockRemediation(t *testing.T) {
	root := t.TempDir()
	writeFrontendSyncTheme(t, root, "Vendor", "Theme", "hyva-lock")
	writeFrontendSyncLumaRoot(t, root)
	if err := os.Remove(filepath.Join(root, "package-lock.json")); err != nil {
		t.Fatalf("remove incomplete Luma package lock: %v", err)
	}

	_, err := magento2.DiscoverFrontendSyncRuntime(root)
	if err == nil {
		t.Fatal("expected Hyva and incomplete Luma setup to be rejected")
	}
	for _, want := range []string{"Hyva", "Luma", "package-lock.json", "npm install", "remove one frontend runtime"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected conflict remediation to contain %q, got %v", want, err)
		}
	}
}

func TestDiscoverFrontendSyncRuntimeRejectsMalformedNPMFilesBeforeCompose(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*testing.T, string)
		path       func(string) string
		wantDetail string
	}{
		{
			name: "Hyva package.json",
			setup: func(t *testing.T, root string) {
				writeFrontendSyncTheme(t, root, "Vendor", "Theme", `{"lockfileVersion":3}`)
				path := filepath.Join(root, "app", "design", "frontend", "Vendor", "Theme", "web", "tailwind", "package.json")
				if err := os.WriteFile(path, []byte(`{"scripts":`), 0o644); err != nil {
					t.Fatalf("write malformed Hyva manifest: %v", err)
				}
			},
			path: func(root string) string {
				return filepath.Join(root, "app", "design", "frontend", "Vendor", "Theme", "web", "tailwind", "package.json")
			},
			wantDetail: "valid JSON",
		},
		{
			name: "Hyva package-lock.json",
			setup: func(t *testing.T, root string) {
				writeFrontendSyncTheme(t, root, "Vendor", "Theme", `{"lockfileVersion":3}`)
				path := filepath.Join(root, "app", "design", "frontend", "Vendor", "Theme", "web", "tailwind", "package-lock.json")
				if err := os.WriteFile(path, []byte(`{"lockfileVersion":`), 0o644); err != nil {
					t.Fatalf("write malformed Hyva lock: %v", err)
				}
			},
			path: func(root string) string {
				return filepath.Join(root, "app", "design", "frontend", "Vendor", "Theme", "web", "tailwind", "package-lock.json")
			},
			wantDetail: "npm ci",
		},
		{
			name: "Luma package.json",
			setup: func(t *testing.T, root string) {
				writeFrontendSyncLumaRoot(t, root)
				if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":`), 0o644); err != nil {
					t.Fatalf("write malformed Luma manifest: %v", err)
				}
			},
			path:       func(root string) string { return filepath.Join(root, "package.json") },
			wantDetail: "valid JSON",
		},
		{
			name: "Luma package-lock.json",
			setup: func(t *testing.T, root string) {
				writeFrontendSyncLumaRoot(t, root)
				if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{"lockfileVersion":`), 0o644); err != nil {
					t.Fatalf("write malformed Luma lock: %v", err)
				}
			},
			path:       func(root string) string { return filepath.Join(root, "package-lock.json") },
			wantDetail: "npm ci",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.setup(t, root)
			_, err := magento2.DiscoverFrontendSyncRuntime(root)
			if err == nil {
				t.Fatal("expected malformed npm file to be rejected")
			}
			for _, want := range []string{test.path(root), test.wantDetail} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v, want actionable detail %q", err, want)
				}
			}
		})
	}
}

func writeFrontendSyncTheme(t *testing.T, root, vendor, theme, packageLock string) {
	t.Helper()
	tailwindDir := filepath.Join(root, "app", "design", "frontend", vendor, theme, "web", "tailwind")
	if err := os.MkdirAll(tailwindDir, 0o755); err != nil {
		t.Fatalf("create Tailwind directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tailwindDir, "package.json"), []byte(`{"scripts":{"watch":"tailwindcss --watch","browser-sync":"browser-sync start --config browser-sync.config.js"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	lockContent := []byte(packageLock)
	if !json.Valid(lockContent) {
		lockContent = []byte(fmt.Sprintf(`{"lockfileVersion":3,"fixture":%q}`, packageLock))
	}
	if err := os.WriteFile(filepath.Join(tailwindDir, "package-lock.json"), lockContent, 0o644); err != nil {
		t.Fatalf("write package-lock.json: %v", err)
	}
}

func writeFrontendSyncThemeWithoutBrowserSync(t *testing.T, root, vendor, theme, packageLock string) {
	t.Helper()
	writeFrontendSyncTheme(t, root, vendor, theme, packageLock)
	packagePath := filepath.Join(root, "app", "design", "frontend", vendor, theme, "web", "tailwind", "package.json")
	if err := os.WriteFile(packagePath, []byte(`{"scripts":{"watch":"tailwindcss --watch"}}`), 0o644); err != nil {
		t.Fatalf("write package.json without BrowserSync: %v", err)
	}
}

func writeFrontendSyncLumaRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "Gruntfile.js"), []byte("module.exports = function (grunt) {};\n"), 0o644); err != nil {
		t.Fatalf("write Luma Gruntfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"watch":"grunt watch"},"devDependencies":{"grunt":"1.6.1"}}`), 0o644); err != nil {
		t.Fatalf("write Luma package manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{"lockfileVersion":3,"packages":{}}`), 0o644); err != nil {
		t.Fatalf("write Luma package lock: %v", err)
	}
}
