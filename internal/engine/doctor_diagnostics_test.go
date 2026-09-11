package engine

import (
	"errors"
	"testing"
)

var errDockerDown = errors.New("cannot connect to the docker daemon")

// stubDoctorDependencies injects every probe so the classification tests never
// touch the host.
func stubDoctorDependencies(dockerErr error) DoctorDependencies {
	return DoctorDependencies{
		CheckDockerStatus:        func() error { return dockerErr },
		CheckDockerComposePlugin: func() error { return dockerErr },
		CheckPortAvailable:       func(string) bool { return true },
		CheckDiskScratch:         func() error { return nil },
		CheckGovardHomeWritable:  func() error { return nil },
		CheckNetworkConnectivity: func() error { return nil },
		CheckSearchIndexBlock:    func() error { return nil },
		CheckSSHAgentStatus:      func() (string, error) { return "agent", nil },
		CheckComposeSpam:         func() error { return nil },
		CheckGovardRegistry:      func() error { return nil },
		CheckProfileSync:         func() error { return nil },
		CheckSystemDependencies:  func() []string { return []string{"docker"} },
		CheckRuntimeImages:       func() ([]string, error) { return nil, nil },
		CheckLegacyConfig:        func() error { return nil },
		CheckConfigDrift:         func() error { return nil },
	}
}

func TestDoctorDockerChecksAreOptional(t *testing.T) {
	report := RunDoctorDiagnostics(stubDoctorDependencies(errDockerDown))
	seen := map[string]bool{}
	for _, check := range report.Checks {
		switch check.ID {
		case "docker.daemon", "docker.compose", "host.deps.docker", "host.deps.remote":
			seen[check.ID] = true
			if check.Required {
				t.Fatalf("check %s must not be required", check.ID)
			}
			if check.Severity != "warning" {
				t.Fatalf("check %s severity = %q, want warning", check.ID, check.Severity)
			}
			if len(check.Affects) == 0 {
				t.Fatalf("check %s must name the command groups it blocks", check.ID)
			}
		}
	}
	for _, id := range []string{"docker.daemon", "docker.compose", "host.deps.docker"} {
		if !seen[id] {
			t.Fatalf("check %s missing from the report", id)
		}
	}
	if report.Failures != 0 {
		t.Fatalf("optional failures must not count as blocking: failures=%d", report.Failures)
	}
	if report.Warnings == 0 {
		t.Fatal("optional failures must surface as warnings")
	}
}

func TestDoctorCoreChecksStayRequired(t *testing.T) {
	report := RunDoctorDiagnostics(stubDoctorDependencies(nil))
	for _, check := range report.Checks {
		if check.ID == "host.govard.home" && !check.Required {
			t.Fatal("host.govard.home must stay required")
		}
	}
}

func TestDoctorSplitsSystemDependencies(t *testing.T) {
	deps := stubDoctorDependencies(nil)
	deps.CheckSystemDependencies = func() []string { return []string{"ssh", "rsync"} }
	report := RunDoctorDiagnostics(deps)
	for _, check := range report.Checks {
		if check.ID == "host.deps.remote" && check.Status != DoctorStatusWarn {
			t.Fatalf("host.deps.remote status = %q, want warn", check.Status)
		}
		if check.ID == "host.deps.docker" && check.Status != DoctorStatusPass {
			t.Fatalf("host.deps.docker status = %q, want pass", check.Status)
		}
	}
}
