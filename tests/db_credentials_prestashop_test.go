package tests

import (
	"testing"

	"govard/internal/cmd"
)

func TestDefaultDBCredentialsForPrestaShop(t *testing.T) {
	creds := cmd.DefaultDBCredentialsForFrameworkForTest("prestashop")
	if creds.Username != "prestashop" {
		t.Fatalf("expected default username 'prestashop', got %q", creds.Username)
	}
	if creds.Password != "prestashop" {
		t.Fatalf("expected default password 'prestashop', got %q", creds.Password)
	}
	if creds.Database != "prestashop" {
		t.Fatalf("expected default database 'prestashop', got %q", creds.Database)
	}
}
