package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
)

// The debug shell must not flatten the caller's argv into a single string.
// `govard debug shell -c "php -r 'echo 1;'"` used to reach bash as three
// separate words, so bash ran `php` with no arguments at all.
func TestDebugShellInvocationPreservesArgumentBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantScript string
		wantTail   []string
	}{
		{
			name:       "passthrough keeps -c and its command as two arguments",
			args:       []string{"-c", "php -r 'echo 1;'"},
			wantScript: `exec bash "$@"`,
			wantTail:   []string{"-c", "php -r 'echo 1;'"},
		},
		{
			name:       "passthrough keeps a multi-word command in one argument",
			args:       []string{"-c", "php -r 'echo \"a b\";'"},
			wantScript: `exec bash "$@"`,
			wantTail:   []string{"-c", `php -r 'echo "a b";'`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, argv := cmd.DebugShellInvocationForTest("bebe9", tt.args)
			if len(argv) < 3 {
				t.Fatalf("expected -c, script and forwarded args, got %#v", argv)
			}
			if argv[0] != "-c" {
				t.Errorf("argv[0] = %q, want %q", argv[0], "-c")
			}
			if !strings.Contains(argv[1], tt.wantScript) {
				t.Errorf("script = %q, want it to contain %q", argv[1], tt.wantScript)
			}
			gotTail := argv[3:]
			if len(gotTail) != len(tt.wantTail) {
				t.Fatalf("forwarded args = %#v, want %#v", gotTail, tt.wantTail)
			}
			for i, want := range tt.wantTail {
				if gotTail[i] != want {
					t.Errorf("forwarded arg %d = %q, want %q", i, gotTail[i], want)
				}
			}
		})
	}
}

// A bare `govard debug` opens a session, so it runs bash with no forwarded
// arguments and asks for an interactive exec.
func TestDebugShellInvocationInteractiveHasNoForwardedArgs(t *testing.T) {
	_, argv := cmd.DebugShellInvocationForTest("bebe9", nil)
	if len(argv) != 2 || argv[0] != "-c" {
		t.Fatalf("expected an interactive [-c script] invocation, got %#v", argv)
	}
	if !strings.Contains(argv[1], "exec bash") {
		t.Errorf("script = %q, want it to exec bash", argv[1])
	}
	if strings.Contains(argv[1], `"$@"`) {
		t.Errorf("script = %q, want no \"$@\" forwarding for a bare session", argv[1])
	}
}

// Both forms must export the Xdebug session env the IDE keys on.
func TestDebugShellInvocationExportsXdebugEnv(t *testing.T) {
	for _, args := range [][]string{nil, {"-c", "php -v"}} {
		_, argv := cmd.DebugShellInvocationForTest("bebe9", args)
		script := argv[1]
		if !strings.Contains(script, "export XDEBUG_SESSION=PHPSTORM") {
			t.Errorf("args %#v: script %q does not export XDEBUG_SESSION", args, script)
		}
		if !strings.Contains(script, `export PHP_IDE_CONFIG="serverName=bebe9-docker"`) {
			t.Errorf("args %#v: script %q does not export PHP_IDE_CONFIG", args, script)
		}
	}
}
