package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

func TestVerifyJSONValidIgnoresStderr(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fakegovard")
	body := "#!/bin/sh\necho 'warning: something noisy' >&2\necho '{\"ok\":true}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(verify.EnvBinaryOverride, script)
	ev := verify.ExecGovardForTest(context.Background(), engine.Config{}, verify.VerifyOpts{}, "x", "--json")
	if !ev.JSONValid {
		t.Fatalf("JSONValid = false although stdout is one JSON document; excerpt=%q", ev.OutputExcerpt)
	}
	if !strings.Contains(ev.OutputExcerpt, "warning: something noisy") {
		t.Fatalf("excerpt lost the stderr text: %q", ev.OutputExcerpt)
	}
}

func TestVerifyJSONValidFalseWhenStdoutNotJSON(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fakegovard")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho not json\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(verify.EnvBinaryOverride, script)
	ev := verify.ExecGovardForTest(context.Background(), engine.Config{}, verify.VerifyOpts{}, "x")
	if ev.JSONValid {
		t.Fatal("JSONValid = true for non-JSON stdout")
	}
}
