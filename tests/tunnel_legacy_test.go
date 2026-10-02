package tests

import (
	"os"
	"strings"
	"testing"

	"govard/internal/cmd"
)

// A cloudflared started by govard v1.77.0 left no PID record. After the upgrade
// it is still running, and `tunnel stop` must not look like it worked.

const legacyQuickTunnelArgv = "cloudflared tunnel --url https://demo.test --no-tls-verify"

func legacyHost(t *testing.T, processes ...cmd.TunnelHostProcess) {
	t.Helper()
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		ListProcesses: func() []cmd.TunnelHostProcess { return processes },
		SignalProcess: func(pid int, sig os.Signal) error {
			t.Fatalf("govard must never signal pid %d it did not start", pid)
			return nil
		},
	})
	t.Cleanup(restore)
}

func TestTunnelStopWarnsAboutAnUnrecordedCloudflaredAndFails(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	legacyHost(t, cmd.TunnelHostProcess{PID: 4242, Argv: legacyQuickTunnelArgv})

	out, err := runTunnel(t, "tunnel", "stop")
	if err == nil {
		t.Fatalf("stop must not report success while an unattributed cloudflared runs, output: %q", out)
	}
	for _, want := range []string{"4242", "kill 4242"} {
		if !strings.Contains(out+err.Error(), want) {
			t.Fatalf("output must contain %q, got: %q / %v", want, out, err)
		}
	}
	if !strings.Contains(out, "Reverting base URL") {
		t.Fatalf("the base URL revert must still run, got: %q", out)
	}
}

func TestTunnelStopIgnoresCloudflaredThatIsNotAGovardQuickTunnel(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	legacyHost(t, cmd.TunnelHostProcess{PID: 77, Argv: "cloudflared --no-autoupdate --config /etc/cloudflared/x.yml tunnel run"})

	if out, err := runTunnel(t, "tunnel", "stop"); err != nil {
		t.Fatalf("an unrelated cloudflared unit must not fail stop: %v (%q)", err, out)
	}
}

func TestTunnelStopDoesNotBlameAProcessAnotherProjectRecorded(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	writeTunnelRecord(t, "other", 5151, legacyQuickTunnelArgv)
	legacyHost(t, cmd.TunnelHostProcess{PID: 5151, Argv: legacyQuickTunnelArgv})

	if out, err := runTunnel(t, "tunnel", "stop"); err != nil {
		t.Fatalf("a process with a record is attributed, not legacy: %v (%q)", err, out)
	}
}

func TestTunnelStatusHintsAtAnUnrecordedCloudflared(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	legacyHost(t, cmd.TunnelHostProcess{PID: 4242, Argv: legacyQuickTunnelArgv})

	out, err := runTunnel(t, "tunnel", "status")
	if err != nil {
		t.Fatalf("status is informational and must exit 0: %v", err)
	}
	if !strings.Contains(out, "INACTIVE") || !strings.Contains(out, "kill 4242") {
		t.Fatalf("status must stay INACTIVE yet name the process and the kill command, got: %q", out)
	}
}

func TestTunnelStopStaysSilentForAnUnrelatedQuickTunnel(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	legacyHost(t, cmd.TunnelHostProcess{PID: 88, Argv: "cloudflared tunnel --url http://localhost:3000"})

	out, err := runTunnel(t, "tunnel", "stop")
	if err != nil {
		t.Fatalf("a user's own quick tunnel must not fail stop: %v (%q)", err, out)
	}
	if strings.Contains(out, "kill 88") || strings.Contains(out, "cannot attribute") {
		t.Fatalf("an unrelated tunnel must not be reported, got: %q", out)
	}
}

func TestTunnelStopStaysSilentForAnotherProjectsQuickTunnel(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	legacyHost(t, cmd.TunnelHostProcess{PID: 99, Argv: "cloudflared tunnel --url https://other-shop.test --no-tls-verify"})

	out, err := runTunnel(t, "tunnel", "stop")
	if err != nil {
		t.Fatalf("another project's tunnel must not fail stop: %v (%q)", err, out)
	}
	if strings.Contains(out, "kill 99") {
		t.Fatalf("another project's tunnel must not be reported, got: %q", out)
	}
}

func TestTunnelStatusStaysSilentForUnrelatedQuickTunnels(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	legacyHost(t,
		cmd.TunnelHostProcess{PID: 88, Argv: "cloudflared tunnel --url http://localhost:3000"},
		cmd.TunnelHostProcess{PID: 99, Argv: "cloudflared tunnel --url https://other-shop.test"})

	out, err := runTunnel(t, "tunnel", "status")
	if err != nil {
		t.Fatalf("status must exit 0: %v", err)
	}
	if strings.Contains(out, "kill ") {
		t.Fatalf("status must not hint at unrelated tunnels, got: %q", out)
	}
}
