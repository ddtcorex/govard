package tests

import (
	"testing"

	"govard/internal/conventions"
	"govard/internal/frameworks/prestashop"
)

func TestPrestaShopConventionConstants(t *testing.T) {
	if prestashop.DefaultDBUser != "prestashop" {
		t.Fatalf("expected DefaultDBUser 'prestashop', got %q", prestashop.DefaultDBUser)
	}
	if prestashop.DefaultDBPass != "prestashop" {
		t.Fatalf("expected DefaultDBPass 'prestashop', got %q", prestashop.DefaultDBPass)
	}
	if prestashop.DefaultDBName != "prestashop" {
		t.Fatalf("expected DefaultDBName 'prestashop', got %q", prestashop.DefaultDBName)
	}
	if prestashop.DefaultTablePrefix != "ps_" {
		t.Fatalf("expected DefaultTablePrefix 'ps_', got %q", prestashop.DefaultTablePrefix)
	}
	if conventions.PrestaShopParametersFile != "app/config/parameters.php" {
		t.Fatalf("expected PrestaShopParametersFile 'app/config/parameters.php', got %q", conventions.PrestaShopParametersFile)
	}
}
