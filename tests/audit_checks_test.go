package tests

import (
	"reflect"
	"testing"

	"govard/internal/audit"
)

func TestNormalizeAuditChecksDefaultsToLint(t *testing.T) {
	got, err := audit.NormalizeChecks(nil)
	if err != nil {
		t.Fatalf("NormalizeChecks returned error: %v", err)
	}
	if want := []string{"lint"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized checks = %#v, want %#v", got, want)
	}
}

func TestNormalizeAuditChecksSupportsProfilerAndDeduplicates(t *testing.T) {
	got, err := audit.NormalizeChecks([]string{" profiler ", "lint", "profiler", " lint "})
	if err != nil {
		t.Fatalf("NormalizeChecks returned error: %v", err)
	}
	if want := []string{"profiler", "lint"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized checks = %#v, want %#v", got, want)
	}
}
