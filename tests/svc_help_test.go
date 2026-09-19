package tests

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func renderSvcHelp(t *testing.T, args []string) string {
	t.Helper()
	root := cmd.RootCommandForTest()
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return output.String()
}

// Govard-only toggles are consumed by handleSvcUp and must never reach
// 'docker compose up', which rejects them as unknown flags.
func TestStripSvcUpGovardToggles(t *testing.T) {
	got := cmd.StripSvcUpArgsForTest([]string{"up", "--no-trust", "--pull", "--no-fallback", "-d", "web"})
	want := []string{"up", "-d", "web"}
	if len(got) != len(want) {
		t.Fatalf("stripped args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stripped args = %v, want %v", got, want)
		}
	}
}

// 'svc up' always prepends -d, so attach-mode compose flags can never apply
// and must not be advertised.
func TestSvcUpHelpHidesDetachedIncompatibleFlags(t *testing.T) {
	help := renderSvcHelp(t, []string{"svc", "up", "--help"})
	if !strings.Contains(help, "always runs detached") {
		t.Fatalf("svc up help does not state detached mode:\n%s", help)
	}
	for _, rejected := range []string{"abort-on-container-exit", "abort-on-container-failure", "exit-code-from", "--attach", "--menu"} {
		if strings.Contains(help, rejected) {
			t.Fatalf("svc up help advertises detached-incompatible %s:\n%s", rejected, help)
		}
	}
}

func TestSvcSleepWakeHelpRendersWithoutCompose(t *testing.T) {
	for _, sub := range []string{"sleep", "wake"} {
		help := renderSvcHelp(t, []string{"svc", sub, "--help"})
		if strings.Contains(help, "Failed to get Docker Compose help") {
			t.Fatalf("svc %s help shells out to compose:\n%s", sub, help)
		}
		if !strings.Contains(help, "Usage:") {
			t.Fatalf("svc %s help has no Usage section:\n%s", sub, help)
		}
	}
}

func TestSvcTopHelpListsNativeCommands(t *testing.T) {
	help := renderSvcHelp(t, []string{"svc", "--help"})
	for _, native := range []string{"sleep", "wake"} {
		if !strings.Contains(help, native) {
			t.Fatalf("svc help does not list native subcommand %q:\n%s", native, help)
		}
	}
}

func TestSvcRestartHelpDocumentsDownPlusUp(t *testing.T) {
	help := renderSvcHelp(t, []string{"svc", "restart", "--help"})
	if !strings.Contains(help, "down") || !strings.Contains(help, "'up'") {
		t.Fatalf("svc restart help does not document down-followed-by-up:\n%s", help)
	}
}

func TestSvcLongNamesDockerRequirement(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"svc"})
	if err != nil {
		t.Fatalf("find svc: %v", err)
	}
	if !strings.Contains(command.Long, "Docker") {
		t.Fatalf("svc Long does not name its Docker requirement:\n%s", command.Long)
	}
}

func TestPolishRebrandedHelp(t *testing.T) {
	top := cmd.PolishRebrandedHelpForTest("Define and run multi-container applications with Docker\n\nUsage:\n  govard svc [OPTIONS] COMMAND", "svc", "")
	if strings.Contains(top, "Define and run multi-container applications") {
		t.Fatalf("compose tagline not stripped from top-level page:\n%s", top)
	}
	exec := cmd.PolishRebrandedHelpForTest("By default 'govard svc exec' allocates a TTY. (default true)", "svc", "exec")
	if strings.Contains(exec, "(default true)") {
		t.Fatalf("exec TTY wording not clarified:\n%s", exec)
	}
	run := cmd.PolishRebrandedHelpForTest("-T, --no-tty  Disable pseudo-TTY allocation (default: auto-detected) (default true)", "svc", "run")
	if strings.Contains(run, "(default true)") {
		t.Fatalf("run doubled default not fixed:\n%s", run)
	}
}

func TestSvcVersionHelpNotesComposePlugin(t *testing.T) {
	help := renderSvcHelp(t, []string{"svc", "version", "--help"})
	if !strings.Contains(help, "Compose plugin") {
		t.Fatalf("svc version help does not disambiguate the Compose plugin version:\n%s", help)
	}
}
