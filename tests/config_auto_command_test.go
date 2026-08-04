package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks/types"

	"github.com/spf13/cobra"
)

func TestApplyFrameworkAutoConfigurationUsesMagento1Handler(t *testing.T) {
	called := false
	restore := cmd.SetFrameworkLookupForAutoConfigureForTest(func(name string) (types.FrameworkDefinition, bool) {
		if name != "magento1" {
			t.Fatalf("expected lookup for magento1, got %s", name)
		}
		return types.FrameworkDefinition{
			AutoConfigure: func(_ *cobra.Command, config engine.Config) error {
				called = true
				if config.ProjectName != "sample-project" {
					t.Fatalf("expected project name sample-project, got %s", config.ProjectName)
				}
				if config.Framework != "magento1" {
					t.Fatalf("expected magento1 config, got %s", config.Framework)
				}
				return nil
			},
		}, true
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
	restore := cmd.SetFrameworkLookupForAutoConfigureForTest(func(name string) (types.FrameworkDefinition, bool) {
		if name != "mageos" {
			t.Fatalf("expected lookup for mageos, got %s", name)
		}
		return types.FrameworkDefinition{
			AutoConfigure: func(_ *cobra.Command, config engine.Config) error {
				called = true
				if config.ProjectName != "sample-project" {
					t.Fatalf("expected project name sample-project, got %s", config.ProjectName)
				}
				if config.Framework != "mageos" {
					t.Fatalf("expected mageos config, got %s", config.Framework)
				}
				return nil
			},
		}, true
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

func TestPrepareFrameworkComposerUsesDefinitionHook(t *testing.T) {
	called := false
	restore := cmd.SetFrameworkLookupForBootstrapForTest(func(name string) (types.FrameworkDefinition, bool) {
		if name != "wordpress" {
			t.Fatalf("expected lookup for wordpress, got %s", name)
		}
		return types.FrameworkDefinition{
			PrepareComposer: func(config engine.Config) error {
				called = config.ProjectName == "sample-project"
				return nil
			},
		}, true
	})
	defer restore()

	if err := cmd.PrepareFrameworkComposerForTest(engine.Config{ProjectName: "sample-project", Framework: "wordpress"}); err != nil {
		t.Fatalf("PrepareFrameworkComposerForTest() error = %v", err)
	}
	if !called {
		t.Fatal("expected framework composer preparation hook to run")
	}
}
