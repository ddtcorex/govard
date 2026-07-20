package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestMagerunFrameworkSupport(t *testing.T) {
	for _, tt := range []struct {
		name      string
		command   string
		framework string
		wantErr   bool
	}{
		{name: "M1 magerun", command: "magerun", framework: "magento1", wantErr: false},
		{name: "M2 magerun", command: "magerun", framework: "magento2", wantErr: false},
		{name: "M2 mr alias", command: "mr", framework: "magento2", wantErr: false},
		{name: "Laravel magerun fail", command: "magerun", framework: "laravel", wantErr: true},
		{name: "Magento artisan fail", command: "artisan", framework: "magento2", wantErr: true},
		{name: "Composer all allowed", command: "composer", framework: "magento2", wantErr: false},
		{name: "Composer all allowed laravel", command: "composer", framework: "laravel", wantErr: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := cmd.ValidateFrameworkForCommandForTest(tt.command, engine.Config{Framework: tt.framework})
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFrameworkForCommandForTest(%q, %q) error = %v, wantErr %v", tt.command, tt.framework, err, tt.wantErr)
			}
		})
	}
}

func TestPrestaShopToolCommand(t *testing.T) {
	err := cmd.ValidateFrameworkForCommandForTest("prestashop", engine.Config{Framework: "prestashop"})
	if err != nil {
		t.Errorf("ValidateFrameworkForCommandForTest(prestashop, prestashop) error = %v, want nil", err)
	}

	err = cmd.ValidateFrameworkForCommandForTest("prestashop", engine.Config{Framework: "laravel"})
	if err == nil {
		t.Error("ValidateFrameworkForCommandForTest(prestashop, laravel) expected error, got nil")
	}
}
