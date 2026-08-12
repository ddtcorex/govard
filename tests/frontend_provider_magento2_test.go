package tests

import (
	"reflect"
	"slices"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"
)

func TestMagentoFrontendSyncProviderDeclaresModeSpecificPublicRuntime(t *testing.T) {
	discover, ok := frameworks.FrontendSyncDiscoverer("magento2")
	if !ok {
		t.Fatal("Magento 2 must register a frontend sync discoverer")
	}

	tests := []struct {
		name          string
		writeRuntime  func(*testing.T, string)
		wantPath      string
		wantStripPath string
		wantPort      int
		wantServices  []string
		wantInjection bool
	}{
		{
			name: "Hyva BrowserSync",
			writeRuntime: func(t *testing.T, root string) {
				writeFrontendSyncTheme(t, root, "Vendor", "Theme", "theme-lock")
			},
			wantPath:      "/browser-sync/*",
			wantStripPath: "",
			wantPort:      3000,
			wantServices:  []string{"sync", "watch-vendor-theme", "inject"},
			wantInjection: true,
		},
		{
			name: "Luma LiveReload",
			writeRuntime: func(t *testing.T, root string) {
				writeFrontendSyncLumaRoot(t, root)
			},
			wantPath:      "/livereload/*",
			wantStripPath: "/livereload",
			wantPort:      35729,
			wantServices:  []string{"sync", "inject"},
			wantInjection: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.writeRuntime(t, root)
			runtime, err := discover(root)
			if err != nil {
				t.Fatalf("discover runtime: %v", err)
			}
			if runtime.PublicEndpoint.Path != test.wantPath || runtime.PublicEndpoint.StripPrefix != test.wantStripPath || runtime.PublicEndpoint.Service != "sync" || runtime.PublicEndpoint.Port != test.wantPort {
				t.Fatalf("public endpoint = %#v", runtime.PublicEndpoint)
			}
			if !slices.Equal(runtime.Services, test.wantServices) {
				t.Fatalf("services = %#v, want %#v", runtime.Services, test.wantServices)
			}
			if (runtime.HTMLInjection != nil) != test.wantInjection {
				t.Fatalf("HTML injection = %#v, want present %t", runtime.HTMLInjection, test.wantInjection)
			}
			if runtime.HTMLInjection != nil && (runtime.HTMLInjection.Service != "inject" || runtime.HTMLInjection.Port != 3000) {
				t.Fatalf("HTML injection = %#v", runtime.HTMLInjection)
			}
		})
	}
}

func TestMageOSInheritsMagentoFrontendSyncProvider(t *testing.T) {
	magentoDiscoverer, ok := frameworks.FrontendSyncDiscoverer("magento2")
	if !ok {
		t.Fatal("Magento 2 must register a frontend sync discoverer")
	}
	mageOSDiscoverer, ok := frameworks.FrontendSyncDiscoverer("mageos")
	if !ok {
		t.Fatal("Mage-OS must inherit the Magento frontend sync discoverer")
	}
	if reflect.ValueOf(magentoDiscoverer).Pointer() != reflect.ValueOf(mageOSDiscoverer).Pointer() {
		t.Fatal("Mage-OS discoverer does not reuse Magento's exact discoverer function")
	}

	magentoRenderer, ok := frameworks.FrontendSyncRenderer("magento2")
	if !ok {
		t.Fatal("Magento 2 must register a frontend sync renderer")
	}
	mageOSRenderer, ok := frameworks.FrontendSyncRenderer("mageos")
	if !ok {
		t.Fatal("Mage-OS must inherit the Magento frontend sync renderer")
	}
	if reflect.ValueOf(magentoRenderer).Pointer() != reflect.ValueOf(mageOSRenderer).Pointer() {
		t.Fatal("Mage-OS renderer does not reuse Magento's exact renderer function")
	}

	root := t.TempDir()
	setTestGovardHome(t, t.TempDir())
	writeFrontendSyncTheme(t, root, "Vendor", "Theme", "theme-lock")
	result, err := mageOSDiscoverer(root)
	if err != nil {
		t.Fatalf("discover Mage-OS frontend runtime: %v", err)
	}
	if result.Mode != "hyva" {
		t.Fatalf("Mage-OS runtime mode = %q, want hyva", result.Mode)
	}

	_, composePath, err := mageOSRenderer(root, engine.Config{
		ProjectName: "synthetic-store",
		Framework:   "mageos",
		Domain:      "synthetic-store.test",
		Stack: engine.Stack{
			Features: engine.Features{FrontendSync: true},
			Services: engine.Services{WebServer: "nginx", Search: "none", Cache: "none", Queue: "none"},
		},
	})
	if err != nil {
		t.Fatalf("render Mage-OS frontend runtime: %v", err)
	}
	if composePath == "" {
		t.Fatal("Mage-OS frontend renderer returned no compose path")
	}
}
