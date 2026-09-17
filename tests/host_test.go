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
