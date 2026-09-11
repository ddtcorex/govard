package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// fakeCapabilityError stands in for runtime.MissingError so this package stays
// dependency-free.
type fakeCapabilityError struct{}

func (fakeCapabilityError) Error() string                 { return `missing capability "docker"` }
func (fakeCapabilityError) MissingCapability() string     { return "docker" }
func (fakeCapabilityError) MissingCapabilityHint() string { return "start Docker" }

func TestErrorEnvelopeForMissingCapability(t *testing.T) {
	err := fmt.Errorf("cannot run %q: %w", "govard env up", fakeCapabilityError{})
	env := NewErrorEnvelope("govard env up", err)
	if env.SchemaVersion != EnvelopeSchemaVersion || env.OK {
		t.Fatalf("envelope = %#v", env)
	}
	if env.Error.Code != CodeCapabilityMissingName {
		t.Fatalf("code = %q, want %q", env.Error.Code, CodeCapabilityMissingName)
	}
	if env.Error.Capability != "docker" || env.Error.Hint != "start Docker" {
		t.Fatalf("envelope error = %#v", env.Error)
	}
	raw, marshalErr := env.JSON()
	if marshalErr != nil {
		t.Fatalf("JSON() error = %v", marshalErr)
	}
	var round map[string]any
	if unmarshalErr := json.Unmarshal(raw, &round); unmarshalErr != nil {
		t.Fatalf("round trip error = %v", unmarshalErr)
	}
	if round["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %v, want 1", round["schema_version"])
	}
}

func TestErrorEnvelopeForUsageError(t *testing.T) {
	env := NewErrorEnvelope("govard up", &UsageError{Err: errors.New("unknown flag: --nope")})
	if env.Error.Code != CodeUsageName {
		t.Fatalf("code = %q, want %q", env.Error.Code, CodeUsageName)
	}
}

func TestErrorEnvelopeForPlainError(t *testing.T) {
	env := NewErrorEnvelope("govard status", errors.New("boom"))
	if env.Error.Code != CodeErrorName {
		t.Fatalf("code = %q, want %q", env.Error.Code, CodeErrorName)
	}
}
