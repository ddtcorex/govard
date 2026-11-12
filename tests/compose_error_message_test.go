package tests

import (
	"bytes"
	"context"
	"errors"
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

// TestRunComposeErrorKeepsTheCauseOfALongLine pins which end of an over-long line
// survives. Compose opens most diagnoses with the offending path and ends with the
// reason ("... is invalid: open /long/path: no such file or directory"), so cutting
// the head of the line kept the path and dropped the cause (measured:
// `govard ps --error-json` outside a project ended in "open /tmp/...").
func TestRunComposeErrorKeepsTheCauseOfALongLine(t *testing.T) {
	bin := t.TempDir()
	fake := filepath.Join(bin, "docker")
	longPath := "/" + strings.Repeat("deep/", 80)
	script := "#!/bin/sh\necho 'compose file \"" + longPath + "x.yml\" is invalid: open " + longPath + "x.yml: no such file or directory' >&2\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write the fake docker: %v", err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")

	err := engine.RunCompose(context.Background(), engine.ComposeOptions{
		ProjectDir:  t.TempDir(),
		ProjectName: "demo",
		Args:        []string{"ps"},
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("a failing compose run must return an error")
	}
	message := err.Error()
	if !strings.HasSuffix(message, "no such file or directory") {
		t.Fatalf("the cause was cut off a long line: %q", message)
	}
	if len(message) > 300 {
		t.Fatalf("the detail must stay bounded, got %d bytes", len(message))
	}
}

// TestComposeErrorFlagsDetailThatAlreadyReachedTheTerminal pins the metadata the
// human error line needs: compose's stderr went to the process stderr (so the
// reason is already on screen) only when the caller did not redirect it.
func TestComposeErrorFlagsDetailThatAlreadyReachedTheTerminal(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\necho 'boom: reason' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the fake docker: %v", err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")

	run := func(stderr *bytes.Buffer) error {
		opts := engine.ComposeOptions{ProjectDir: t.TempDir(), ProjectName: "demo", Args: []string{"ps"}, Stdout: &bytes.Buffer{}}
		if stderr != nil {
			opts.Stderr = stderr
		}
		return engine.RunCompose(context.Background(), opts)
	}

	var composeErr *engine.ComposeError
	if err := run(&bytes.Buffer{}); !errors.As(err, &composeErr) || composeErr.Streamed {
		t.Fatalf("a redirected stderr must not count as shown: %#v", err)
	}
	if composeErr.Detail != "boom: reason" {
		t.Fatalf("detail = %q", composeErr.Detail)
	}
	// The default stream is the process stderr, which a person is reading.
	if err := run(nil); !errors.As(err, &composeErr) || !composeErr.Streamed {
		t.Fatalf("the default stderr is shown to the user: %#v", err)
	}
}
