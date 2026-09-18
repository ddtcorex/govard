package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/desktop"

	dockertypes "github.com/docker/docker/api/types/container"
)

func TestDesktopGlobalServiceDefinitionsIncludeDNSMasqForTest(t *testing.T) {
	desktop.ResetStateForTest()

	service, ok := desktop.ResolveGlobalServiceForTest("dnsmasq")
	if !ok {
		t.Fatalf("expected dnsmasq global service definition")
	}
	if service.ComposeService != "dnsmasq" {
		t.Fatalf("expected compose service dnsmasq, got %q", service.ComposeService)
	}
	if service.ContainerName != "govard-proxy-dnsmasq" {
		t.Fatalf("expected dnsmasq container name, got %q", service.ContainerName)
	}
	if service.Openable {
		t.Fatalf("dnsmasq should not be openable in desktop UI")
	}
}

func TestDesktopGlobalServiceStatusDerivationForTest(t *testing.T) {
	status, health, running := desktop.DeriveGlobalContainerStatusForTest(
		"running",
		"Up 3 minutes (healthy)",
	)
	if status != "running" {
		t.Fatalf("expected running status, got %q", status)
	}
	if health != "healthy" {
		t.Fatalf("expected healthy health status, got %q", health)
	}
	if !running {
		t.Fatalf("expected running=true")
	}
}

