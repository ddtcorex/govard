package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestDjangoFrameworkConfigDefaults(t *testing.T) {
	fwConfig, ok := engine.GetFrameworkConfig("django")
	if !ok {
		t.Fatal("expected engine.GetFrameworkConfig(\"django\") to return ok=true")
	}
	if fwConfig.Runtime != "python" {
		t.Errorf("Runtime = %q, want %q", fwConfig.Runtime, "python")
	}
	if fwConfig.AppService != "web" {
		t.Errorf("AppService = %q, want %q", fwConfig.AppService, "web")
	}
	if fwConfig.DefaultPHP != "" {
		t.Errorf("DefaultPHP = %q, want empty (no PHP container)", fwConfig.DefaultPHP)
	}
	if fwConfig.DefaultPythonVer != "3.12" {
		t.Errorf("DefaultPythonVer = %q, want %q", fwConfig.DefaultPythonVer, "3.12")
	}
	if fwConfig.DefaultDB != "postgres" {
		t.Errorf("DefaultDB = %q, want %q", fwConfig.DefaultDB, "postgres")
	}
	if fwConfig.DefaultDBVer == "" {
		t.Error("DefaultDBVer should not be empty for django")
	}
	if fwConfig.DatabaseName != "django" {
		t.Errorf("DatabaseName = %q, want %q", fwConfig.DatabaseName, "django")
	}
}

func TestDjangoRequiresPHPIsFalse(t *testing.T) {
	config := engine.Config{Framework: "django"}
	if engine.RequiresPHP(config) {
		t.Error("expected RequiresPHP to be false for django")
	}
}

func TestFrameworkUsesPythonRuntime(t *testing.T) {
	if !engine.FrameworkUsesPythonRuntime("django") {
		t.Error("expected FrameworkUsesPythonRuntime(\"django\") to be true")
	}
	if engine.FrameworkUsesPythonRuntime("laravel") {
		t.Error("expected FrameworkUsesPythonRuntime(\"laravel\") to be false")
	}
}

func TestNormalizeConfigResolvesPythonVersionDefault(t *testing.T) {
	config := engine.Config{Framework: "django"}
	engine.NormalizeConfig(&config, "")
	if config.Stack.PythonVersion != "3.12" {
		t.Errorf("Stack.PythonVersion = %q, want %q", config.Stack.PythonVersion, "3.12")
	}
}

func TestNormalizeConfigPreservesExplicitPythonVersion(t *testing.T) {
	config := engine.Config{Framework: "django", Stack: engine.Stack{PythonVersion: "3.11"}}
	engine.NormalizeConfig(&config, "")
	if config.Stack.PythonVersion != "3.11" {
		t.Errorf("Stack.PythonVersion = %q, want %q (explicit value should win)", config.Stack.PythonVersion, "3.11")
	}
}

func TestRequiredRuntimeImagesForDjango(t *testing.T) {
	config := engine.Config{
		Framework: "django",
		Stack: engine.Stack{
			Services: engine.Services{DB: "postgres"},
		},
	}
	engine.NormalizeConfig(&config, "")

	images := engine.RequiredRuntimeImages(config, "")

	foundPython := false
	for _, image := range images {
		if image == "python:3.12-slim" {
			foundPython = true
		}
		if strings.Contains(image, "nginx:") || strings.Contains(image, "php:") {
			t.Errorf("did not expect a PHP/nginx image for django, got %s", image)
		}
	}
	if !foundPython {
		t.Errorf("expected python:3.12-slim in %v", images)
	}
}

func TestDjangoFrameworkManifestConfig(t *testing.T) {
	manifest, ok := engine.GetFrameworkManifestConfig("django")
	if !ok {
		t.Fatal("expected engine.GetFrameworkManifestConfig(\"django\") to return ok=true")
	}
	if manifest.Paths.LocalMedia == "" {
		t.Error("expected a non-empty local_media path for django")
	}
	if !manifest.Features.SupportsPostClone {
		t.Error("expected supports_post_clone to be true for django")
	}
}
