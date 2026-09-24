//go:build desktop

package main

import (
	"log"

	"govard/desktop/frontend"
	"govard/internal/desktop"
)

func main() {
	if err := desktop.Run(frontend.Assets); err != nil {
		log.Fatalf("Failed to start Govard Desktop: %v", err)
	}
}
