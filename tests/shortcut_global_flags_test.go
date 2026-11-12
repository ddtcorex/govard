package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// stubEnvDependencies replaces the environment dependencies the shortcuts reach,
// so a test can observe the compose arguments without touching Docker, the proxy
// or the hosts file.
func stubEnvDependencies(t *testing.T, captured *[]string) {
	t.Helper()
	restore := cmd.SetEnvDependenciesForTest(cmd.EnvDependenciesForTest{
		RunCompose: func(_ context.Context, opts engine.ComposeOptions) error {
			*captured = append([]string{}, opts.Args...)
			return nil
		},
		RegisterDomains:           func([]string, string) error { return nil },
		UnregisterDomain:          func(string) error { return nil },
		RegisterSearchDomains:     func([]string, string) error { return nil },
		UnregisterSearchDomain:    func(string) error { return nil },
		UnregisterRabbitMQDomain:  func(string) error { return nil },
		AddHostsEntry:             func(string) error { return nil },
		RemoveHostsEntry:          func(string) error { return nil },
		IsDomainResolvableLocally: func(string) bool { return false },
		RunHooks:                  func(engine.Config, string, io.Writer, io.Writer) error { return nil },
		RefreshPMAActiveProjects:  func() error { return nil },
	})
	t.Cleanup(restore)
}

// writeShortcutProject gives the shortcut something to resolve: the commands load
// the project config from the working directory before touching compose.
func writeShortcutProject(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	chdirForTest(t, dir)
	writeRuntimeConfig(t, dir, "project_name: shortcut-demo\ndomain: shortcut-demo.test\nframework: wordpress\n")
}

// TestDownShortcutHonoursErrorJSONInsteadOfForwardingIt pins the flag contract the
// audit found broken: the root environment shortcuts disable flag parsing so Docker
// Compose flags pass through untouched, and that also swallowed the global flags.
// `govard down --error-json` answered "unknown flag: --error-json" from compose and
// emitted no envelope, and the banner still landed on stdout.
func TestDownShortcutHonoursErrorJSONInsteadOfForwardingIt(t *testing.T) {
	writeShortcutProject(t)
	var composedArgs []string
	stubEnvDependencies(t, &composedArgs)

	stdout := &bytes.Buffer{}
	processStdout, restoreStdout := captureProcessStdout(t)
	t.Cleanup(restoreStdout)
	pterm.SetDefaultOutput(stdout)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })

	stderr := &bytes.Buffer{}
	root := cmd.RootCommandForTest()
	t.Cleanup(func() { _ = root.PersistentFlags().Set("error-json", "false") })
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"down", "--error-json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("govard down --error-json: %v", err)
	}

	for _, forwarded := range composedArgs {
		if forwarded == "--error-json" {
			t.Fatalf("the global flag reached compose: %v", composedArgs)
		}
	}
	if got, want := composedArgs[0], "down"; got != want {
		t.Fatalf("first compose argument = %q, want %q", got, want)
	}
	// Byte-exact, not "no banner": the blank lines the banner is wrapped in were
	// still written to stdout, so a consumer saw two stray newlines before the
	// document it was told to parse. fmt.Println reaches os.Stdout directly, so the
	// capture has to cover both writers.
	written := append(processStdout(), stdout.Bytes()...)
	if len(written) != 0 {
		t.Fatalf("machine mode left %d bytes on stdout: %q", len(written), string(written))
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Tearing Down Govard Environment")) {
		t.Fatalf("the banner should be readable on stderr, got:\n%q", stderr.String())
	}
}

// TestShortcutConsumesVerboseWithoutLosingComposeFlags pins the other half: the
// passthrough exists for a reason, so Docker Compose flags and service names must
// still arrive verbatim, and a `--` must stop the consumption.
func TestShortcutConsumesVerboseWithoutLosingComposeFlags(t *testing.T) {
	writeShortcutProject(t)
	var composedArgs []string
	stubEnvDependencies(t, &composedArgs)

	root := cmd.RootCommandForTest()
	t.Cleanup(func() { _ = root.PersistentFlags().Set("verbose", "false") })
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"down", "--verbose", "-v", "php", "--remove-orphans"})

	if err := root.Execute(); err != nil {
		t.Fatalf("govard down --verbose -v php: %v", err)
	}

	// down appends --remove-orphans only when the caller did not pass one, so the
	// compose flags and the service name arrive exactly as written.
	want := []string{"down", "-v", "php", "--remove-orphans"}
	if len(composedArgs) != len(want) {
		t.Fatalf("compose args = %v, want %v", composedArgs, want)
	}
	for i := range want {
		if composedArgs[i] != want[i] {
			t.Fatalf("compose args = %v, want %v", composedArgs, want)
		}
	}
}

// TestSplitPassthroughGlobalsKeepsTheBoundaryExplicit covers the splitter itself,
// including the forms a shell will produce and the `--` terminator compose uses.
func TestSplitPassthroughGlobalsKeepsTheBoundaryExplicit(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		forwarded []string
		errorJSON bool
		verbose   bool
	}{
		{"plain", []string{"logs", "-f"}, []string{"logs", "-f"}, false, false},
		{"error-json", []string{"ps", "--error-json"}, []string{"ps"}, true, false},
		{"error-json=false", []string{"ps", "--error-json=false"}, []string{"ps"}, false, false},
		{"verbose", []string{"down", "--verbose"}, []string{"down"}, false, true},
		{"both", []string{"--error-json", "down", "--verbose"}, []string{"down"}, true, true},
		{"after terminator", []string{"logs", "--", "--verbose"}, []string{"logs", "--", "--verbose"}, false, false},
		{"unknown stays", []string{"logs", "--tail=5"}, []string{"logs", "--tail=5"}, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cmd.SplitPassthroughGlobalsForTest(tc.args)
			if len(got.Forwarded) != len(tc.forwarded) {
				t.Fatalf("forwarded = %v, want %v", got.Forwarded, tc.forwarded)
			}
			for i := range tc.forwarded {
				if got.Forwarded[i] != tc.forwarded[i] {
					t.Fatalf("forwarded = %v, want %v", got.Forwarded, tc.forwarded)
				}
			}
			if got.ErrorJSON != tc.errorJSON || got.Verbose != tc.verbose {
				t.Fatalf("globals = errorJSON:%v verbose:%v, want errorJSON:%v verbose:%v",
					got.ErrorJSON, got.Verbose, tc.errorJSON, tc.verbose)
			}
		})
	}
}

var _ = cobra.Command{}