func TestDesktopStartGlobalServiceRunsComposeForTargetServiceForTest(t *testing.T) {
	desktop.ResetStateForTest()
	restoreEnsure := desktop.SetEnsureGlobalServicesForDesktopForTest(func() error {
		return nil
	})
	defer restoreEnsure()

	var capturedArgs []string
	restoreRun := desktop.SetRunGlobalServicesComposeForDesktopForTest(
		func(args ...string) (string, error) {
			capturedArgs = append([]string{}, args...)
			return "", nil
		},
	)
	defer restoreRun()

	app := desktop.NewApp()
	message, err := app.StartGlobalService("dnsmasq")
	if err != nil {
		t.Fatalf("StartGlobalService failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "started") {
		t.Fatalf("unexpected message: %q", message)
	}

	expected := []string{"up", "-d", "dnsmasq"}
	if strings.Join(capturedArgs, " ") != strings.Join(expected, " ") {
		t.Fatalf("unexpected compose args: got %v want %v", capturedArgs, expected)
	}
}

func TestDesktopStopGlobalServiceRunsComposeForTargetServiceForTest(t *testing.T) {
	desktop.ResetStateForTest()
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	composeDir := filepath.Join(home, "proxy")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatalf("mkdir compose dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(composeDir, "docker-compose.yml"),
		[]byte("services:\n  caddy: {}\n"),
		0o644,
	); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	var capturedArgs []string
	restoreRun := desktop.SetRunGlobalServicesComposeForDesktopForTest(
		func(args ...string) (string, error) {
			capturedArgs = append([]string{}, args...)
			return "", nil
		},
	)
	defer restoreRun()

	app := desktop.NewApp()
	message, err := app.StopGlobalService("mail")
	if err != nil {
		t.Fatalf("StopGlobalService failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "stopped") {
		t.Fatalf("unexpected message: %q", message)
	}

	expected := []string{"stop", "mail"}
	if strings.Join(capturedArgs, " ") != strings.Join(expected, " ") {
		t.Fatalf("unexpected compose args: got %v want %v", capturedArgs, expected)
	}
}

func TestDesktopRestartGlobalServicesUsesGovardSvcRestartForTest(t *testing.T) {
	desktop.ResetStateForTest()

	var gotArgs []string
	restoreRun := desktop.SetRunGovardCommandForDesktopForTest(
		func(_ string, args []string) (string, error) {
			gotArgs = append([]string{}, args...)
			return "services restarted via cli", nil
		},
	)
	defer restoreRun()

	app := desktop.NewApp()
	message, err := app.RestartGlobalServices()
	if err != nil {
		t.Fatalf("RestartGlobalServices failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "restarted") {
		t.Fatalf("unexpected message: %q", message)
	}

	expected := []string{"svc", "restart"}
	if !reflect.DeepEqual(gotArgs, expected) {
		t.Fatalf("unexpected govard args: got %v want %v", gotArgs, expected)
	}
}

func TestDesktopStartGlobalServicesUsesGovardSvcUpForTest(t *testing.T) {
	desktop.ResetStateForTest()

	var gotArgs []string
	restoreRun := desktop.SetRunGovardCommandForDesktopForTest(
		func(_ string, args []string) (string, error) {
			gotArgs = append([]string{}, args...)
			return "services started via cli", nil
		},
	)
	defer restoreRun()

	app := desktop.NewApp()
	message, err := app.StartGlobalServices()
	if err != nil {
		t.Fatalf("StartGlobalServices failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "started") {
		t.Fatalf("unexpected message: %q", message)
	}

	expected := []string{"svc", "up"}
	if !reflect.DeepEqual(gotArgs, expected) {
		t.Fatalf("unexpected govard args: got %v want %v", gotArgs, expected)
	}
}

func TestDesktopGlobalServiceLogsRejectUnknownServiceForTest(t *testing.T) {
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	_, err := app.GetGlobalServiceLogs("unknown", 100)
	if err == nil {
		t.Fatalf("expected unknown global service error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown global service") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDesktopDockerPortConflictWarningsForTest(t *testing.T) {
	desktop.ResetStateForTest()

	warnings := desktop.DetectDockerPortConflictWarningsForTest([]dockertypes.Summary{
		{
			Names: []string{"/warden-nginx-1"},
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project": "warden",
				"com.docker.compose.service": "nginx",
			},
			Ports: []dockertypes.Port{
				{PublicPort: 80, Type: "tcp"},
			},
		},
		{
			Names: []string{"/govard-proxy-caddy"},
			State: "running",
			Labels: map[string]string{
				"com.docker.compose.project": "proxy",
				"com.docker.compose.service": "caddy",
			},
			Ports: []dockertypes.Port{
				{PublicPort: 80, Type: "tcp"},
			},
		},
	})

	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "Port conflict 80/tcp: docker container warden-nginx-1") {
		t.Fatalf("expected docker port conflict warning, got %v", warnings)
	}
	if strings.Contains(joined, "govard-proxy-caddy") {
		t.Fatalf("expected govard proxy containers to be ignored, got %v", warnings)
	}
}

func TestDesktopHostPortConflictWarningsFromLsofForTest(t *testing.T) {
	desktop.ResetStateForTest()

	output := strings.Join([]string{
		"COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME",
		"nginx    2023 root   6u  IPv4  12345      0t0  TCP *:80 (LISTEN)",
	}, "\n")

	warnings := desktop.BuildHostPortConflictWarningsFromLsofForTest(output, 80, "tcp")
	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "Port conflict 80/tcp: host process nginx (pid: 2023, user: root)") {
		t.Fatalf("expected lsof warning to include process info, got %v", warnings)
	}
}

func TestDesktopHostPortConflictWarningsFromSSForTest(t *testing.T) {
	desktop.ResetStateForTest()

	output := strings.Join([]string{
		"Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process",
		`tcp   LISTEN 0      4096         0.0.0.0:443       0.0.0.0:* users:(("nginx",pid=2345,fd=6))`,
	}, "\n")

	warnings := desktop.BuildHostPortConflictWarningsFromSSForTest(output, 443, "tcp")
	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "Port conflict 443/tcp: host process nginx (pid: 2345)") {
		t.Fatalf("expected ss warning to include process info, got %v", warnings)
	}
}

func TestDesktopRoutingBindingWarningsForRunningServicesForTest(t *testing.T) {
	desktop.ResetStateForTest()

	services := []desktop.GlobalService{
		{
			ID:            "caddy",
			Name:          "Caddy Proxy",
			ContainerName: "govard-proxy-caddy",
			Running:       true,
			Status:        "running",
			State:         "running",
		},
		{
			ID:            "dnsmasq",
			Name:          "DNSMasq",
			ContainerName: "govard-proxy-dnsmasq",
			Running:       true,
			Status:        "running",
			State:         "running",
		},
	}

	containersByName := map[string]dockertypes.Summary{
		"govard-proxy-caddy": {
			Ports: []dockertypes.Port{
				{PublicPort: 2019, Type: "tcp"},
			},
		},
		"govard-proxy-dnsmasq": {
			Ports: []dockertypes.Port{
				{PublicPort: 53, Type: "tcp"},
			},
		},
	}

	warnings := desktop.DetectRoutingPublishedPortBindingWarningsForTest(services, containersByName)
	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "Port conflict 80/tcp: Caddy Proxy is running") {
		t.Fatalf("expected caddy bind warning, got %v", warnings)
	}
	if !strings.Contains(joined, "Port conflict 443/tcp: Caddy Proxy is running") {
		t.Fatalf("expected caddy https bind warning, got %v", warnings)
	}
	if !strings.Contains(joined, "Port conflict 53/udp: DNSMasq is running") {
		t.Fatalf("expected dnsmasq udp bind warning, got %v", warnings)
	}
}
