package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestGenCompletionForTest(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		script, err := cmd.GenCompletionForTest(shell)
		if err != nil {
			t.Fatalf("GenCompletionForTest(%q) returned error: %v", shell, err)
		}
		if len(strings.TrimSpace(script)) == 0 {
			t.Fatalf("GenCompletionForTest(%q) returned an empty script", shell)
		}
		if !strings.Contains(script, "govard") {
			t.Fatalf("GenCompletionForTest(%q) does not mention govard", shell)
		}
	}

	if _, err := cmd.GenCompletionForTest("tcsh"); err == nil {
		t.Fatal("GenCompletionForTest(tcsh) should return an error for unsupported shells")
	}
}
