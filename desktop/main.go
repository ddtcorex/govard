//go:build desktop

package main

import (
	"log"

	"govard/desktop/frontend"
	"govard/internal/desktop"
)

func main() {
	assets, err := desktop.ResolveAssets(frontend.Assets)
	if err != nil {
		log.Fatalf("Failed to locate frontend assets: %v", err)
	}
	if err := desktop.Run(assets); err != nil {
		log.Fatalf("Failed to start Govard Desktop: %v", err)
	}
}
