package tests

import (
	"errors"
	"testing"

	"govard/internal/engine"
)

func TestRunDoctorDiagnosticsAllPass(t *testing.T) {
	report := engine.RunDoctorDiagnostics(engine.DoctorDependencies{
		CheckDockerStatus:        func() error { return nil },
		CheckDockerComposePlugin: func() error { return nil },
		CheckPortAvailable:       func(port string) bool { return true },
		CheckDiskScratch:         func() error { return nil },
		CheckGovardHomeWritable:  func() error { return nil },
		CheckNetworkConnectivity: func() error { return nil },
		CheckSearchIndexBlock:    func() error { return nil },
		CheckSSHAgentStatus:      func() (string, error) { return "ok", nil },
	})

	if report.Failures != 0 {
		t.Fatalf("expected 0 failures, got %d", report.Failures)
	}
	if report.Warnings != 0 {
		t.Fatalf("expected 0 warnings, got %d", report.Warnings)
	}
	if report.Passed != len(report.Checks) {
		t.Fatalf("expected all checks passed, got passed=%d checks=%d", report.Passed, len(report.Checks))
	}
	if len(report.IssueCards) != 0 {
		t.Fatalf("expected no issue cards for all-pass report, got %d", len(report.IssueCards))
	}
}

func TestRunDoctorDiagnosticsComposeFailure(t *testing.T) {
	report := engine.RunDoctorDiagnostics(engine.DoctorDependencies{
		CheckDockerStatus:        func() error { return nil },
		CheckDockerComposePlugin: func() error { return errors.New("missing compose plugin") },
		CheckPortAvailable:       func(port string) bool { return true },
		CheckDiskScratch:         func() error { return nil },
		CheckGovardHomeWritable:  func() error { return nil },
		CheckNetworkConnectivity: func() error { return nil },
		CheckSearchIndexBlock:    func() error { return nil },
		CheckSSHAgentStatus:      func() (string, error) { return "ok", nil },
	})

	// The Compose plugin gates the stack commands, not the core, so its absence
	// is a warning rather than a blocking failure.
	if report.Failures != 0 {
		t.Fatalf("expected compose failure to be optional, got %d failure(s)", report.Failures)
	}
	if report.Warnings == 0 {
		t.Fatal("expected the compose failure to surface as a warning")
	}
	if len(report.IssueCards) == 0 {
		t.Fatal("expected issue cards for the warning report")
	}

	found := false
	for _, check := range report.Checks {
		if check.ID != "docker.compose" {
			continue
		}
		found = true
		if check.Status != engine.DoctorStatusWarn {
			t.Fatalf("expected docker.compose warn status, got %s", check.Status)
		}
		if check.Required {
			t.Fatal("docker.compose must not be required")
		}
		if check.Severity != "warning" {
			t.Fatalf("expected warning severity, got %s", check.Severity)
		}
		if len(check.Affects) == 0 {
			t.Fatal("docker.compose must name the command groups it blocks")
		}
		if check.SuggestedCommand != "docker compose version" {
			t.Fatalf("expected suggested command docker compose version, got %s", check.SuggestedCommand)
		}
	}
	if !found {
		t.Fatal("expected docker.compose check")
	}
}

func TestRunDoctorDiagnosticsPortAndNetworkWarnings(t *testing.T) {
	report := engine.RunDoctorDiagnostics(engine.DoctorDependencies{
		CheckDockerStatus:        func() error { return nil },
		CheckDockerComposePlugin: func() error { return nil },
		CheckPortAvailable: func(port string) bool {
			return port != "80"
		},
		CheckDiskScratch:         func() error { return nil },
		CheckGovardHomeWritable:  func() error { return nil },
		CheckNetworkConnectivity: func() error { return errors.New("timeout") },
		CheckSearchIndexBlock:    func() error { return nil },
		CheckSSHAgentStatus:      func() (string, error) { return "ok", nil },
	})

	if report.Failures != 0 {
		t.Fatalf("expected 0 failures, got %d", report.Failures)
	}
	if report.Warnings < 2 {
		t.Fatalf("expected at least 2 warnings, got %d", report.Warnings)
	}

	var port80Warn bool
	var networkWarn bool
	for _, check := range report.Checks {
		if check.ID == "host.port.80" && check.Status == engine.DoctorStatusWarn {
			port80Warn = true
		}
		if check.ID == "host.network.outbound" && check.Status == engine.DoctorStatusWarn {
			networkWarn = true
		}
	}
	if !port80Warn {
		t.Fatal("expected warning for host.port.80")
	}
	if !networkWarn {
		t.Fatal("expected warning for host.network.outbound")
	}
	if len(report.IssueCards) < 2 {
		t.Fatalf("expected warning issue cards, got %d", len(report.IssueCards))
	}
}
