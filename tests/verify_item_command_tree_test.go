package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/verify"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// verifyNoArgvItemsForTest is the exact set of rows that build no command line
// under this fence's fixture, each with what it does instead.
//
// Two of the six can never appear here, whatever the fixture: P2-13 and P4-11
// probe the project's own site in-process (`probeSite`, `probeSearchHealth`) and
// never shell out to a govard child, so they have no argv to resolve at all. The
// other four skip on a fixture that does not seed what they need, which the
// fixture below now does seed — a Hyvä theme marker and a module under
// app/code — so only P5-05's phase-5 gate is left, and no hermetic capture can
// satisfy it.
//
// The set is pinned in both directions: a row that stops building argv must be
// added here, with its reason, rather than quietly shrinking the fence's reach.
var verifyNoArgvItemsForTest = map[string]string{
	"P2-13": "never builds one: it probes the site in-process, so it has no govard child to resolve",
	"P4-11": "never builds one: it probes the search route in-process, so it has no govard child to resolve",
	"P5-05": "fails: its phase-5 gate recorded no snapshot in a phase-4 run",
}

// The reach floors are the numbers this fence measured on the registry it was
// written against: 63 distinct argvs over 126 invocations, both exit codes
// included (P5-02 no longer runs `env up` after a failed `env down -v`, which
// took one invocation off the old 127). They are floors rather than equalities, so a new branch or a new
// item only raises them; a fall means an item lost a call or a subset stopped
// building argv — a row can keep running something while dropping a later
// command, which the no-argv pin above cannot see. Each is the exact measured
// value because there is no slack to give: the smallest real loss is one call.
const (
	verifyDistinctArgvsFloorForTest = 63
	verifyInvocationsFloorForTest   = 126
)

// Every verify item is a command line, and verify never parses that line
// itself: it builds an argv and hands it to a `govard` child process
// (internal/verify/exec.go). So a registry string the command tree does not
// accept still produces a verdict, and both shapes have shipped:
//
//   - a word the tree does not know under a command that is a group with no
//     RunE — `tool redis-cli ping` (issue #475). cobra answers with the help
//     page and exit 0, and the item recorded a green from a page nobody ran;
//   - a flag the command does not declare — `env up --build` (#491). cobra
//     rejects it before RunE, so the item is a permanent exit-2 usage error;
//   - a command renamed or removed under the item (the `Find` arm below).
//
// Only a test that resolves the captured argv against the real tree — the same
// `Find` and the same flag set a run uses — can fence that class. The per-item
// pins in verify_item_argv_test.go cover the two items the class was found on;
// this one covers every item, every argv.
//
// An item that produced no argv at all is not a gap: it skipped — a remote item
// with no usable remote, a missing module, a domain the fixture does not
// configure — or failed before it built one, and each of those states is
// asserted by that item's own tests. The exact set is pinned in
// verifyNoArgvItemsForTest above, so a row that quietly stops running fails here
// instead of shrinking a log line. This fence's business is the argv an item did
// build.
//
// Reach boundary, stated because the name overclaims without it: this resolves
// command *paths* and *flags*. Positional arguments are cobra's business at
// execute time (`Command.Args` validators run in `execute()`, after
// `Find`), so a command line that is missing or mistyping a positional — #461's
// nameless `snapshot restore` — is invisible here and needs its own test.
func TestEveryItemArgvResolvesToARunnableCommand(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	cfg := engine.Config{Framework: "magento2", Domain: "sample.test"}
	// The fixture seeds what three items need to build an argv at all: a Hyvä
	// theme's Tailwind manifest (P2-09) and a module under app/code (P3-13 and
	// P3-14). Those three carry the argv shapes this branch changed most —
	// `--prefix <theme>`, `--mode module_in_project --path <dir>`,
	// `--mode standalone --path <dir>` — so a fixture that left them out would
	// make the blindness an expectation instead of a gap. The fence only resolves
	// argvs; it never runs a child, so no npm install and no container is
	// involved.
	projectRoot, _ := magentoProjectWithAppCodeModule(t, "DemoFence")
	writeFixtureFile(t, filepath.Join(projectRoot, "app", "design", "frontend", "Acme", "Blank", "web", "tailwind", "package.json"),
		`{"name":"acme/blank-tailwind","version":"1.0.0"}`)
	// P1-06 only runs lock commands when a lock file exists.
	writeFixtureFile(t, engine.LockFilePath(projectRoot), "{}")
	argvs := captureAllItemArgvs(t, cfg, verify.VerifyOpts{
		Remote:      captureRemoteForTest,
		ProjectRoot: projectRoot,
	})

	root := cmd.RootCommandForTest()

	items, reached, invocations := 0, 0, 0
	distinct := make(map[string]struct{})
	var noArgv []string
	for _, it := range verify.RegistryFor(cfg) {
		if it.Run == nil {
			continue
		}
		items++
		itemArgvs, captured := argvs[it.ID]
		// "The capture has no entry" and "the item ran no command" are
		// different facts, and reading them as one would let a capture that
		// silently dropped an item pass this fence.
		if !captured {
			t.Errorf("%s is runnable but the capture returned no entry for it: the fence cannot tell an item that ran nothing from one it never ran", it.ID)
			continue
		}
		if len(itemArgvs) == 0 {
			noArgv = append(noArgv, it.ID)
			continue
		}
		reached++
		for _, argv := range itemArgvs {
			invocations++
			distinct[strings.Join(argv, "\x00")] = struct{}{}
			assertItemArgvResolvesForTest(t, root, it.ID, argv)
		}
	}

	if items == 0 {
		t.Fatal("the registry has no runnable item: this fence would police nothing")
	}
	// A run in which nothing resolved the tree would satisfy every assertion
	// below by never making one, which is the vacuity this line exists to stop.
	if reached == 0 {
		t.Fatal("no item produced an argv at all: every resolution below would pass vacuously")
	}
	assertVerifyReachForTest(t, noArgv, reached, items, invocations, len(distinct))
}

