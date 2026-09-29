package tests

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"govard/internal/cmd"
)

// snapshotCommandTreeFlags returns a func that puts every flag of the whole
// command tree back the way it was, and calling it is not optional.
//
// cmd.RootCommandForTest hands out ONE process-wide object, and cobra parses
// flags onto the live command, not onto a copy. Two leaks in this package were
// pre-existing and only became visible when #469 added tests that actually run
// `tunnel stop`:
//
//   - renderDeployHelp executes `tunnel stop --help`, and cobra registers
//     --help at execute time (command.go:1219) and leaves it set on the
//     command, so every later Execute of that command printed help and skipped
//     RunE entirely.
//   - the tunnel tests execute `tunnel start --plan` and `tunnel start <url>
//     --url <flag>`, leaving --plan and --url set, so a later `tunnel start`
//     silently became a dry run and a later one reported a bogus "not both".
//
// The tree is walked rather than one command, because the leak can sit on a
// parent (persistent flags) as easily as on the command under test. Values are
// captured and restored rather than reset to DefValue, because `audit run
// --checks` declares a non-empty default ([]string{"lint"}) that
// Replace([]string{}) would silently delete.
func snapshotCommandTreeFlags(t *testing.T, root *cobra.Command) func() {
	t.Helper()
	type savedFlag struct {
		flag    *pflag.Flag
		isSlice bool
		value   string
		slice   []string
	}
	var saved []savedFlag
	seen := map[*pflag.Flag]bool{}
	visit := func(set *pflag.FlagSet) {
		if set == nil {
			return
		}
		set.VisitAll(func(flag *pflag.Flag) {
			if seen[flag] {
				return
			}
			seen[flag] = true
			entry := savedFlag{flag: flag, value: flag.Value.String()}
			if slice, ok := flag.Value.(pflag.SliceValue); ok {
				entry.isSlice = true
				entry.slice = append([]string(nil), slice.GetSlice()...)
			}
			saved = append(saved, entry)
		})
	}
	var walk func(command *cobra.Command)
	walk = func(command *cobra.Command) {
		visit(command.Flags())
		visit(command.PersistentFlags())
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)

	return func() {
		t.Helper()
		for _, entry := range saved {
			var err error
			if entry.isSlice {
				err = entry.flag.Value.(pflag.SliceValue).Replace(entry.slice)
			} else {
				err = entry.flag.Value.Set(entry.value)
			}
			if err != nil {
				t.Errorf("restore --%s to %q: %v", entry.flag.Name, entry.value, err)
			}
			entry.flag.Changed = false
		}
		// Flags cobra registers lazily at execute time -- --help above all,
		// command.go:1219 -- do not exist yet when the snapshot is taken, so
		// they cannot be captured. Reset anything that appeared meanwhile to its
		// default; leaving --help set makes every later Execute of that command
		// print help and skip RunE.
		var resetLate func(command *cobra.Command)
		resetLate = func(command *cobra.Command) {
			reset := func(set *pflag.FlagSet) {
				if set == nil {
					return
				}
				set.VisitAll(func(flag *pflag.Flag) {
					if seen[flag] {
						return
					}
					seen[flag] = true
					if slice, ok := flag.Value.(pflag.SliceValue); ok {
						_ = slice.Replace(nil)
					} else {
						_ = flag.Value.Set(flag.DefValue)
					}
					flag.Changed = false
				})
			}
			reset(command.Flags())
			reset(command.PersistentFlags())
			for _, child := range command.Commands() {
				resetLate(child)
			}
		}
		resetLate(root)
	}
}

func lookupHelpCommand(t *testing.T, path ...string) (flagUsage func(name string) string, long string, use string, example string) {
	t.Helper()
	root := cmd.RootCommandForTest()
	command, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("find %v: %v", path, err)
	}
	return func(name string) string {
		flag := command.Flags().Lookup(name)
		if flag == nil {
			flag = command.PersistentFlags().Lookup(name)
		}
		if flag == nil {
			t.Fatalf("%v has no --%s flag", path, name)
		}
		return flag.Usage
	}, command.Long, command.Use, command.Example
}

func TestDoctorCommitImpliesFix(t *testing.T) {
	for _, tc := range []struct {
		fix, commit, want bool
	}{
		{false, false, false},
		{true, false, true},
		{false, true, true},
		{true, true, true},
	} {
		if got := cmd.DoctorFixEnabledForTest(tc.fix, tc.commit); got != tc.want {
			t.Fatalf("DoctorFixEnabled(fix=%v, commit=%v) = %v, want %v", tc.fix, tc.commit, got, tc.want)
		}
	}
}

func TestAuditChecksUsageDocumentsIntegrity(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "audit")
	if !strings.Contains(usage("checks"), "integrity") {
		t.Fatalf("--checks usage omits integrity: %q", usage("checks"))
	}
}

func TestAuditBaseUsageDocumentsAuto(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "audit")
	if !strings.Contains(usage("base"), "auto") {
		t.Fatalf("--base usage omits auto detection: %q", usage("base"))
	}
}

