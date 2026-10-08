package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
)

func TestOpenAdminURLForSandboxUsesPublishedWebPort(t *testing.T) {
	remote := deploy.SandboxRemoteConfig(deploy.SandboxProfilePHP, "8.3", 2222, 49153, deploy.SandboxDefaultPaths(), deploy.SandboxKeyPair{})
	if got, want := cmd.BuildRemoteAdminURLForTest(remote, "admin"), "http://127.0.0.1:49153/admin"; got != want {
		t.Fatalf("admin URL = %q, want %q", got, want)
	}
}

func TestOpenAdminURLForSandboxWithoutWebPortKeepsHostFallback(t *testing.T) {
	remote := deploy.SandboxRemoteConfig(deploy.SandboxProfilePHP, "8.3", 2222, 0, deploy.SandboxDefaultPaths(), deploy.SandboxKeyPair{})
	if got := cmd.BuildRemoteAdminURLForTest(remote, "admin"); got != "https://127.0.0.1/admin" {
		t.Fatalf("unexpected fallback %q", got)
	}
}
