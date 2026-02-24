package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestRequiredRuntimeImagesMagento(t *testing.T) {
	images := cmd.RequiredRuntimeImages(engine.Config{
		Recipe: "magento2",
		Stack: engine.Stack{
			PHPVersion:    "8.3",
			DBType:        "mariadb",
			DBVersion:     "10.6",
			CacheVersion:  "7.4",
			SearchVersion: "2.12.0",
			QueueVersion:  "3.13.7",
			Services: engine.Services{
				WebServer: "nginx",
				Cache:     "redis",
				Search:    "opensearch",
				Queue:     "rabbitmq",
			},
			Features: engine.Features{
				Varnish: true,
			},
		},
	})

	expected := map[string]bool{
		"govard/nginx:1.28":        true,
		"govard/php-magento2:8.3":  true,
		"govard/mariadb:10.6":      true,
		"govard/redis:7.4":         true,
		"govard/opensearch:2.12.0": true,
		"govard/rabbitmq:3.13.7":   true,
		"govard/varnish:7.6":       true,
	}

	for _, image := range images {
		delete(expected, image)
	}

	if len(expected) > 0 {
		t.Fatalf("missing expected images: %+v (got %v)", expected, images)
	}
}

func TestRequiredRuntimeImagesNextjs(t *testing.T) {
	images := cmd.RequiredRuntimeImages(engine.Config{
		Recipe: "nextjs",
		Stack: engine.Stack{
			NodeVersion:  "24",
			CacheVersion: "7.4",
			QueueVersion: "3.13.7",
			Services: engine.Services{
				Cache: "redis",
				Queue: "rabbitmq",
			},
		},
	})

	expected := map[string]bool{
		"node:24-alpine":         true,
		"govard/redis:7.4":       true,
		"govard/rabbitmq:3.13.7": true,
	}

	for _, image := range images {
		delete(expected, image)
		if image == "govard/php:8.4" || image == "govard/nginx:latest" {
			t.Fatalf("unexpected non-nextjs image: %s", image)
		}
	}

	if len(expected) > 0 {
		t.Fatalf("missing expected images: %+v (got %v)", expected, images)
	}
}

func TestRequiredRuntimeImagesMagentoHybrid(t *testing.T) {
	images := cmd.RequiredRuntimeImages(engine.Config{
		Recipe: "magento2",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			DBType:     "mariadb",
			DBVersion:  "10.6",
			Services: engine.Services{
				WebServer: "hybrid",
				Cache:     "none",
				Search:    "none",
				Queue:     "none",
			},
		},
	})

	expected := map[string]bool{
		"govard/nginx:1.28":       true,
		"govard/apache:2.4":       true,
		"govard/php-magento2:8.3": true,
		"govard/mariadb:10.6":     true,
	}

	for _, image := range images {
		delete(expected, image)
	}

	if len(expected) > 0 {
		t.Fatalf("missing expected images: %+v (got %v)", expected, images)
	}
}
