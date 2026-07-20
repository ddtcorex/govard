package tests

import (
	"testing"

	"govard/internal/conventions"
)

func TestPrestaShopConventionConstants(t *testing.T) {
	if conventions.FrameworkPrestaShop != "prestashop" {
		t.Fatalf("expected FrameworkPrestaShop 'prestashop', got %q", conventions.FrameworkPrestaShop)
	}
	if conventions.DefaultPrestaShopDBUser != "prestashop" {
		t.Fatalf("expected DefaultPrestaShopDBUser 'prestashop', got %q", conventions.DefaultPrestaShopDBUser)
	}
	if conventions.DefaultPrestaShopDBPass != "prestashop" {
		t.Fatalf("expected DefaultPrestaShopDBPass 'prestashop', got %q", conventions.DefaultPrestaShopDBPass)
	}
	if conventions.DefaultPrestaShopDBName != "prestashop" {
		t.Fatalf("expected DefaultPrestaShopDBName 'prestashop', got %q", conventions.DefaultPrestaShopDBName)
	}
	if conventions.DefaultPrestaShopTablePrefix != "ps_" {
		t.Fatalf("expected DefaultPrestaShopTablePrefix 'ps_', got %q", conventions.DefaultPrestaShopTablePrefix)
	}
	if conventions.PrestaShopParametersFile != "app/config/parameters.php" {
		t.Fatalf("expected PrestaShopParametersFile 'app/config/parameters.php', got %q", conventions.PrestaShopParametersFile)
	}
}
