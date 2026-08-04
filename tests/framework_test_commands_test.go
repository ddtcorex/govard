package tests

import (
	"reflect"
	"testing"

	"govard/internal/cmd"
)

// This catches framework test-suite routing drifting back into cmd switches.
func TestFrameworkTestCommandsComeFromDefinitions(t *testing.T) {
	laravelDefault, ok := cmd.FrameworkTestCommandForTest("laravel", "default")
	if !ok {
		t.Fatal("expected Laravel default test command")
	}
	if laravelDefault.Binary != "php" || !reflect.DeepEqual(laravelDefault.Args, []string{"artisan", "test"}) {
		t.Fatalf("Laravel default test command = %#v, want php artisan test", laravelDefault)
	}

	mageOSMFTF, ok := cmd.FrameworkTestCommandForTest("mageos", "mftf")
	if !ok {
		t.Fatal("expected Mage-OS to inherit Magento MFTF test command")
	}
	if mageOSMFTF.Binary != "php" || !reflect.DeepEqual(mageOSMFTF.Args, []string{"vendor/bin/mftf", "run:group"}) {
		t.Fatalf("Mage-OS MFTF command = %#v, want php vendor/bin/mftf run:group", mageOSMFTF)
	}

	if _, ok := cmd.FrameworkTestCommandForTest("wordpress", "integration"); ok {
		t.Fatal("WordPress must not expose Magento integration tests")
	}
}
