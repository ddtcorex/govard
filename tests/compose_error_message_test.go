package tests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

// TestRunComposeErrorCarriesWhatComposeSaid pins that a failed compose run keeps
// its diagnosis. `docker compose ps` on a missing compose file exits 1 and explains
// itself on stderr, but RunCompose returned only `exit status 1`: the human saw the
// explanation, while a script reading --error-json got a message that named no
// cause at all (measured: `govard ps --error-json` -> message "exit status 1").
func TestRunComposeErrorCarriesWhatComposeSaid(t *testing.T) {
	bin := t.TempDir()
	fake := filepath.Join(bin, "docker")
	script := "#!/bin/sh\necho 'compose file \"/tmp/nope.yml\" is invalid: no such file' >&2\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write the fake docker: %v", err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")

	stderr := &bytes.Buffer{}
	err := engine.RunCompose(context.Background(), engine.ComposeOptions{
		ProjectDir:  t.TempDir(),
		ProjectName: "demo",
		Args:        []string{"ps"},
		Stdout:      &bytes.Buffer{},
		Stderr:      stderr,
	})
	if err == nil {
		t.Fatal("a failing compose run must return an error")
	}

	message := err.Error()
	if strings.Contains(message, "no such file") == false {
		t.Fatalf("the error hides what compose said: %q (stderr was %q)", message, stderr.String())
	}
	if !strings.Contains(message, "exit status 1") {
		t.Fatalf("the exit status should stay in the message, got %q", message)
	}
	// The explanation still reaches the caller's stderr stream untouched.
	if !strings.Contains(stderr.String(), "is invalid") {
		t.Fatalf("the compose stderr stream was altered: %q", stderr.String())
	}
}
