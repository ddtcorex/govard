package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// A deploy runs one ssh command per step, and every one of them used to pay a TCP
// handshake, a key exchange and an authentication — around twenty for a Magento
// pipeline, half of them inside the maintenance window. The options below are
// what makes the first connection the only one.
func TestSSHArgsShareOneConnectionPerTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := engine.RemoteConfig{Host: "example.com", User: "deploy", Port: 2222}
	joined := strings.Join(remote.BuildSSHArgs("staging", cfg, false, false), " ")

	for _, want := range []string{
		"ControlMaster=auto",
		"ControlPath=" + filepath.Join(home, ".govard", "ssh", "%C"),
		"ControlPersist=60s",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("ssh args %q missing %q", joined, want)
		}
	}

	// The socket is per target and must not be readable by anyone else: the
	// directory is created by the first command that needs it.
	info, err := os.Stat(filepath.Join(home, ".govard", "ssh"))
	if err != nil {
		t.Fatalf("the socket directory must be created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("socket directory mode = %o, want 700", perm)
	}

	// The deploy runner is the caller that pays the cost, so it has to carry them.
	runner := deploy.SSHRunner{RemoteName: "staging", Config: cfg}
	if got := strings.Join(runner.Args("true"), " "); !strings.Contains(got, "ControlMaster=auto") {
		t.Fatalf("the deploy runner does not share connections: %q", got)
	}
}

// Connection reuse is a performance property, not a requirement: a host whose
// home cannot hold the socket must still be deployable.
func TestSSHArgsDropConnectionReuseWhenTheSocketCannotBeWritten(t *testing.T) {
	notADirectory := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADirectory, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed the blocking file: %v", err)
	}
	t.Setenv("HOME", notADirectory)

	joined := strings.Join(remote.BuildSSHArgs("staging", engine.RemoteConfig{Host: "example.com", User: "deploy"}, false, false), " ")
	if strings.Contains(joined, "ControlMaster") || strings.Contains(joined, "ControlPath") {
		t.Fatalf("connection reuse cannot work here, so it must not be requested: %q", joined)
	}
	// The rest of the non-interactive contract is untouched.
	for _, want := range []string{"BatchMode=yes", "ConnectTimeout=10"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ssh args %q missing %q", joined, want)
		}
	}
}

// Executed rather than asserted on the builder: the options have to reach the ssh
// process, which is where they mean anything.
func TestSSHRunnerHandsConnectionReuseToSSH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "ssh.log")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write the ssh stub: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	runner := deploy.SSHRunner{
		RemoteName: "staging",
		Config:     engine.RemoteConfig{Host: "staging.example.com", User: "deploy", Port: 2222},
	}
	if _, err := runner.Run(context.Background(), "echo hello", deploy.RunOptions{}); err != nil {
		t.Fatalf("run against the stub: %v", err)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the stub ssh did not run: %v", err)
	}
	recorded := string(raw)
	for _, want := range []string{"ControlMaster=auto", "ControlPersist=60s", "deploy@staging.example.com", "echo hello"} {
		if !strings.Contains(recorded, want) {
			t.Fatalf("the ssh stub received %q, missing %q", recorded, want)
		}
	}
}
