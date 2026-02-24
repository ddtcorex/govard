//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestFrameworkDetectionIntegration(t *testing.T) {
	tests := []struct {
		name              string
		files             map[string]string
		expectedFramework string
		expectedVersion   string
	}{
		{
			name: "Magento 2 Community",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"magento/product-community-edition": "2.4.7",
					},
				}),
			},
			expectedFramework: "magento2",
			expectedVersion:   "2.4.7",
		},
		{
			name: "Magento 2 Enterprise",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"magento/product-enterprise-edition": "2.4.6",
					},
				}),
			},
			expectedFramework: "magento2",
			expectedVersion:   "2.4.6",
		},
		{
			name: "Magento 1 / OpenMage via package",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"openmage/magento-lts": "20.0.0",
					},
				}),
			},
			expectedFramework: "magento1",
			expectedVersion:   "20.0.0",
		},
		{
			name: "Magento 1 via Mage.php",
			files: map[string]string{
				"app/Mage.php": "<?php // Magento 1",
			},
			expectedFramework: "magento1",
			expectedVersion:   "",
		},
		{
			name: "Laravel",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"laravel/framework": "11.0.0",
					},
				}),
			},
			expectedFramework: "laravel",
			expectedVersion:   "11.0.0",
		},
		{
			name: "Next.js",
			files: map[string]string{
				"package.json": MustMarshalJSON(t, map[string]interface{}{
					"dependencies": map[string]string{
						"next": "14.2.0",
					},
				}),
			},
			expectedFramework: "nextjs",
			expectedVersion:   "14.2.0",
		},
		{
			name: "Drupal",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"drupal/core": "10.2.0",
					},
				}),
			},
			expectedFramework: "drupal",
			expectedVersion:   "10.2.0",
		},
		{
			name: "Symfony",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"symfony/framework-bundle": "7.0.0",
					},
				}),
			},
			expectedFramework: "symfony",
			expectedVersion:   "7.0.0",
		},
		{
			name: "Shopware",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"shopware/core": "6.6.0.0",
					},
				}),
			},
			expectedFramework: "shopware",
			expectedVersion:   "6.6.0.0",
		},
		{
			name: "CakePHP",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"cakephp/cakephp": "5.0.0",
					},
				}),
			},
			expectedFramework: "cakephp",
			expectedVersion:   "5.0.0",
		},
		{
			name: "WordPress",
			files: map[string]string{
				"composer.json": MustMarshalJSON(t, map[string]interface{}{
					"require": map[string]string{
						"johnpbloch/wordpress": "6.5.0",
					},
				}),
			},
			expectedFramework: "wordpress",
			expectedVersion:   "6.5.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := NewTestEnvironment(t)
			projectDir := env.CreateTestProject(t, "detection-test", tt.files)

			metadata := engine.DetectFramework(projectDir)

			if metadata.Framework != tt.expectedFramework {
				t.Errorf("Expected framework %s, got %s", tt.expectedFramework, metadata.Framework)
			}
			if metadata.Version != tt.expectedVersion {
				t.Errorf("Expected version %s, got %s", tt.expectedVersion, metadata.Version)
			}
		})
	}
}

func TestFrameworkAutoConfiguration(t *testing.T) {
	tests := []struct {
		name             string
		framework        string
		expectedPHP      string
		expectedNginxPub string
	}{
		{
			name:             "Magento 2",
			framework:        "magento2",
			expectedPHP:      "8.4",
			expectedNginxPub: "/pub",
		},
		{
			name:             "Magento 1",
			framework:        "magento1",
			expectedPHP:      "8.1",
			expectedNginxPub: "",
		},
		{
			name:             "Laravel",
			framework:        "laravel",
			expectedPHP:      "8.4",
			expectedNginxPub: "/public",
		},
		{
			name:             "Next.js",
			framework:        "nextjs",
			expectedPHP:      "",
			expectedNginxPub: "",
		},
		{
			name:             "Drupal",
			framework:        "drupal",
			expectedPHP:      "8.4",
			expectedNginxPub: "/web",
		},
		{
			name:             "Symfony",
			framework:        "symfony",
			expectedPHP:      "8.4",
			expectedNginxPub: "/public",
		},
		{
			name:             "Shopware",
			framework:        "shopware",
			expectedPHP:      "8.4",
			expectedNginxPub: "/public",
		},
		{
			name:             "CakePHP",
			framework:        "cakephp",
			expectedPHP:      "8.4",
			expectedNginxPub: "/webroot",
		},
		{
			name:             "WordPress",
			framework:        "wordpress",
			expectedPHP:      "8.3",
			expectedNginxPub: "/wordpress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fwConfig, ok := engine.GetFrameworkConfig(tt.framework)
			if !ok {
				t.Fatalf("Framework %s not found", tt.framework)
			}

			if fwConfig.DefaultPHP != tt.expectedPHP {
				t.Errorf("Expected PHP version %s, got %s", tt.expectedPHP, fwConfig.DefaultPHP)
			}
			if fwConfig.NGINXPUBLIC != tt.expectedNginxPub {
				t.Errorf("Expected nginx public %s, got %s", tt.expectedNginxPub, fwConfig.NGINXPUBLIC)
			}
		})
	}
}

