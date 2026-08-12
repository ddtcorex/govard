package magento2

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"govard/internal/conventions"
	"govard/internal/engine"

	"gopkg.in/yaml.v3"
)

// lumaInjectScriptHTML and hyvaInjectScriptHTML are inserted before </body>
// by the shared frontend-inject.mjs proxy. Both are served on the
// application's own domain (never a separate host), so the injected
// client's same-origin requests always match the application's own
// configured base URL.
const (
	lumaInjectScriptHTML = `<script src="/livereload/livereload.js?snipver=1&port=443&path=livereload/livereload"></script>`
	hyvaInjectScriptHTML = `<script src="/browser-sync/browser-sync-client.js"></script>`
)

type frontendRenderData struct {
	Config                engine.Config
	Runtime               FrontendSyncRuntime
	FrontendSyncWatchers  []FrontendSyncWatcher
	ApplicationNetwork    string
	FrontendSyncTarget    string
	FrontendSyncPublicURL string
	WorkDir               string
	InjectScriptPath      string
	InjectScriptHTML      string
}

// RenderFrontendBlueprint renders the Magento-family frontend runtime into
// its own Compose file. Application rendering deliberately never includes
// these development-only services.
func RenderFrontendBlueprint(root string, config engine.Config) (FrontendSyncRuntime, string, error) {
	engine.NormalizeConfig(&config, root)
	if config.Framework != "magento2" && config.Framework != "mageos" {
		return FrontendSyncRuntime{}, "", fmt.Errorf("frontend sync is only supported for Magento 2 and Mage-OS, got %q", config.Framework)
	}
	if !config.Stack.Features.FrontendSync {
		return FrontendSyncRuntime{}, "", fmt.Errorf("frontend sync is disabled; enable stack.features.frontend_sync before rendering frontend services")
	}

	runtime, err := DiscoverFrontendSyncRuntime(root)
	if err != nil {
		return FrontendSyncRuntime{}, "", err
	}
	blueprintsFS, err := engine.ResolveBlueprintsFS(root, config)
	if err != nil {
		return FrontendSyncRuntime{}, "", err
	}

	data := frontendRenderData{
		Config:                config,
		Runtime:               runtime,
		ApplicationNetwork:    config.ProjectName + "_govard-net",
		FrontendSyncTarget:    "http://" + config.ProjectName + conventions.WebSuffix,
		FrontendSyncPublicURL: "https://" + config.Domain,
		WorkDir:               conventions.DefaultWorkDir,
	}
	templatePath := "magento2/includes/frontend-luma.yml"
	data.InjectScriptHTML = lumaInjectScriptHTML
	if runtime.Mode == FrontendSyncModeHyva {
		data.FrontendSyncWatchers = buildFrontendSyncWatchers(runtime.Themes)
		data.InjectScriptHTML = hyvaInjectScriptHTML
		templatePath = "magento2/includes/frontend-hyva.yml"
	}
	composePath := engine.FrontendComposeFilePath(root, config.ProjectName, config.Profile)
	injectScriptPath := filepath.Join(filepath.Dir(composePath), "frontend-inject.mjs")
	injectScript, readErr := fs.ReadFile(blueprintsFS, "magento2/support/frontend-inject.mjs")
	if readErr != nil {
		return FrontendSyncRuntime{}, "", fmt.Errorf("read HTML injector: %w", readErr)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(injectScriptPath), 0o755); mkdirErr != nil {
		return FrontendSyncRuntime{}, "", fmt.Errorf("create frontend runtime directory: %w", mkdirErr)
	}
	if writeErr := os.WriteFile(injectScriptPath, injectScript, 0o644); writeErr != nil {
		return FrontendSyncRuntime{}, "", fmt.Errorf("write HTML injector: %w", writeErr)
	}
	data.InjectScriptPath = injectScriptPath
	rendered, err := engine.RenderBlueprintTemplate(blueprintsFS, templatePath, data)
	if err != nil {
		return FrontendSyncRuntime{}, "", fmt.Errorf("render frontend blueprint: %w", err)
	}

	var compose map[string]interface{}
	if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
		return FrontendSyncRuntime{}, "", fmt.Errorf("parse frontend compose: %w", err)
	}
	if compose == nil {
		return FrontendSyncRuntime{}, "", fmt.Errorf("frontend compose template %s rendered no services", templatePath)
	}
	if err := engine.WriteRenderedCompose(composePath, compose); err != nil {
		return FrontendSyncRuntime{}, "", err
	}
	return runtime, composePath, nil
}
