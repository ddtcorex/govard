package tests

import (
	"testing"

	"govard/internal/engine"
)

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
