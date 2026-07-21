package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestApplyFrameworkAutoConfigurationUsesMagento1Handler(t *testing.T) {
	called := false
	restore := cmd.SetMagento1AutoConfigurationRunnerForTest(func(projectName string, config engine.Config) error {
		called = true
		if projectName != "sample-project" {
			t.Fatalf("expected project name sample-project, got %s", projectName)
		}
		if config.Framework != "magento1" {
			t.Fatalf("expected magento1 config, got %s", config.Framework)
		}
		return nil
	})
	defer restore()

	if err := cmd.ApplyFrameworkAutoConfigurationForTest(engine.Config{
		ProjectName: "sample-project",
		Framework:   "magento1",
		Domain:      "sample.test",
	}); err != nil {
		t.Fatalf("applyFrameworkAutoConfiguration returned error: %v", err)
	}

	if !called {
		t.Fatal("expected Magento 1 auto configuration runner to be invoked")
	}
}

func TestApplyFrameworkAutoConfigurationUsesMagento2HandlerForMageOS(t *testing.T) {
	called := false
	restore := cmd.SetMagento2AutoConfigurationRunnerForTest(func(projectName string, config engine.Config, force bool) error {
		called = true
		if projectName != "sample-project" {
			t.Fatalf("expected project name sample-project, got %s", projectName)
		}
		if config.Framework != "mageos" {
			t.Fatalf("expected mageos config, got %s", config.Framework)
		}
		return nil
	})
	defer restore()

	if err := cmd.ApplyFrameworkAutoConfigurationForTest(engine.Config{
		ProjectName: "sample-project",
		Framework:   "mageos",
		Domain:      "sample.test",
		Stack: engine.Stack{
			Services: engine.Services{Search: "none"},
		},
	}); err != nil {
		t.Fatalf("applyFrameworkAutoConfiguration returned error: %v", err)
	}

	if !called {
		t.Fatal("expected Magento 2 auto configuration runner to be invoked for mageos")
	}
}
