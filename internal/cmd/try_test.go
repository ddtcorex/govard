package cmd

import (
	"strings"
	"testing"

	"govard/internal/runtime"
)

func TestTryCommandDefaultsToMageOS(t *testing.T) {
	got, err := resolveTryFramework("")
	if err != nil {
		t.Fatalf("resolveTryFramework: %v", err)
	}
	if got != "mageos" {
		t.Fatalf("framework = %q, want mageos", got)
	}
}

func TestTryCommandNormalisesAliases(t *testing.T) {
	got, err := resolveTryFramework("magento")
	if err != nil {
		t.Fatalf("resolveTryFramework: %v", err)
	}
	if got != "magento2" {
		t.Fatalf("framework = %q, want magento2", got)
	}
}

func TestTryCommandRejectsUnknownFramework(t *testing.T) {
	_, err := resolveTryFramework("wordpress2")
	if err == nil {
		t.Fatalf("resolveTryFramework accepted an unknown framework")
	}
	if !strings.Contains(err.Error(), "magento2") {
		t.Fatalf("error = %v, want it to list a valid framework such as magento2", err)
	}
}

func TestTryCommandDeclaresDockerCapability(t *testing.T) {
	want := string(runtime.CapDocker)
	got := tryCmd.Annotations[runtime.AnnotationRequires]
	if got != want {
		t.Fatalf("try requirement = %q, want %q", got, want)
	}
}
