package cli

import (
	"encoding/json"
	"errors"
)

// EnvelopeSchemaVersion versions the machine-readable error envelope; it must
// never change without a documented migration.
const EnvelopeSchemaVersion = 1

const (
	CodeCapabilityMissingName = "CAPABILITY_MISSING"
	CodeUsageName             = "USAGE"
	CodeConfigName            = "CONFIG"
	CodeErrorName             = "ERROR"
)

// capabilityError is implemented by errors that carry a missing-capability
// payload (see runtime.MissingError).
type capabilityError interface {
	MissingCapability() string
	MissingCapabilityHint() string
}

type EnvelopeError struct {
	Code       string `json:"code"`
	Capability string `json:"capability,omitempty"`
	Command    string `json:"command,omitempty"`
	Message    string `json:"message"`
	Hint       string `json:"hint,omitempty"`
}

type ErrorEnvelope struct {
	SchemaVersion int           `json:"schema_version"`
	OK            bool          `json:"ok"`
	Error         EnvelopeError `json:"error"`
}

// NewErrorEnvelope classifies err by type, never by message text.
func NewErrorEnvelope(command string, err error) ErrorEnvelope {
	entry := EnvelopeError{Code: CodeErrorName, Command: command, Message: err.Error()}
	var capability capabilityError
	var usage *UsageError
	var coded interface{ ExitCode() int }
	switch {
	case errors.As(err, &capability):
		entry.Code = CodeCapabilityMissingName
		entry.Capability = capability.MissingCapability()
		entry.Hint = capability.MissingCapabilityHint()
	case errors.As(err, &usage):
		entry.Code = CodeUsageName
	case errors.As(err, &coded) && coded.ExitCode() == CodeConfig:
		entry.Code = CodeConfigName
	}
	return ErrorEnvelope{SchemaVersion: EnvelopeSchemaVersion, OK: false, Error: entry}
}

func (e ErrorEnvelope) JSON() ([]byte, error) { return json.MarshalIndent(e, "", "  ") }
