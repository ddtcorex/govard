package tests

import (
	"govard/internal/engine"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestInitCommandLogic(t *testing.T) {
	// Create a temp directory for simulation
	tempDir, err := os.MkdirTemp("", "govard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Simulate Magento 2 project
	composerJson := `{"require": {"magento/product-community-edition": "2.4.7"}}`
	_ = os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composerJson), 0644)

	// Detect framework
	metadata := engine.DetectFramework(tempDir)

	// Create config (Simulating what internal/cmd/init.go does)
	config := engine.Config{
		ProjectName: filepath.Base(tempDir),
		Framework:   metadata.Framework,
		Stack: engine.Stack{
			PHPVersion: "8.1",
			WebServer:  "nginx",
			Features: engine.Features{
				Xdebug:  true,
				Redis:   true,
				Varnish: false,
			},
		},
	}

	// Marshall to YAML
	data, err := yaml.Marshal(&config)
	if err != nil {
		t.Fatal(err)
	}

	configFile := filepath.Join(tempDir, ".govard.yml")
	err = os.WriteFile(configFile, data, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the file exists and content is correct
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error(".govard.yml was not created")
	}

	var savedConfig engine.Config
	savedData, _ := os.ReadFile(configFile)
	_ = yaml.Unmarshal(savedData, &savedConfig)

	if savedConfig.Framework != "magento2" {
		t.Errorf("Expected framework magento2, got %s", savedConfig.Framework)
	}
	if savedConfig.Stack.Features.Varnish != false {
		t.Error("Varnish should be disabled by default")
	}
}
