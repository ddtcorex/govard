package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRabbitMQBlueprintJoinsProxyNetwork(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "internal", "blueprints", "files", "includes", "rabbitmq.yml"))
	if err != nil {
		t.Fatalf("read rabbitmq blueprint: %v", err)
	}
	if !strings.Contains(string(content), "govard-proxy") {
		t.Fatal("rabbitmq.yml must join the govard-proxy network so Caddy can reach the management UI from the host")
	}
	if !strings.Contains(string(content), "govard-net") {
		t.Fatal("rabbitmq.yml must stay on govard-net (its project-internal traffic never moves)")
	}
}
