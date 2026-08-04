package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/blueprints"
	"govard/internal/engine"
)

func TestRenderBlueprintWithRabbitMQ(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "rabbitmq-test",
		Framework:   "magento2",
		Domain:      "rabbitmq.test",
		Stack: engine.Stack{
			Features: engine.Features{
				Queue: true,
			},
			Services: engine.Services{
				Queue: "rabbitmq",
			},
		},
	})

	if !strings.Contains(content, "rabbitmq:") {
		t.Fatalf("Expected rabbitmq service in compose output")
	}
	if !strings.Contains(content, "ddtcorex/govard-rabbitmq:4.2") {
		t.Fatalf("Expected rabbitmq image to use default version")
	}
	if !strings.Contains(content, "/etc/rabbitmq/rabbitmq.conf:ro") {
		t.Fatalf("Expected rabbitmq.conf to be mounted read-only, got:\n%s", content)
	}
	if !strings.Contains(content, filepath.Join("rabbitmq", "rabbitmq-test", "rabbitmq.conf")) {
		t.Fatalf("Expected rabbitmq.conf staged under GovardHomeDir()/rabbitmq/<project>/, got:\n%s", content)
	}
}

func TestRenderBlueprintWithValkey(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "valkey-test",
		Framework:   "magento2",
		Domain:      "valkey.test",
		Stack: engine.Stack{
			Features: engine.Features{
				Cache: true,
			},
			Services: engine.Services{
				Cache: "valkey",
			},
		},
	})

	if !strings.Contains(content, "ddtcorex/govard-valkey:9.0") {
		t.Fatalf("Expected valkey image with default version")
	}
}

func TestRenderBlueprintWithOpensearch(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "opensearch-test",
		Framework:   "magento2",
		Domain:      "opensearch.test",
		Stack: engine.Stack{
			Features: engine.Features{
				Search: true,
			},
			Services: engine.Services{
				Search: "opensearch",
			},
		},
	})

	if !strings.Contains(content, "ddtcorex/govard-opensearch:3.0") {
		t.Fatalf("Expected opensearch image with default version")
	}
}

func TestRenderNextjsNodeVersionOverride(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "node-version-test",
		Framework:   "nextjs",
		Domain:      "nextjs.test",
		Stack: engine.Stack{
			NodeVersion: "20",
		},
	})

	if !strings.Contains(content, "image: node:20") {
		t.Fatalf("Expected node image to use overridden version")
	}
}

func TestRenderMinimalMagento2(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "minimal-mg2",
		Framework:   "magento2",
		Domain:      "minimal.test",
	})

	// Core services should be present
	mandatory := []string{"web:", "php:"}
	for _, s := range mandatory {
		if !strings.Contains(content, s) {
			t.Errorf("Expected mandatory service %s to be present", s)
		}
	}

	// Optional services should NOT be present
	optional := []string{"db:", "redis:", "elasticsearch:", "varnish:", "rabbitmq:", "selenium:"}
	for _, s := range optional {
		if strings.Contains(content, s) {
			t.Errorf("Unexpected optional service %s found in minimal Magento 2 output", s)
		}
	}
}

func TestRenderMinimalLaravel(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "minimal-laravel",
		Framework:   "laravel",
		Domain:      "minimal.test",
	})

	// Core services should be present
	mandatory := []string{"web:", "php:"}
	for _, s := range mandatory {
		if !strings.Contains(content, s) {
			t.Errorf("Expected mandatory service %s to be present", s)
		}
	}

	// Optional services (including the previously unconditional queue) should NOT be present
	optional := []string{"db:", "redis:", "elasticsearch:", "queue:"}
	for _, s := range optional {
		if strings.Contains(content, s) {
			t.Errorf("Unexpected optional service %s found in minimal Laravel output", s)
		}
	}
}

func renderComposeWithConfig(t *testing.T, config engine.Config) string {
	t.Helper()

	tempDir := t.TempDir()
	setTestGovardHome(t, tempDir)

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := os.CopyFS(destBlueprintsDir, blueprints.FS); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePathWithProfile(tempDir, config.ProjectName, config.Profile))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}

	return string(content)
}
