package tests

import (
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks"
)

func TestBootstrapDispatcherAllFrameworks(t *testing.T) {
	frameworkNames := []string{
		"magento2",
		"mageos",
		"magento1",
		"openmage",
		"symfony",
		"laravel",
		"drupal",
		"wordpress",
		"nextjs",
		"emdash",
		"shopware",
		"cakephp",
		"prestashop",
		"django",
		"dagster",
	}

	opts := bootstrap.DefaultOptions()

	for _, fw := range frameworkNames {
		t.Run(fw, func(t *testing.T) {
			err := frameworks.RunBootstrap(fw, opts)
			if err != nil {
				t.Fatalf("Run(%s) failed: %v", fw, err)
			}
		})
	}
}
