package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/engine"
)

// Capability is one runtime requirement a command can declare.
type Capability string

const (
	CapNone        Capability = "none"
	CapDocker      Capability = "docker"
	CapSSH         Capability = "ssh"
	CapRsync       Capability = "rsync"
	CapCloudflared Capability = "cloudflared"
	CapNet         Capability = "net"
)

// CodeCapabilityMissing is both the machine-readable error code and the
// process exit code for an unsatisfied runtime requirement.
const CodeCapabilityMissing = 3

// MissingError reports one or more unsatisfied capabilities.
type MissingError struct {
	Caps   []Capability
	Detail string
	Hint   string
}

func (e *MissingError) Error() string {
	names := make([]string, 0, len(e.Caps))
	for _, capability := range e.Caps {
		names = append(names, string(capability))
	}
	base := fmt.Sprintf("missing capability %q", strings.Join(names, `", "`))
	if strings.TrimSpace(e.Detail) == "" {
		return base
	}
	return base + ": " + strings.TrimSpace(e.Detail)
}

// ExitCode makes this error self-describing to internal/cli.Code.
func (e *MissingError) ExitCode() int { return CodeCapabilityMissing }

// Injection seam: tests replace these to simulate a host with or without a
// capability. Production always uses the real engine probes.
var (
	lookPath            = exec.LookPath
	dockerStatus        = engine.CheckDockerStatus
	dockerCompose       = engine.CheckDockerComposePlugin
	networkConnectivity = engine.CheckNetworkConnectivity
)

// forcedSatisfied is a test-only override: capabilities listed here always
// probe as available.
var forcedSatisfied map[Capability]bool

// testSatisfiedCapabilitiesEnv names capabilities that must probe as available
// for one process, as a comma-separated list. The integration suite runs the
// real binary against local mock servers, where a live probe — the network dial
// in particular — would make the result depend on the host's connectivity
// instead of on the code under test. Production never sets it; GOVARD_TEST_RUNTIME
// is the same kind of switch.
const testSatisfiedCapabilitiesEnv = "GOVARD_TEST_SATISFIED_CAPABILITIES"

// envForcedSatisfied reports whether the environment forces capability to probe
// as available.
func envForcedSatisfied(capability Capability) bool {
	raw := strings.TrimSpace(os.Getenv(testSatisfiedCapabilitiesEnv))
	if raw == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		if Capability(strings.ToLower(strings.TrimSpace(part))) == capability {
			return true
		}
	}
	return false
}

var capabilityHints = map[Capability]string{
	CapDocker:      "start Docker Desktop/daemon and ensure the current user can reach the Docker socket; commands that need no containers are listed by `govard capabilities`",
	CapSSH:         "install an SSH client (openssh-client) and retry",
	CapRsync:       "install rsync and retry",
	CapCloudflared: "install cloudflared (see the Govard tunnel documentation) and retry",
	CapNet:         "check outbound network connectivity and retry",
}

// Probe reports the unsatisfied capabilities, or nil when all are met.
func Probe(caps ...Capability) error {
	missing := make([]Capability, 0, len(caps))
	details := make([]string, 0, len(caps))
	prefixed := make([]string, 0, len(caps))
	var hint string
	for _, capability := range caps {
		detail := probeOne(capability)
		if detail == "" {
			continue
		}
		missing = append(missing, capability)
		details = append(details, detail)
		prefixed = append(prefixed, fmt.Sprintf("%s: %s", capability, detail))
		if hint == "" {
			hint = capabilityHints[capability]
		}
	}
	if len(missing) == 0 {
		return nil
	}
	// A single failure reads best without repeating the capability name.
	detail := details[0]
	if len(missing) > 1 {
		detail = strings.Join(prefixed, "; ")
	}
	return &MissingError{Caps: missing, Detail: detail, Hint: hint}
}

// probeOne returns an empty string when the capability is satisfied.
func probeOne(capability Capability) string {
	if forcedSatisfied[capability] || envForcedSatisfied(capability) {
		return ""
	}
	ctx := context.Background()
	switch capability {
	case CapNone:
		return ""
	case CapDocker:
		if _, err := lookPath("docker"); err != nil {
			return err.Error()
		}
		if err := dockerStatus(ctx); err != nil {
			return err.Error()
		}
		if err := dockerCompose(ctx); err != nil {
			return err.Error()
		}
		return ""
	case CapSSH:
		if _, err := lookPath("ssh"); err != nil {
			return err.Error()
		}
		return ""
	case CapRsync:
		if _, err := lookPath("rsync"); err != nil {
			return err.Error()
		}
		return ""
	case CapCloudflared:
		if _, err := lookPath("cloudflared"); err != nil {
			return err.Error()
		}
		return ""
	case CapNet:
		if err := networkConnectivity(); err != nil {
			return err.Error()
		}
		return ""
	default:
		return fmt.Sprintf("unknown capability %q", capability)
	}
}

// StubProbesForTest replaces the docker probes for the duration of a test and
// returns a restore function. It is exported because tests outside this package
// (internal/cmd's gate test) need it; production code never calls it.
func StubProbesForTest(status, compose func(context.Context) error) func() {
	previousStatus, previousCompose := dockerStatus, dockerCompose
	if status != nil {
		dockerStatus = status
	}
	if compose != nil {
		dockerCompose = compose
	}
	return func() {
		dockerStatus, dockerCompose = previousStatus, previousCompose
	}
}

// MissingCapability names the first unsatisfied capability. It lets
// internal/cli build the machine-readable error envelope without importing
// this package.
func (e *MissingError) MissingCapability() string {
	if len(e.Caps) == 0 {
		return ""
	}
	return string(e.Caps[0])
}

// MissingCapabilityHint returns the operator-facing remediation hint.
func (e *MissingError) MissingCapabilityHint() string { return e.Hint }

// StubSatisfiedCapabilitiesForTest forces the listed capabilities to probe as
// available for the duration of a test and returns a restore function. Command
// tests use it when they exercise command logic, not capability detection.
func StubSatisfiedCapabilitiesForTest(satisfied ...Capability) func() {
	previous := forcedSatisfied
	forced := make(map[Capability]bool, len(satisfied))
	for _, capability := range satisfied {
		forced[capability] = true
	}
	forcedSatisfied = forced
	return func() { forcedSatisfied = previous }
}

// StubLookPathForTest replaces the executable lookup for the duration of a test
// and returns a restore function.
func StubLookPathForTest(lookup func(string) (string, error)) func() {
	previous := lookPath
	if lookup != nil {
		lookPath = lookup
	}
	return func() { lookPath = previous }
}
