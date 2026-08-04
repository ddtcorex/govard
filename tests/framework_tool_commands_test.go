package tests

import (
	"reflect"
	"testing"

	"govard/internal/cmd"
)

// This catches framework CLI availability drifting from its owning framework
// definitions (the former static cmd list made Mage-OS/OpenMage inheritance
// easy to miss).
func TestFrameworkToolCommandsComeFromFrameworkDefinitions(t *testing.T) {
	commands := cmd.FrameworkToolCommandsForTest()
	byName := make(map[string]cmd.FrameworkCommand, len(commands))
	for _, command := range commands {
		byName[command.Name] = command
	}

	magerun, ok := byName["magerun"]
	if !ok {
		t.Fatal("expected magerun command")
	}
	if !reflect.DeepEqual(magerun.Frameworks, []string{"magento1", "magento2", "mageos", "openmage"}) {
		t.Fatalf("magerun frameworks = %v, want Magento family", magerun.Frameworks)
	}

	artisan, ok := byName["artisan"]
	if !ok {
		t.Fatal("expected artisan command")
	}
	if !reflect.DeepEqual(artisan.Frameworks, []string{"laravel"}) {
		t.Fatalf("artisan frameworks = %v, want [laravel]", artisan.Frameworks)
	}

	if _, found := byName["composer"]; found {
		t.Fatal("composer is a generic command and must not be published by a framework definition")
	}
}
