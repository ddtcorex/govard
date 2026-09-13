package tests

import (
	"testing"

	"govard/internal/engine"
)

func TestWordpressDiscovery(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"composer.json": composerJSON(t, map[string]string{
			"johnpbloch/wordpress": "6.0.0",
		}),
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "wordpress" {
		t.Errorf("Expected framework wordpress, got %s", metadata.Framework)
	}
}

// A classic WordPress checkout has no composer.json at all: the core files sit
// in the repository root and `wp-content/` holds the project. Detection used to
// look only for Composer-managed WordPress, so the largest kind of WordPress
// project there is — the one the deploy recipe targets — came back as `custom`,
// and a recipe the framework owns would never have run.
//
// The marker is the core loader rather than `wp-config.php`: a Bedrock project
// also has a `wp-config.php`-shaped file, but only a classic tree has
// `wp-load.php` in the root, where the served docroot is.
func TestWordpressDiscoveryForAClassicCheckout(t *testing.T) {
	testDir := tempProject(t, map[string]string{
		"wp-load.php":             "<?php require __DIR__ . '/wp-settings.php';",
		"wp-includes/version.php": "$wp_version = '6.9.4';",
		"wp-config.php":           "<?php define('DB_NAME', 'wordpress');",
	})

	metadata := engine.DetectFramework(testDir)
	if metadata.Framework != "wordpress" {
		t.Errorf("Expected framework wordpress for a classic checkout, got %q", metadata.Framework)
	}
}
