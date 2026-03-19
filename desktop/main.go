//go:build desktop

package main

import (
	"log"

	"govard/desktop/frontend"
	"govard/internal/desktop"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	app := desktop.NewApp()
	assets, err := desktop.ResolveAssets(frontend.Assets)
	if err != nil {
		log.Fatalf("Failed to locate frontend assets: %v", err)
	}

	err = wails.Run(&options.App{
		Title:         "Govard Desktop",
		Width:         1200,
		Height:        800,
		AssetServer:   &assetserver.Options{Assets: assets},
		OnStartup:     app.Startup,
		OnBeforeClose: app.BeforeClose,
		OnShutdown:    app.Shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatalf("Failed to start Govard Desktop: %v", err)
	}
}
