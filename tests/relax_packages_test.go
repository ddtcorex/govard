package tests

import (
	"govard/internal/frameworks/magento2"
	"testing"
)

func TestRelaxPackagesFromContent(t *testing.T) {
	content := `{
    "require": {
        "magento/product-community-edition": "2.4.6-p4",
        "symfony/process": "<=5.4.23",
        "laminas/laminas-escaper": "^2.10"
    },
    "require-dev": {
        "phpunit/phpunit": "^9.5",
        "sebastian/comparator": "<=4.0.6",
        "magento/magento-allure-phpunit": "3.0.2"
    }
}`

	// Call the function with empty containerName so it doesn't try to run docker commands
	relaxed := magento2.RelaxPackagesFromContentForTest(content, "")

	expected := []string{
		"symfony/process:*",
		"laminas/laminas-escaper:*",
		"phpunit/phpunit:*",
		"magento/magento-allure-phpunit:*",
		"sebastian/comparator:*",
	}

	found := make(map[string]bool)
	for _, r := range relaxed {
		found[r] = true
	}

	for _, e := range expected {
		if !found[e] {
			t.Errorf("Expected package %s to be relaxed, but it was not", e)
		}
	}

	if len(relaxed) != len(expected) {
		t.Errorf("Expected %d relaxed packages, got %d: %v", len(expected), len(relaxed), relaxed)
	}
}

func TestRelaxPackagesFromContentWithNoMatches(t *testing.T) {
	content := `{
    "require": {
        "some/other-package": "^1.0"
    }
}`
	relaxed := magento2.RelaxPackagesFromContentForTest(content, "")
	if len(relaxed) != 0 {
		t.Errorf("Expected 0 relaxed packages, got %d: %v", len(relaxed), relaxed)
	}
}
