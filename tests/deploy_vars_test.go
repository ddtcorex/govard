package tests

import (
	"errors"
	"govard/internal/cmd"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestExpandQuotesEverySubstitutedValue(t *testing.T) {
	vars := deploy.NewVars().
		Set("branch", `main; rm -rf /`).
		Set("revision", `abc'def`).
		SetPath("deploy_path", "~/.deployer").
		SetPath("current_path", "/home/deploy/public_html")

	got, err := vars.Expand("mkdir -p {{deploy_path}}/releases && cd {{current_path}} && git fetch origin {{branch}} # {{revision}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if !strings.Contains(got, `origin 'main; rm -rf /'`) {
		t.Fatalf("branch must be single-quoted immediately after the command: %s", got)
	}
	if strings.Contains(got, `origin main; rm`) {
		t.Fatalf("branch was substituted unquoted: %s", got)
	}
	if !strings.Contains(got, `'abc'"'"'def'`) {
		t.Fatalf("single quote not escaped: %s", got)
	}
	if !strings.Contains(got, `$HOME/'.deployer'`) {
		t.Fatalf("deploy_path must expand $HOME on the remote, got: %s", got)
	}
	if !strings.Contains(got, `'/home/deploy/public_html'`) {
		t.Fatalf("absolute path not quoted: %s", got)
	}
}

func TestExpandLeavesUnknownVariableAloneInFreeText(t *testing.T) {
	vars := deploy.NewVars().Set("branch", "main")

	got, err := vars.Expand("echo {{branch}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if got != `echo 'main'` {
		t.Fatalf("expand = %q, want echo 'main'", got)
	}

	if _, err := vars.Expand("echo {{nope}}"); !errors.Is(err, deploy.ErrUnknownVariable) {
		t.Fatalf("err = %v, want ErrUnknownVariable", err)
	}
}

// Spec 10.4: the content version is the deployed revision, "short SHA", so a
// static URL stays readable and a retry of the same revision writes the same one.
func TestDeployVarsUseAShortContentVersion(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	const revision = "0123456789abcdef0123456789abcdef01234567"

	options := deploy.Options{Revision: revision, Settings: map[string]any{"content_version": ""}}
	expanded, err := cmd.DeployVarsForTest(host, options).Expand("{{settings.content_version}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	// Values are shell-quoted on substitution, so the expanded form carries
	// quotes; what matters here is the length, not the quoting.
	if got := strings.Trim(expanded, "'"); got != revision[:8] {
		t.Fatalf("content_version = %q, want the short revision %q", got, revision[:8])
	}

	// An explicit value still wins: the setting exists to re-version content
	// without a new revision.
	options.Settings["content_version"] = "campaign-2026"
	expanded, err = cmd.DeployVarsForTest(host, options).Expand("{{settings.content_version}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if got := strings.Trim(expanded, "'"); got != "campaign-2026" {
		t.Fatalf("content_version = %q, want the configured value", expanded)
	}

	// With no revision at all the variable stays empty rather than becoming a
	// placeholder that would leak into asset URLs.
	options = deploy.Options{Settings: map[string]any{"content_version": ""}}
	expanded, err = cmd.DeployVarsForTest(host, options).Expand("{{settings.content_version}}")
	if err != nil {
		t.Fatalf("expand without a revision: %v", err)
	}
	if got := strings.Trim(expanded, "'"); got != "" {
		t.Fatalf("content_version = %q, want empty", got)
	}
}
