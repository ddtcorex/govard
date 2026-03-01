package tests

import (
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func TestDesktopPkgBuildDerivedServicesForTestUsesServiceState(t *testing.T) {
	config := engine.Config{
		Stack: engine.Stack{
			PHPVersion: "8.2",
			DBType:     "mariadb",
			Services: engine.Services{
				WebServer: "nginx",
				Cache:     "redis",
				Search:    "elasticsearch",
				Queue:     "rabbitmq",
			},
			Features: engine.Features{
				Varnish: true,
			},
		},
	}

	serviceState := map[string]string{
		"web":           "running",
		"db":            "running",
		"php":           "running",
		"redis":         "running",
		"elasticsearch": "running",
		"rabbitmq":      "running",
		"varnish":       "running",
	}

	services := desktop.BuildDerivedServicesForTest(config, serviceState)
	if len(services) != 7 {
		t.Fatalf("expected 7 services, got %d", len(services))
	}

	for _, service := range services {
		if service.Target == "" {
			t.Fatalf("service %q missing target", service.Name)
		}
		if service.Status != "running" {
			t.Fatalf("service %q expected running status, got %q", service.Name, service.Status)
		}
	}
}

func TestDesktopPkgBuildFallbackServicesForTestPreservesObservedState(t *testing.T) {
	discovered := map[string]bool{
		"web":   true,
		"php":   true,
		"db":    true,
		"redis": true,
	}
	serviceState := map[string]string{
		"web":   "running",
		"php":   "running",
		"db":    "exited",
		"redis": "running",
	}

	services := desktop.BuildFallbackServicesForTest(discovered, serviceState)
	if len(services) != 4 {
		t.Fatalf("expected 4 fallback services, got %d", len(services))
	}

	statusByTarget := map[string]string{}
	for _, service := range services {
		statusByTarget[service.Target] = service.Status
	}

	if statusByTarget["web"] != "running" {
		t.Fatalf("expected web running, got %q", statusByTarget["web"])
	}
	if statusByTarget["php"] != "running" {
		t.Fatalf("expected php running, got %q", statusByTarget["php"])
	}
	if statusByTarget["db"] != "exited" {
		t.Fatalf("expected db exited, got %q", statusByTarget["db"])
	}
	if statusByTarget["redis"] != "running" {
		t.Fatalf("expected redis running, got %q", statusByTarget["redis"])
	}
}
