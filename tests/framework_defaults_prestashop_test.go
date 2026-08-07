package tests

import (
	"testing"

	"govard/internal/engine"
)

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
