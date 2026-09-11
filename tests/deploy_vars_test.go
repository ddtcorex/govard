package tests

import (
	"errors"
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
