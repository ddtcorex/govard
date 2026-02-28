//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"

	"govard/internal/engine"
)

func TestDockerStatusCheck(t *testing.T) {
	err := engine.CheckDockerStatus(context.Background())

	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

func TestDockerComposePluginCheck(t *testing.T) {
	err := engine.CheckDockerComposePlugin(context.Background())

	if err != nil {
		t.Skipf("Docker Compose plugin not available: %v", err)
	}
}

func TestPortAvailability(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{"Port 80", "80"},
		{"Port 443", "443"},
		{"Port 8080", "8080"},
		{"Port 3000", "3000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available := engine.CheckPort(tt.port)

			t.Logf("Port %s availability: %v", tt.port, available)
		})
	}
}

func TestContainerHelpers(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		expectExists  bool
	}{
		{
			name:          "Non-existent container",
			containerName: "govard-test-nonexistent-container",
			expectExists:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists := ContainerExists(tt.containerName)

			if exists != tt.expectExists {
				t.Errorf("Expected container exists=%v, got %v", tt.expectExists, exists)
			}

			running := ContainerRunning(tt.containerName)
			if running {
				t.Error("Non-existent container should not be running")
			}
		})
	}
}

func TestNetworkHelpers(t *testing.T) {
	tests := []struct {
		name         string
		networkName  string
		expectExists bool
	}{
		{
			name:         "Non-existent network",
			networkName:  "govard-test-nonexistent-network",
			expectExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists := NetworkExists(tt.networkName)

			if exists != tt.expectExists {
				t.Errorf("Expected network exists=%v, got %v", tt.expectExists, exists)
			}
		})
	}
}
