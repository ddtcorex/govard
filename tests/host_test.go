package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/engine"
)

func TestHostForConfigResolvesSandbox(t *testing.T) {
	restore := deploy.StubResolveSyntheticSandboxRemoteForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{
			Host: "127.0.0.1", User: deploy.SandboxUser, Path: "/home/deployer/public_html", Sandbox: true,
			Deploy: &engine.DeployConfig{DeployPath: "/home/deployer/.deployer"},
		}, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	host, err := deploy.HostForConfigForTest(engine.Config{ProjectName: "sample-project"}, "sandbox", deploy.Options{})
	if err != nil {
		t.Fatalf("host for config: %v", err)
	}
	if host.Remote.Host != "127.0.0.1" || host.CurrentPath != "/home/deployer/public_html" {
		t.Fatalf("host = %+v, want the synthetic sandbox remote", host)
	}
}

func TestHostForConfigSurfacesTheAbsentError(t *testing.T) {
	restore := deploy.StubResolveSyntheticSandboxRemoteForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{}, deploy.SandboxLivenessAbsent, fmt.Errorf(
			"unknown remote: sandbox — no sandbox exists for this project; run 'govard sandbox up' to create it")
	})
	defer restore()

	_, err := deploy.HostForConfigForTest(engine.Config{ProjectName: "sample-project"}, "sandbox", deploy.Options{})
	if err == nil || !strings.Contains(err.Error(), "run 'govard sandbox up'") {
		t.Fatalf("err = %v, want the absent-sandbox message", err)
	}
}

// TestHostForConfigLayersConfiguredSandboxProtection is why the overlay exists
// on this path at all: `protected` is the one field that is about the operator's
// intent rather than the container, and a Host built from the un-overlaid
// synthetic remote could never honour it.
func TestHostForConfigLayersConfiguredSandboxProtection(t *testing.T) {
	restore := deploy.StubResolveSyntheticSandboxRemoteForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		base := overlayBase()
		return base, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	cfg := engine.Config{
		ProjectName: "sample-project",
		Remotes: engine.RemoteConfigMap{"sandbox": {
			Host:      "legacy.example.com",
			Protected: engine.BoolPtr(true),
		}},
	}

	host, err := deploy.HostForConfigForTest(cfg, "sandbox", deploy.Options{})
	if err != nil {
		t.Fatalf("host for config: %v", err)
	}
	if got := protectedFlag(host.Remote.Protected); got != "true" {
		t.Errorf("host.Remote.Protected = %s, want the configured protection (true)", got)
	}
	if host.Remote.Host != "127.0.0.1" {
		t.Errorf("host.Remote.Host = %q, want the container-derived 127.0.0.1", host.Remote.Host)
	}
	if host.CurrentPath != "/home/deployer/public_html" {
		t.Errorf("host.CurrentPath = %q, want the container's document root", host.CurrentPath)
	}
}

// protectedFlag renders a *bool the way a failure message can be read: a raw
// pointer address tells the reader nothing about the value it points at.
func protectedFlag(v *bool) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%t", *v)
}
