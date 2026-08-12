package magento2

import (
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

// FrontendSyncDiscoverer exposes Magento's frontend-sync discovery through
// the framework registry. Mage-OS inherits this from Magento 2.
func FrontendSyncDiscoverer(root string) (types.FrontendSyncRuntime, error) {
	runtime, err := DiscoverFrontendSyncRuntime(root)
	if err != nil {
		return types.FrontendSyncRuntime{}, err
	}
	return frontendSyncProviderRuntime(runtime), nil
}

// FrontendSyncRenderer exposes Magento's frontend-sync blueprint rendering
// through the framework registry. Mage-OS inherits this from Magento 2.
func FrontendSyncRenderer(root string, config engine.Config) (types.FrontendSyncRuntime, string, error) {
	runtime, composePath, err := RenderFrontendBlueprint(root, config)
	if err != nil {
		return types.FrontendSyncRuntime{}, "", err
	}
	return frontendSyncProviderRuntime(runtime), composePath, nil
}

func frontendSyncProviderRuntime(runtime FrontendSyncRuntime) types.FrontendSyncRuntime {
	services := []string{"sync"}
	providerRuntime := types.FrontendSyncRuntime{
		Mode:     string(runtime.Mode),
		Services: services,
	}
	switch runtime.Mode {
	case FrontendSyncModeHyva:
		for _, watcher := range buildFrontendSyncWatchers(runtime.Themes) {
			services = append(services, watcher.ServiceName)
		}
		services = append(services, "inject")
		providerRuntime.PublicEndpoint = types.FrontendSyncPublicEndpoint{
			Path:    "/browser-sync/*",
			Service: "sync",
			Port:    3000,
		}
		providerRuntime.HTMLInjection = &types.FrontendSyncHTMLInjection{
			Service: "inject",
			Port:    3000,
		}
	case FrontendSyncModeLuma:
		services = append(services, "inject")
		providerRuntime.PublicEndpoint = types.FrontendSyncPublicEndpoint{
			Path:        "/livereload/*",
			StripPrefix: "/livereload",
			Service:     "sync",
			Port:        35729,
		}
		providerRuntime.HTMLInjection = &types.FrontendSyncHTMLInjection{
			Service: "inject",
			Port:    3000,
		}
	}
	providerRuntime.Services = services
	return providerRuntime
}
