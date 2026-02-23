package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestRenderBlueprintWithRabbitMQ(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "rabbitmq-test",
		Recipe:      "magento2",
		Domain:      "rabbitmq.test",
		Stack: engine.Stack{
			Services: engine.Services{
				Queue: "rabbitmq",
			},
		},
	})

	if !strings.Contains(content, "rabbitmq:") {
		t.Fatalf("Expected rabbitmq service in compose output")
	}
	if !strings.Contains(content, "rabbitmq:3.13.7-management-alpine") {
		t.Fatalf("Expected rabbitmq image to use default version")
	}
}

func TestRenderBlueprintWithValkey(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "valkey-test",
		Recipe:      "magento2",
		Domain:      "valkey.test",
		Stack: engine.Stack{
			Services: engine.Services{
				Cache: "valkey",
			},
		},
	})

	if !strings.Contains(content, "valkey/valkey:8.0.0") {
		t.Fatalf("Expected valkey image with default version")
	}
}

func TestRenderBlueprintWithOpensearch(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "opensearch-test",
		Recipe:      "magento2",
		Domain:      "opensearch.test",
		Stack: engine.Stack{
			Services: engine.Services{
				Search: "opensearch",
			},
		},
	})

	if !strings.Contains(content, "govard/opensearch:2.19.0") {
		t.Fatalf("Expected opensearch image with default version")
	}
}

func TestRenderNextjsNodeVersionOverride(t *testing.T) {
	content := renderComposeWithConfig(t, engine.Config{
		ProjectName: "node-version-test",
		Recipe:      "nextjs",
		Domain:      "nextjs.test",
		Stack: engine.Stack{
			NodeVersion: "20",
		},
	})

	if !strings.Contains(content, "image: node:20-alpine") {
		t.Fatalf("Expected node image to use overridden version")
	}
}

func renderComposeWithConfig(t *testing.T, config engine.Config) string {
	t.Helper()

	tempDir := t.TempDir()

	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..")
	blueprintsDir := filepath.Join(projectRoot, "blueprints")

	destBlueprintsDir := filepath.Join(tempDir, "blueprints")
	if err := copyDir(blueprintsDir, destBlueprintsDir); err != nil {
		t.Fatalf("Failed to copy blueprints: %v", err)
	}

	if err := engine.RenderBlueprint(tempDir, config); err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	content, err := os.ReadFile(engine.ComposeFilePath(tempDir, config.ProjectName))
	if err != nil {
		t.Fatalf("Failed to read generated compose file: %v", err)
	}

	return string(content)
}
