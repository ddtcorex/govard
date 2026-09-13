package tests

import (
	"strings"
	"testing"

	"govard/internal/deploy"
)

// The tool name arrives from project configuration and is rendered into a
// Dockerfile, so it is checked against a closed table rather than trusted.
func TestSandboxToolsAreAClosedList(t *testing.T) {
	if err := deploy.ValidateSandboxTools([]string{"wp-cli"}); err != nil {
		t.Fatalf("wp-cli must be a known tool: %v", err)
	}
	if err := deploy.ValidateSandboxTools([]string{"wp-cli", "curl-everything"}); err == nil {
		t.Fatal("an unknown tool name must be refused")
	}
}

// A spec that asks for no tools must render no tool block. This is the property
// that keeps every existing project's image unchanged.
func TestSandboxWithoutToolsRendersNoToolBlock(t *testing.T) {
	for _, profile := range []string{deploy.SandboxProfilePHP, deploy.SandboxProfileFull} {
		dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: profile, PHP: "8.4"})
		if err != nil {
			t.Fatalf("render the %s profile: %v", profile, err)
		}
		if strings.Contains(dockerfile, "/usr/local/bin/wp") {
			t.Errorf("the %s profile installs wp-cli without being asked:\n%s", profile, dockerfile)
		}
	}
}

func TestSandboxInstallsARequestedTool(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileFull,
		PHP:          "8.4",
		Requirements: deploy.SandboxRequirements{Tools: []string{"wp-cli"}},
	})
	if err != nil {
		t.Fatalf("render with tools: %v", err)
	}
	for _, want := range []string{"/usr/local/bin/wp", "chmod 0755 /usr/local/bin/wp", "wp --version"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the rendered image does not %q:\n%s", want, dockerfile)
		}
	}
}

// The basic profile has no PHP and no toolchain, and it promises neither.
func TestSandboxBasicProfileInstallsNoTools(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileBasic,
		Requirements: deploy.SandboxRequirements{Tools: []string{"wp-cli"}},
	})
	if err != nil {
		t.Fatalf("render the basic profile: %v", err)
	}
	if strings.Contains(dockerfile, "/usr/local/bin/wp") {
		t.Errorf("the basic profile must not install tools:\n%s", dockerfile)
	}
}

// An image that cannot be built is a far worse diagnosis than a refusal that
// names the tools the engine knows.
func TestSandboxRefusesAnUnknownTool(t *testing.T) {
	if _, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileFull,
		PHP:          "8.4",
		Requirements: deploy.SandboxRequirements{Tools: []string{"not-a-tool"}},
	}); err == nil {
		t.Fatal("an unknown tool must fail the render, not the image build")
	}
}
