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

func TestFrameworkDefaultsEmdash(t *testing.T) {
	config, ok := engine.GetFrameworkConfig("emdash")
	if !ok {
		t.Fatal("Expected emdash framework config")
	}

	if config.DefaultNodeVer != "22" {
		t.Fatalf("Expected DefaultNodeVer 22, got %s", config.DefaultNodeVer)
	}
	if config.DefaultDB != "none" {
		t.Fatalf("Expected DefaultDB none, got %s", config.DefaultDB)
	}
	if config.DefaultWebServer != "none" {
		t.Fatalf("Expected DefaultWebServer none, got %s", config.DefaultWebServer)
	}
}

func TestFrameworkDefaultsPrestaShop(t *testing.T) {
	config, ok := engine.GetFrameworkConfig("prestashop")
	if !ok {
		t.Fatal("Expected prestashop framework config")
	}

	if config.NGINXPUBLIC != "" {
		t.Fatalf("Expected NGINXPUBLIC empty (docroot = project root), got %s", config.NGINXPUBLIC)
	}
	if config.NGINXTemplate != "prestashop.conf" {
		t.Fatalf("Expected NGINXTemplate prestashop.conf, got %s", config.NGINXTemplate)
	}
	if config.DatabaseName != "prestashop" {
		t.Fatalf("Expected DatabaseName prestashop, got %s", config.DatabaseName)
	}
	if config.DefaultPHP != "8.1" {
		t.Fatalf("Expected DefaultPHP 8.1, got %s", config.DefaultPHP)
	}
	if config.DefaultDB != "mariadb" {
		t.Fatalf("Expected DefaultDB mariadb, got %s", config.DefaultDB)
	}
	if config.DefaultDBVer != "10.11" {
		t.Fatalf("Expected DefaultDBVer 10.11, got %s", config.DefaultDBVer)
	}
	if config.DefaultWebServer != "nginx" {
		t.Fatalf("Expected DefaultWebServer nginx, got %s", config.DefaultWebServer)
	}
	if config.DefaultComposerVer != "latest" {
		t.Fatalf("Expected DefaultComposerVer latest, got %s", config.DefaultComposerVer)
	}
}

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
