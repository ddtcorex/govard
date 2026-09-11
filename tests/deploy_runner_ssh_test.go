package tests

import (
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/engine"
)

func TestSSHRunnerArgsAreNonInteractiveAndPassTheCommand(t *testing.T) {
	runner := deploy.SSHRunner{
		RemoteName: "staging",
		Config: engine.RemoteConfig{
			Host: "staging.example.com",
			User: "deploy",
			Port: 2222,
			Auth: engine.RemoteAuth{KeyPath: "/tmp/key"},
		},
	}
	args := runner.Args("echo hello")
	joined := strings.Join(args, " ")
	for _, want := range []string{"-o BatchMode=yes", "-o ConnectTimeout=10", "-p 2222", "-i /tmp/key", "deploy@staging.example.com", "echo hello"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, " -t ") {
		t.Error("a deploy command must not allocate a TTY")
	}
	if args[len(args)-1] != "echo hello" {
		t.Fatalf("last arg = %q, want the remote command", args[len(args)-1])
	}
}