// assertVerifyReachForTest pins how much of the registry the capture reached.
// The counts alone would not: they are proportional to nothing, so losing a
// subset of items — every `--remote` row skipping, say — would leave the fence
// green and the log line smaller.
func assertVerifyReachForTest(t *testing.T, noArgv []string, reached, items, invocations, distinctArgvs int) {
	t.Helper()

	// Every declared check must be a name `audit run --checks` accepts, or that
	// item is unreachable under every legal selection while every test stays
	// green. The production validator is the source of truth, so this compares
	// against it instead of restating its three names here.
	for _, it := range verify.RegistryFor(engine.Config{Framework: "magento2"}) {
		for _, name := range it.Checks {
			if _, err := audit.NormalizeChecks([]string{name}); err != nil {
				t.Errorf("%s declares Item.Checks %q, which `audit run --checks` rejects (%v): a row whose declared check no selection can name is unreachable", it.ID, name, err)
			}
		}
	}

	for id, why := range verifyNoArgvItemsForTest {
		if !containsString(noArgv, id) {
			t.Errorf("%s is expected to build no argv (%s) and built one: if it can run now, drop it from verifyNoArgvItemsForTest rather than leaving the pin wrong", id, why)
		}
	}
	for _, id := range noArgv {
		if _, known := verifyNoArgvItemsForTest[id]; !known {
			t.Errorf("%s built no argv and is not in verifyNoArgvItemsForTest: a row that stops building a command is the regression this fence exists to catch, so record why it cannot run here", id)
		}
	}

	if invocations < verifyInvocationsFloorForTest {
		t.Errorf("captured %d invocations over both exit codes, want at least %d: an item lost a call, so part of the tree it used to exercise is now unchecked",
			invocations, verifyInvocationsFloorForTest)
	}
	if distinctArgvs < verifyDistinctArgvsFloorForTest {
		t.Errorf("resolved %d distinct argvs, want at least %d: command lines the fence used to check are gone",
			distinctArgvs, verifyDistinctArgvsFloorForTest)
	}

	t.Logf("reach: %d of %d runnable items produced an argv; %d invocations over both exit codes, %d distinct argvs; no argv for %d item(s): %s",
		reached, items, invocations, distinctArgvs, len(noArgv), strings.Join(noArgv, " "))
}

