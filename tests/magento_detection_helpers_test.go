package tests

import (
	"govard/internal/frameworks/magento2"
	"os"
	"testing"
)

func TestIsMagentoElasticsuiteProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "govard-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origCwd) }()

	// Test case 1: smile/elasticsuite in composer.json
	composerContent := `{"require": {"smile/elasticsuite": "^2.11"}}`
	if err := os.WriteFile("composer.json", []byte(composerContent), 0644); err != nil {
		t.Fatalf("Failed to write composer.json: %v", err)
	}

	if !magento2.IsMagentoElasticsuiteProjectForTest() {
		t.Errorf("Expected elasticsuite detection via composer.json to be true")
	}

	// Test case 2: Clear composer.json, test app/etc/config.php
	if err := os.WriteFile("composer.json", []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to write composer.json: %v", err)
	}
	if err := os.MkdirAll("app/etc", 0755); err != nil {
		t.Fatalf("Failed to mkdir: %v", err)
	}
	configContent := "<?php return ['modules' => ['Smile_ElasticsuiteCore' => 1]];"
	if err := os.WriteFile("app/etc/config.php", []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config.php: %v", err)
	}

	if !magento2.IsMagentoElasticsuiteProjectForTest() {
		t.Errorf("Expected elasticsuite detection via config.php to be true")
	}
}

func TestIsMagentoConfigPathUnavailable(t *testing.T) {
	tests := []struct {
		output   string
		expected bool
	}{
		{"path \"catalog/search/engine\" doesn't exist", true},
		{"Path \"twofactorauth/general/enable\" not found", true},
		{"The \"twofactorauth/general/enable\" configuration path is not defined", true},
		{"some other error", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := magento2.IsMagentoConfigPathUnavailableForTest(tt.output); got != tt.expected {
			t.Errorf("IsMagentoConfigPathUnavailable(%q) = %v; want %v", tt.output, got, tt.expected)
		}
	}
}
