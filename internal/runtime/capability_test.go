package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestProbeDockerReportsMissingCapability(t *testing.T) {
	restore := stubProbes()
	defer restore()
	dockerStatus = func(context.Context) error { return errors.New("Cannot connect to the Docker daemon") }

	err := Probe(CapDocker)
	var missing *MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Probe error = %v, want *MissingError", err)
	}
	if len(missing.Caps) != 1 || missing.Caps[0] != CapDocker {
		t.Fatalf("missing caps = %#v, want [docker]", missing.Caps)
	}
	if missing.ExitCode() != CodeCapabilityMissing {
		t.Fatalf("ExitCode() = %d, want %d", missing.ExitCode(), CodeCapabilityMissing)
	}
	if want := `missing capability "docker": Cannot connect to the Docker daemon`; missing.Error() != want {
		t.Fatalf("Error() = %q, want %q", missing.Error(), want)
	}
}

func TestProbeDockerSatisfiedIsNil(t *testing.T) {
	restore := stubProbes()
	defer restore()
	dockerStatus = func(context.Context) error { return nil }
	dockerCompose = func(context.Context) error { return nil }
	lookPath = func(string) (string, error) { return "/usr/bin/docker", nil }
	if err := Probe(CapDocker); err != nil {
		t.Fatalf("Probe(CapDocker) = %v, want nil", err)
	}
}

func TestProbeDockerRequiresCLIAndComposePlugin(t *testing.T) {
	restore := stubProbes()
	defer restore()
	dockerStatus = func(context.Context) error { return nil }
	dockerCompose = func(context.Context) error { return errors.New(`exec: "docker": executable file not found in $PATH`) }
	lookPath = func(string) (string, error) { return "/usr/bin/docker", nil }

	err := Probe(CapDocker)
	var missing *MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("Probe error = %v, want *MissingError", err)
	}
	if missing.Hint == "" {
		t.Fatal("MissingError.Hint is empty")
	}
}

func TestProbeNoneIsAlwaysSatisfied(t *testing.T) {
	if err := Probe(CapNone); err != nil {
		t.Fatalf("Probe(CapNone) = %v, want nil", err)
	}
}

func stubProbes() func() {
	prevStatus, prevCompose, prevNet, prevLook := dockerStatus, dockerCompose, networkConnectivity, lookPath
	return func() {
		dockerStatus, dockerCompose, networkConnectivity, lookPath = prevStatus, prevCompose, prevNet, prevLook
	}
}

func TestStubSatisfiedCapabilitiesForTest(t *testing.T) {
	restoreProbes := stubProbes()
	defer restoreProbes()
	dockerStatus = func(context.Context) error { return errors.New("cannot connect to the docker daemon") }

	restoreCapabilities := StubSatisfiedCapabilitiesForTest(CapDocker)
	if err := Probe(CapDocker); err != nil {
		t.Fatalf("Probe(CapDocker) with forced capability = %v, want nil", err)
	}
	restoreCapabilities()
	if err := Probe(CapDocker); err == nil {
		t.Fatal("Probe(CapDocker) after restore = nil, want the stubbed failure")
	}
}
