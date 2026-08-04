package tests

import (
	"testing"

	"govard/internal/desktop"
)

func TestListFrameworksReturnsAllRegisteredFrameworks(t *testing.T) {
	app := &desktop.App{}
	options, err := app.ListFrameworks()
	if err != nil {
		t.Fatalf("ListFrameworks() error: %v", err)
	}
	if len(options) != 15 {
		t.Fatalf("ListFrameworks() returned %d frameworks, want 15", len(options))
	}

	byName := make(map[string]desktop.FrameworkOption, len(options))
	for _, opt := range options {
		byName[opt.Name] = opt
	}

	magento2, ok := byName["magento2"]
	if !ok {
		t.Fatal("expected \"magento2\" in ListFrameworks() result")
	}
	if magento2.DisplayName != "Magento 2" {
		t.Errorf("magento2.DisplayName = %q, want %q", magento2.DisplayName, "Magento 2")
	}
	found := false
	for _, alias := range magento2.Aliases {
		if alias == "magento" {
			found = true
		}
	}
	if !found {
		t.Errorf("magento2.Aliases = %v, want to contain %q", magento2.Aliases, "magento")
	}

	custom, ok := byName["custom"]
	if !ok {
		t.Fatal("expected \"custom\" in ListFrameworks() result")
	}
	if custom.DisplayName != "Custom" {
		t.Errorf("custom.DisplayName = %q, want %q", custom.DisplayName, "Custom")
	}
}
