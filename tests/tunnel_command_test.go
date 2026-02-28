package tests

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/tunnel"
)

type fakeTunnelProvider struct {
	name       string
	buildErr   error
	captured   []tunnel.StartOptions
	returnPlan tunnel.StartPlan
}

func (provider *fakeTunnelProvider) Name() string {
	if strings.TrimSpace(provider.name) == "" {
		return "fake"
	}
	return provider.name
}

func (provider *fakeTunnelProvider) BuildStartPlan(options tunnel.StartOptions) (tunnel.StartPlan, error) {
	provider.captured = append(provider.captured, options)
	if provider.buildErr != nil {
		return tunnel.StartPlan{}, provider.buildErr
	}
	return provider.returnPlan, nil
}

func TestTunnelCommandExists(t *testing.T) {
	root := cmd.RootCommandForTest()

	tunnelCommand, _, err := root.Find([]string{"tunnel"})
	if err != nil {
		t.Fatalf("find tunnel command: %v", err)
	}
	if tunnelCommand == nil || tunnelCommand.Use != "tunnel" {
		t.Fatalf("unexpected tunnel command: %#v", tunnelCommand)
	}

	startCommand, _, err := root.Find([]string{"tunnel", "start"})
	if err != nil {
		t.Fatalf("find tunnel start command: %v", err)
	}
	if startCommand == nil || startCommand.Use != "start [url]" {
		t.Fatalf("unexpected tunnel start command: %#v", startCommand)
	}
}

func TestCloudflareTunnelProviderBuildStartPlan(t *testing.T) {
	provider, err := tunnel.NewProvider(engine.ProviderRef{Kind: engine.ProviderKindTunnel, Name: "cloudflare"})
	if err != nil {
		t.Fatalf("new cloudflare provider: %v", err)
	}

	plan, err := provider.BuildStartPlan(tunnel.StartOptions{
		TargetURL:   "https://demo.test",
		NoTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("build start plan: %v", err)
	}
	if plan.Binary != "cloudflared" {
		t.Fatalf("expected cloudflared binary, got %s", plan.Binary)
	}
	joined := strings.Join(plan.Args, " ")
	if !strings.Contains(joined, "tunnel --url https://demo.test") {
		t.Fatalf("expected url args in plan, got: %s", joined)
	}
	if !strings.Contains(joined, "--no-tls-verify") {
		t.Fatalf("expected --no-tls-verify flag in plan, got: %s", joined)
	}
}

func TestTunnelStartPlanUsesConfigDomainByDefault(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: laravel
`), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeProvider := &fakeTunnelProvider{
		name: "fake-tunnel",
		returnPlan: tunnel.StartPlan{
			Binary: "fake-tunnel",
			Args:   []string{"start", "--target", "https://demo.test"},
		},
	}
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider: func(ref engine.ProviderRef) (tunnel.Provider, error) {
			if ref.Kind != engine.ProviderKindTunnel {
				t.Fatalf("expected tunnel provider kind, got %s", ref.Kind)
			}
			if ref.Name != "cloudflare" {
				t.Fatalf("expected default provider name cloudflare, got %s", ref.Name)
			}
			return fakeProvider, nil
		},
		RunCommand: func(_ *exec.Cmd) error {
			t.Fatal("did not expect command execution in --plan mode")
			return nil
		},
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	buf := &strings.Builder{}
	root := cmd.RootCommandForTest()
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"tunnel", "start", "--plan"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute tunnel start --plan: %v", err)
	}
	if len(fakeProvider.captured) != 1 {
		t.Fatalf("expected one BuildStartPlan call, got %d", len(fakeProvider.captured))
	}
	options := fakeProvider.captured[0]
	if options.TargetURL != "https://demo.test" {
		t.Fatalf("expected default target URL from config domain, got %s", options.TargetURL)
	}
	if !options.NoTLSVerify {
		t.Fatal("expected no-tls-verify default true")
	}

	out := buf.String()
	if !strings.Contains(out, "Tunnel Plan") {
		t.Fatalf("expected tunnel plan header, got: %s", out)
	}
	if !strings.Contains(out, "fake-tunnel start --target https://demo.test") {
		t.Fatalf("expected planned command output, got: %s", out)
	}
}

func TestTunnelStartRejectsConflictingURLInputs(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: laravel
`), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider: func(ref engine.ProviderRef) (tunnel.Provider, error) {
			_ = ref
			return &fakeTunnelProvider{
				name: "fake",
				returnPlan: tunnel.StartPlan{
					Binary: "fake",
				},
			}, nil
		},
		RunCommand: func(_ *exec.Cmd) error {
			return errors.New("unexpected command run")
		},
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"tunnel", "start", "https://arg.test", "--url", "https://flag.test"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflicting URL input error")
	}
	if !strings.Contains(err.Error(), "either positional [url] or --url") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
}