func TestConfigLayeringIntegration(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		".govard.yml": MustMarshalYAML(t, map[string]interface{}{
			"project_name": "base-project",
			"recipe":       "magento2",
			"stack": map[string]interface{}{
				"php_version": "8.1",
				"features": map[string]interface{}{
					"xdebug": true,
				},
				"services": map[string]interface{}{
					"web_server": "nginx",
					"search":     "none",
					"cache":      "none",
					"queue":      "none",
				},
			},
		}),
		".govard.local.yml": MustMarshalYAML(t, map[string]interface{}{
			"stack": map[string]interface{}{
				"features": map[string]interface{}{
					"xdebug": false,
				},
				"services": map[string]interface{}{
					"cache": "redis",
				},
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "config-layering", files)

	config, _, err := engine.LoadConfigFromDir(projectDir, true)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.Stack.PHPVersion != "8.1" {
		t.Errorf("Expected PHP version 8.1, got %s", config.Stack.PHPVersion)
	}

	if config.Stack.Features.Xdebug {
		t.Error("Expected Xdebug to be disabled by local override")
	}

	if !config.Stack.Features.Redis {
		t.Error("Expected Redis to be enabled by local override")
	}
}

func TestConfigValidationIntegration(t *testing.T) {
	tests := []struct {
		name        string
		config      engine.Config
		expectError bool
	}{
		{
			name: "Valid Magento 2 config",
			config: engine.Config{
				ProjectName: "test",
				Recipe:      "magento2",
				Domain:      "test.test",
				Stack: engine.Stack{
					PHPVersion: "8.3",
					WebServer:  "nginx",
					Services: engine.Services{
						WebServer: "nginx",
						Search:    "none",
						Cache:     "none",
						Queue:     "none",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Missing project name",
			config: engine.Config{
				Recipe: "magento2",
				Domain: "test.test",
				Stack: engine.Stack{
					PHPVersion: "8.3",
					Services: engine.Services{
						WebServer: "nginx",
						Search:    "none",
						Cache:     "none",
						Queue:     "none",
					},
				},
			},
			expectError: true,
		},
		{
			name: "Missing domain",
			config: engine.Config{
				ProjectName: "test",
				Recipe:      "magento2",
				Stack: engine.Stack{
					PHPVersion: "8.3",
					Services: engine.Services{
						WebServer: "nginx",
						Search:    "none",
						Cache:     "none",
						Queue:     "none",
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateConfig(tt.config)
			hasErrors := err != nil

			if hasErrors != tt.expectError {
				if tt.expectError {
					t.Errorf("Expected validation errors, got none")
				} else {
					t.Errorf("Expected no validation errors, got: %v", err)
				}
			}
		})
	}
}

func TestFrameworkConfigOverride(t *testing.T) {
	env := NewTestEnvironment(t)

	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
	}

	projectDir := env.CreateTestProject(t, "framework-override", files)

	config := engine.Config{
		ProjectName: "override-test",
		Recipe:      "magento2",
		Domain:      "override-test.test",
		Stack: engine.Stack{
			WebRoot: "custom-web",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	CreateGovardConfig(t, projectDir, config)
	CopyBlueprints(t, env.BlueprintsPath, filepath.Join(projectDir, "blueprints"))

	err := engine.RenderBlueprint(projectDir, config)
	if err != nil {
		t.Fatalf("Failed to render blueprint: %v", err)
	}

	composePath := engine.ComposeFilePath(projectDir, config.ProjectName)
	content, _ := os.ReadFile(composePath)

	if !strings.Contains(string(content), "custom-web") {
		t.Error("Custom web root was not applied in rendered blueprint")
	}
}