func TestAuditSessionCommandsDocumentRequiredFlags(t *testing.T) {
	for _, tc := range []struct {
		path []string
		want []string
	}{
		{[]string{"audit", "rerun"}, []string{"--session"}},
		{[]string{"audit", "status"}, []string{"--session"}},
		{[]string{"audit", "result"}, []string{"--session", "--run"}},
		{[]string{"audit", "cleanup"}, []string{"--older-than"}},
	} {
		_, long, use, example := lookupHelpCommand(t, tc.path...)
		documented := long + " " + use + " " + example
		for _, want := range tc.want {
			if !strings.Contains(documented, want) {
				t.Fatalf("%v help does not document required %s (Long/Use/Example):\n%s", tc.path, want, documented)
			}
		}
	}
}

func TestAuditToolchainLongScope(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "audit", "toolchain")
	if strings.Contains(long, "Magento") {
		t.Fatalf("toolchain Long over-specifies Magento:\n%s", long)
	}
}

func TestDbLongRemoteNotDefault(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "db")
	if strings.Contains(long, "Remote Environment (default)") {
		t.Fatalf("db Long mislabels remote dumps as default:\n%s", long)
	}
	if !strings.Contains(long, "Remote Environment:") {
		t.Fatalf("db Long lost the remote paragraph:\n%s", long)
	}
}

func TestDbLongDocumentsTopAndCloneVolume(t *testing.T) {
	_, long, _, example := lookupHelpCommand(t, "db")
	for _, action := range []string{"top", "clone-volume"} {
		if !strings.Contains(long, action) || !strings.Contains(example, action) {
			t.Fatalf("db help does not document %q (Long + Example):\n%s\n%s", action, long, example)
		}
	}
}

func TestDbNoiseFlagsUsageCoversStreamImport(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "db")
	for _, name := range []string{"no-noise", "no-pii"} {
		if !strings.Contains(usage(name), "stream-db") {
			t.Fatalf("--%s usage omits --stream-db import:\n%s", name, usage(name))
		}
	}
}

func TestSyncLongDocumentsCatalogMode(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "sync")
	if !strings.Contains(long, "catalog") {
		t.Fatalf("sync Long omits the catalog media mode:\n%s", long)
	}
}

func TestSyncLongDefinesProtectedRemotes(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "sync")
	if !strings.Contains(long, "protected: true") {
		t.Fatalf("sync Long does not say which remotes are protected:\n%s", long)
	}
}

func TestSnapshotShortCoversRemote(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"snapshot"})
	if err != nil {
		t.Fatalf("find snapshot: %v", err)
	}
	if strings.Contains(command.Short, "local snapshots") {
		t.Fatalf("snapshot Short claims local-only: %q", command.Short)
	}
}

func TestSnapshotCreateLocalUsageNotesUnimplemented(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "snapshot", "create")
	if !strings.Contains(usage("local"), "not yet implemented") {
		t.Fatalf("--local usage hides the unimplemented state: %q", usage("local"))
	}
}

func TestSnapshotPullPushDocumentRemoteEnv(t *testing.T) {
	for _, sub := range []string{"pull", "push"} {
		_, long, use, example := lookupHelpCommand(t, "snapshot", sub)
		documented := long + " " + use + " " + example
		if !strings.Contains(documented, "-e ") && !strings.Contains(documented, "--environment") {
			t.Fatalf("snapshot %s help does not document the required remote environment:\n%s", sub, documented)
		}
	}
}

func TestBootstrapExampleExcludesNoiseWithFlag(t *testing.T) {
	_, _, _, example := lookupHelpCommand(t, "bootstrap")
	if !strings.Contains(example, "--no-noise --no-pii") {
		t.Fatalf("bootstrap noise+PII example lacks --no-noise:\n%s", example)
	}
}

func TestBootstrapNoPIIUsesPShorthand(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"bootstrap"})
	if err != nil {
		t.Fatalf("find bootstrap: %v", err)
	}
	if got := command.Flags().Lookup("no-pii").Shorthand; got != "P" {
		t.Fatalf("--no-pii shorthand = %q, want P (aligned with db/sync)", got)
	}
}

func TestBootstrapHyvaTokenDefaultEmpty(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"bootstrap"})
	if err != nil {
		t.Fatalf("find bootstrap: %v", err)
	}
	if got := command.Flags().Lookup("hyva-token").DefValue; got != "" {
		t.Fatalf("--hyva-token default is %q, want empty (resolved from the built-in fallback in code)", got)
	}
}

func TestBootstrapFreshCloneDocumented(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "bootstrap")
	if !strings.Contains(long, "--clone") || !strings.Contains(long, "cannot be used with --clone") {
		t.Fatalf("bootstrap Long does not document the --fresh/--clone rule:\n%s", long)
	}
}

func TestDebugLongDocumentsXdebugPrereq(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "debug")
	if !strings.Contains(long, "govard debug on") {
		t.Fatalf("debug Long omits the Xdebug prerequisite:\n%s", long)
	}
}

func TestTestUsageListsStaticAlias(t *testing.T) {
	_, _, use, _ := lookupHelpCommand(t, "test")
	if !strings.Contains(use, "static") {
		t.Fatalf("test Use omits the static alias: %q", use)
	}
}

func TestToolLongDocumentsHelpPassthrough(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "tool")
	if !strings.Contains(long, "--help") {
		t.Fatalf("tool Long does not document <sub> --help passthrough:\n%s", long)
	}
}

func TestVerifyFlagUsageStyle(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "verify")
	for _, name := range []string{"allow-destructive", "allow-xdebug", "json", "lint-jobs"} {
		text := usage(name)
		if text == "" || text[0] < 'A' || text[0] > 'Z' {
			t.Fatalf("--%s usage is not sentence style: %q", name, text)
		}
	}
}