// assertItemArgvResolvesForTest is the fence itself, one captured argv at a
// time: the argv must resolve to a command that can actually run, and that
// command must accept the flags the item passes it.
func assertItemArgvResolvesForTest(t *testing.T, root *cobra.Command, id string, argv []string) {
	t.Helper()
	joined := strings.Join(argv, " ")

	command, rest, err := root.Find(argv)
	if err != nil {
		t.Errorf("%s invoked `govard %s`, which the command tree does not resolve: %v", id, joined, err)
		return
	}
	// A group with no RunE is not a failure cobra reports: the not-runnable
	// branch prints help and returns nil, so the item's verdict comes from a
	// help page. That is #475, and it is the reason this half is checked at all.
	if !command.Runnable() {
		t.Errorf("%s invoked `govard %s`, which resolves to `%s` — a command with no RunE, so cobra prints help and exits 0 having run nothing",
			id, joined, command.CommandPath())
		return
	}
	// A pass-through child owns every word after its own name: the framework
	// commands are `DisableFlagParsing` (internal/cmd/frameworks.go:161) because
	// the rest of the line belongs to the wrapped tool —
	// `tool php vendor/bin/phpstan --help` is not a flag this tree must accept.
	// The command itself still had to resolve and be runnable, which is the half
	// of the check a pass-through child can carry.
	if command.DisableFlagParsing {
		return
	}

	// cobra registers `--help`/`-h` while it executes (InitDefaultHelpFlag,
	// command.go:1219), which this test never reaches: without the same call,
	// `open --help` would fail on a flag that exists at run time. It also merges
	// the parents' persistent flags, so the flag set parsed below is the one a
	// real run parses.
	command.InitDefaultHelpFlag()
	restore := snapshotCommandFlagsForTest(t, command)
	defer restore()

	if err := command.ParseFlags(rest); err != nil {
		t.Errorf("%s invoked `govard %s`, which resolves to `%s` but rejects it: %v", id, joined, command.CommandPath(), err)
	}
}

// snapshotCommandFlagsForTest returns a func that puts every flag of the
// command's flag set back where it was, and it is not optional: the tree is one
// process-wide object (cmd.RootCommandForTest, internal/cmd/remote.go:732), so a
// `--plan` parsed here would turn whichever test runs next into a dry run, and a
// slice flag would grow an entry on every Set after the first — pflag clears one
// only through SliceValue.Replace.
//
// Values are captured and restored rather than reset to DefValue, because
// `audit run --checks` declares a non-empty default ([]string{"lint"},
// internal/cmd/audit.go:205) that Replace([]string{}) would silently delete.
func snapshotCommandFlagsForTest(t *testing.T, command *cobra.Command) func() {
	t.Helper()
	type savedFlag struct {
		flag    *pflag.Flag
		isSlice bool
		value   string
		slice   []string
	}
	var saved []savedFlag
	command.Flags().VisitAll(func(flag *pflag.Flag) {
		s := savedFlag{flag: flag, value: flag.Value.String()}
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			s.isSlice = true
			s.slice = append([]string(nil), slice.GetSlice()...)
		}
		saved = append(saved, s)
	})

	return func() {
		t.Helper()
		for _, s := range saved {
			var err error
			if s.isSlice {
				err = s.flag.Value.(pflag.SliceValue).Replace(s.slice)
			} else {
				err = s.flag.Value.Set(s.value)
			}
			if err != nil {
				t.Errorf("restore --%s to %q: %v", s.flag.Name, s.value, err)
			}
			s.flag.Changed = false
		}
	}
}
