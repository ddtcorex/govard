package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestGetFrameworkSyncNoiseExcludesIncludesSharedAndFrameworkSpecificRules(t *testing.T) {
	excludes := engine.GetFrameworkSyncNoiseExcludes("laravel")

	assertContains(t, excludes, ".env")
	assertContains(t, excludes, "storage/logs/*")
	assertNotContains(t, excludes, "var/cache/")
}

func TestGetFrameworkSyncNoiseExcludesFallsBackForUnknownFrameworks(t *testing.T) {
	excludes := engine.GetFrameworkSyncNoiseExcludes("unknown-framework")

	assertContains(t, excludes, ".git/")
	assertContains(t, excludes, "var/cache/")
}

func TestGetFrameworkMediaExcludesByMode(t *testing.T) {
	magentoMinimal := engine.GetFrameworkMediaExcludes("magento2", engine.FrameworkMediaModeMinimal)
	assertContains(t, magentoMinimal, "catalog/product")
	assertContains(t, magentoMinimal, "*.jpg")

	symfonyOptimized := engine.GetFrameworkMediaExcludes("symfony", engine.FrameworkMediaModeOptimized)
	assertContains(t, symfonyOptimized, "cache/")
	assertNotContains(t, symfonyOptimized, "*.jpg")
}

func TestFrameworkManifestFeatureFlags(t *testing.T) {
	if engine.FrameworkRequiresRunningEnvForFreshInstall("nextjs") {
		t.Fatal("expected nextjs fresh install to skip env startup requirement")
	}
	if !engine.FrameworkRequiresRunningEnvForFreshInstall("magento2") {
		t.Fatal("expected magento2 fresh install to require env startup")
	}

	if !engine.FrameworkSupportsPostClone("openmage") {
		t.Fatal("expected openmage to support post-clone steps")
	}
	if engine.FrameworkSupportsPostClone("magento2") {
		t.Fatal("expected magento2 post-clone steps to remain disabled")
	}
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("expected %q in %v", want, values)
}

func TestPrestaShopFrameworkManifest(t *testing.T) {
	if engine.ResolveFrameworkLocalMediaSubpath("prestashop") != "img" {
		t.Fatalf("expected prestashop local media subpath 'img', got %q", engine.ResolveFrameworkLocalMediaSubpath("prestashop"))
	}
	if engine.ResolveFrameworkRemoteMediaSubpath("prestashop") != "img" {
		t.Fatalf("expected prestashop remote media subpath 'img', got %q", engine.ResolveFrameworkRemoteMediaSubpath("prestashop"))
	}

	noiseExcludes := engine.GetFrameworkSyncNoiseExcludes("prestashop")
	assertContains(t, noiseExcludes, "var/cache/")
	assertContains(t, noiseExcludes, "var/logs/")

	tables := engine.GetFrameworkIgnoredTables("prestashop", true, true)
	assertContains(t, tables, "connections")
	assertContains(t, tables, "guest")
	assertContains(t, tables, "customer")
	assertContains(t, tables, "orders")

	if !engine.FrameworkSupportsPostClone("prestashop") {
		t.Fatal("expected prestashop to support post-clone steps")
	}
}

func assertNotContains(t *testing.T, values []string, unwanted string) {
	t.Helper()
	for _, value := range values {
		if value == unwanted {
			t.Fatalf("did not expect %q in %v", unwanted, values)
		}
	}
}
