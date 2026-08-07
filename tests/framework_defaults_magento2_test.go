package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestFrameworkDefaultsMagento2(t *testing.T) {
	config, ok := engine.GetFrameworkConfig("magento2")
	if !ok {
		t.Fatal("Expected magento2 framework config")
	}

	if config.DefaultCache != "redis" {
		t.Fatalf("Expected DefaultCache redis, got %s", config.DefaultCache)
	}

	if config.DefaultSearch != "opensearch" {
		t.Fatalf("Expected DefaultSearch opensearch, got %s", config.DefaultSearch)
	}
	if config.DefaultQueue != "none" {
		t.Fatalf("Expected DefaultQueue none, got %s", config.DefaultQueue)
	}
	if config.DefaultNodeVer != "24" {
		t.Fatalf("Expected DefaultNodeVer 24, got %s", config.DefaultNodeVer)
	}
	if config.DefaultDBVer != "11.8" {
		t.Fatalf("Expected DefaultDBVer 11.8, got %s", config.DefaultDBVer)
	}
	if config.DefaultMySQLVer != "8.4" {
		t.Fatalf("Expected DefaultMySQLVer 8.4, got %s", config.DefaultMySQLVer)
	}
	if config.DefaultCacheVer != "7.4" {
		t.Fatalf("Expected DefaultCacheVer 7.4, got %s", config.DefaultCacheVer)
	}

	if config.DefaultSearchVer != "3.0" {
		t.Fatalf("Expected DefaultSearchVer 3.0, got %s", config.DefaultSearchVer)
	}
	if config.DefaultQueueVer != "4.2" {
		t.Fatalf("Expected DefaultQueueVer 4.2, got %s", config.DefaultQueueVer)
	}
}
