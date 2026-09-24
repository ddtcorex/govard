package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// A configured command is rendered as shell-quoted words so a wrapper of several
// words works (`php -d memory_limit=-1`) and shell syntax in the value is inert.
// The one piece of shell syntax a real configuration relies on is a leading
// tilde: `composer_bin: "php ~/composer.phar"` is common on shared hosting, and
// quoting every word turns that tilde into a literal directory name, which fails
// with "Could not open input file" at the first command the deploy runs.
func TestCommandWordsExpandsALeadingTilde(t *testing.T) {
	home := t.TempDir()
	stub := filepath.Join(home, "bin", "php")
	if err := os.MkdirAll(filepath.Dir(stub), 0o755); err != nil {
		t.Fatalf("mkdir the stub's directory: %v", err)
	}
	writeFile(t, stub, "#!/bin/sh\necho stub-ran\n")
	if err := os.Chmod(stub, 0o755); err != nil {
		t.Fatalf("make the stub executable: %v", err)
	}

	rendered := deploy.CommandWords(map[string]any{"php_bin": "~/bin/php"}, "php_bin", "php")
	cmd := exec.Command("sh", "-c", rendered)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the rendered command must run the binary the tilde names: %v (%s)\nrendered: %s", err, out, rendered)
	}
	if !strings.Contains(string(out), "stub-ran") {
		t.Fatalf("the stub did not run: %q\nrendered: %s", out, rendered)
	}
}

// A wrapper of several words keeps working, which is the other half of the same
// contract: quoting must not weld `-d memory_limit=-1` into the command name.
func TestCommandWordsKeepsAWrapperOfSeveralWords(t *testing.T) {
	home := t.TempDir()
	stub := filepath.Join(home, "php")
	writeFile(t, stub, "#!/bin/sh\nprintf '%s\\n' \"$@\"\n")
	if err := os.Chmod(stub, 0o755); err != nil {
		t.Fatalf("make the stub executable: %v", err)
	}

	rendered := deploy.CommandWords(map[string]any{"php_bin": stub + " -d memory_limit=-1"}, "php_bin", "php")
	out, err := exec.Command("sh", "-c", rendered+" -r 'echo PHP_VERSION;'").CombinedOutput()
	if err != nil {
		t.Fatalf("run the wrapper: %v (%s)\nrendered: %s", err, out, rendered)
	}
	want := "-d\nmemory_limit=-1\n-r\necho PHP_VERSION;\n"
	if string(out) != want {
		t.Fatalf("the wrapper must receive its own words untouched:\n got %q\nwant %q\nrendered: %s", out, want, rendered)
	}
}

// Nothing else is expanded: a `$VAR` or a command substitution inside the value
// stays inert, because the value came from the project and a deploy target is not
// the place to evaluate it. A leading tilde is the single exception, and it is
// expanded by the shell rather than by govard, so the value never becomes code.
func TestCommandWordsKeepsShellSyntaxInert(t *testing.T) {
	for _, value := range []string{"php; echo pwned", "php $(echo pwned)", "php `echo pwned`", "php && echo pwned"} {
		rendered := deploy.CommandWords(map[string]any{"php_bin": value}, "php_bin", "php")
		out, _ := exec.Command("sh", "-c", rendered).CombinedOutput()
		if strings.Contains(string(out), "pwned") {
			t.Fatalf("value %q must stay inert, but %s ran %q", value, rendered, out)
		}
		if strings.Contains(rendered, "echo pwned") && !strings.Contains(rendered, "'") {
			t.Fatalf("value %q reached the command unquoted: %s", value, rendered)
		}
	}
}
