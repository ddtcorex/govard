package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestFrameworkDefaultsNonMagento2DisableCacheAndSearch(t *testing.T) {
	frameworks := []string{
		"laravel",
		"nextjs",
		"emdash",
		"drupal",
		"symfony",
		"magento1",
		"openmage",
		"shopware",
		"cakephp",
		"wordpress",
		"prestashop",
		"custom",
		"dagster",
		"django",
	}

	for _, framework := range frameworks {
		config, ok := engine.GetFrameworkConfig(framework)
		if !ok {
			t.Fatalf("expected %s framework config", framework)
		}
		if config.DefaultCache != "none" {
			t.Fatalf("expected %s DefaultCache none, got %s", framework, config.DefaultCache)
		}
		if config.DefaultSearch != "none" {
			t.Fatalf("expected %s DefaultSearch none, got %s", framework, config.DefaultSearch)
		}
	}
}
