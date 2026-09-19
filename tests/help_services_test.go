package tests

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func renderServiceHelp(t *testing.T, args []string) string {
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

func TestRedisCliHelpShowsWrapperHelp(t *testing.T) {
	help := renderServiceHelp(t, []string{"redis", "cli", "--help"})
	if !strings.Contains(help, "Usage:") {
		t.Fatalf("redis cli --help shows no Usage:\n%s", help)
	}
	if strings.Contains(help, "is unknown") {
		t.Fatalf("redis cli --help executed instead of showing help:\n%s", help)
	}
}

func TestInitLongListsCurrentMagentoVersions(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "init")
	if !strings.Contains(long, "2.4.8") {
		t.Fatalf("init Long stops before 2.4.8:\n%s", long)
	}
}

func TestInitLongMentionsWardenAndDrupal(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "init")
	for _, want := range []string{"warden", "Drupal"} {
		if !strings.Contains(long, want) {
			t.Fatalf("init Long omits %q:\n%s", want, long)
		}
	}
}

func TestVarnishBanWithoutPatternIsUsageError(t *testing.T) {
	root := cmd.RootCommandForTest()
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"varnish", "ban"})
	err := root.Execute()
	if err == nil {
		t.Fatal("varnish ban without a pattern must fail, got nil error")
	}
	if !strings.Contains(err.Error(), "ban <pattern>") {
		t.Fatalf("varnish ban error does not document the pattern: %v", err)
	}
}

func TestLogsShortcutDocumentsErrorsFlag(t *testing.T) {
	_, long, _, example := lookupHelpCommand(t, "logs")
	if !strings.Contains(long, "--errors") || !strings.Contains(example, "--errors") {
		t.Fatalf("logs shortcut hides --errors:\n%s\n%s", long, example)
	}
}

func TestDownShortcutExampleShowsVolumes(t *testing.T) {
	_, _, _, example := lookupHelpCommand(t, "down")
	if !strings.Contains(example, "-v") {
		t.Fatalf("down shortcut has no volumes example:\n%s", example)
	}
}

func TestSearchLongStatesGetOnly(t *testing.T) {
	for _, svc := range []string{"elasticsearch", "opensearch"} {
		_, long, _, _ := lookupHelpCommand(t, svc)
		if !strings.Contains(long, "GET") {
			t.Fatalf("%s Long understates the GET-only query semantics:\n%s", svc, long)
		}
	}
}

func TestValkeyLongDocumentsPrereq(t *testing.T) {
	_, long, _, example := lookupHelpCommand(t, "valkey")
	if !strings.Contains(long, "stack.services.cache=valkey") {
		t.Fatalf("valkey Long omits the enablement precondition:\n%s", long)
	}
	if !strings.Contains(example, "valkey cli") {
		t.Fatalf("valkey has no usage example:\n%s", example)
	}
}

func TestVarnishLongDisambiguatesLogAndLogs(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "varnish")
	if !strings.Contains(long, "container logs") {
		t.Fatalf("varnish Long does not disambiguate log vs logs:\n%s", long)
	}
}

func TestUpLongDocumentsQuickstartScope(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "env", "up")
	if !strings.Contains(long, "Xdebug") {
		t.Fatalf("up Long omits the quickstart scope:\n%s", long)
	}
}

func TestVersionLongMentionsChannel(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "version")
	if !strings.Contains(long, "channel") {
		t.Fatalf("version help omits the update-channel line:\n%s", long)
	}
}

func TestShellLongDocumentsShFallback(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "shell")
	if !strings.Contains(long, "falls back to sh") {
		t.Fatalf("shell Long omits the sh fallback:\n%s", long)
	}
}

func TestServiceExamplesUseRootForm(t *testing.T) {
	for _, svc := range []string{"redis", "rabbitmq", "varnish"} {
		_, _, _, example := lookupHelpCommand(t, svc)
		if strings.Contains(example, "govard env "+svc) {
			t.Fatalf("%s examples use the env-prefixed form:\n%s", svc, example)
		}
		if !strings.Contains(example, "govard "+svc) {
			t.Fatalf("%s examples lack the root form:\n%s", svc, example)
		}
	}
}

func TestServiceLongSpelling(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "opensearch")
	if !strings.Contains(long, "OpenSearch") {
		t.Fatalf("opensearch Long misspells the product name:\n%s", long)
	}
	for _, tc := range []struct {
		path []string
	}{
		{[]string{"redis"}},
		{[]string{"varnish"}},
		{[]string{"debug"}},
	} {
		_, l, _, _ := lookupHelpCommand(t, tc.path...)
		if strings.Contains(l, " \n") {
			t.Fatalf("%v Long has trailing whitespace:\n%q", tc.path, l)
		}
	}
}
