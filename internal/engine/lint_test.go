package engine

import (
	"strings"
	"testing"
)

func contains(values []string, pattern string) bool {
	for _, value := range values {
		if value == pattern || strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func TestMagelintIgnoreQuickDeep(t *testing.T) {
	quick := LintIgnore(true)
	if contains(quick, "vendor") == false {
		t.Fatal("quick must ignore vendor")
	}
	if contains(quick, "dev/tests") == false {
		t.Fatal("quick must ignore dev/tests")
	}
	if contains(quick, "pub/media") == false {
		t.Fatal("always ignore pub/media")
	}
	deep := LintIgnore(false)
	if contains(deep, "vendor") {
		t.Fatal("deep must not ignore vendor")
	}
	if contains(deep, "pub/media") == false {
		t.Fatal("deep must still ignore pub/media")
	}
}

func TestStableVolumeKey(t *testing.T) {
	a := StableVolumeKey("bebe9")
	b := StableVolumeKey("bebe9")
	if a != b {
		t.Fatal("stable")
	}
	if StableVolumeKey("bebe9") == StableVolumeKey("bebe9-123") {
		t.Fatal("must differ")
	}
}
