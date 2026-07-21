package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/conventions"
)

func TestDefaultDBCredentialsForMageOS(t *testing.T) {
	creds := cmd.DefaultDBCredentialsForFrameworkForTest("mageos")
	if creds.Username != "mageos" {
		t.Fatalf("expected default username 'mageos', got %q", creds.Username)
	}
	if creds.Password != "mageos" {
		t.Fatalf("expected default password 'mageos', got %q", creds.Password)
	}
	if creds.Database != "mageos" {
		t.Fatalf("expected default database 'mageos', got %q", creds.Database)
	}
	if creds.Port != conventions.MySQLPort {
		t.Fatalf("expected default port %d, got %d", conventions.MySQLPort, creds.Port)
	}
}

func TestDefaultDBCredentialsForMagento2StillUsesMagentoCredentials(t *testing.T) {
	creds := cmd.DefaultDBCredentialsForFrameworkForTest("magento2")
	if creds.Username != "magento" {
		t.Fatalf("expected default username 'magento', got %q", creds.Username)
	}
	if creds.Password != "magento" {
		t.Fatalf("expected default password 'magento', got %q", creds.Password)
	}
	if creds.Database != "magento" {
		t.Fatalf("expected default database 'magento', got %q", creds.Database)
	}
}
