package tests

import (
	"context"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// captureRemoteForTest is the remote every capture names. A capture with an
// empty Remote would be vacuous: a remote-touching item takes its target from
// --remote alone, so withRemote returns Skip before it builds any argv — and the
// remote items are exactly the ones this file exists to police. The empty-Remote
// behaviour is asserted separately in verify_remote_items_test.go.
const captureRemoteForTest = "sandbox"

// captureItemArgvs runs every item in RegistryFor(cfg) with a fake exec that
// records argv and returns ExitCode 1 (which ends an item at its first govard
// invocation unless it ignores that exit code), against a temp ProjectRoot. The
// fake answers every govard invocation, and the items' only other work is a
// local read under that root, so nothing leaves the process.
//
// It MUST pass Remote: "sandbox" (or any name). Since Task 3, a remote-touching
// item with an empty Remote returns Skip and builds no argv — so an unfixed
// capture would be vacuous for exactly the 13 items this fence exists to police.
// The empty-Remote case is asserted separately, in Task 3's tests.
//
// The map is keyed by item id and holds that item's argv, which ExitCode 1 keeps
// to the item's first invocation. Five items ignore that exit code and make one
// more call — P1-06 (`lock diff`), P2-01/P5-02/P5-06 (`env up`) and P2-13 (the
// http retry) — all of them local, none of them remote-guarded. A remote-guarded
// item that ran twice would leave the fence blind to half of its argv, so the
// capture fails the test rather than return a partial one.
func captureItemArgvs(t *testing.T, cfg engine.Config, opts verify.VerifyOpts) map[string][]string {
	t.Helper()
	if opts.Remote == "" {
		opts.Remote = captureRemoteForTest
	}

	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	argvs := make(map[string][]string)
	for _, it := range verify.RegistryFor(cfg) {
		if it.Run == nil {
			continue
		}
		var first []string
		invocations := 0
		verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
			invocations++
			if first == nil {
				first = append([]string(nil), args...)
			}
			return verify.Evidence{ExitCode: 1, OutputExcerpt: "fake: " + strings.Join(args, " ")}, true
		})
		_ = it.Run(context.Background(), cfg, opts)
		if invocations > 1 && (it.Guard == verify.GuardReadOnlyRemote || it.Guard == verify.GuardRemoteWrite) {
			t.Errorf("%s carries %s and made %d govard invocations; the capture keeps only the first, so it can no longer classify all of this item's argv",
				it.ID, it.Guard, invocations)
		}
		argvs[it.ID] = first
	}
	verify.SetExecGovardFakeForTest(nil)

	return argvs
}

// isRemoteWriteArgv classifies one captured argv against the design's
// remote-write table (§4.5): `bootstrap` or `sync` without `--plan`,
// `db import`, `snapshot restore`/`create`/`delete`/`push`, and `open -e`.
//
// It classifies instead of matching the bare verb, because the verb alone does
// not say what an item does:
//
//   - `bootstrap` and `sync` reach a remote by default (each auto-selects one
//     when none is named), so their read-only form is marked by `--plan`;
//   - `db import` and `snapshot restore`/`create`/`delete` are local unless
//     -e/--environment names a remote — see the two helpers below;
//   - `snapshot push` and `open -e <remote>` write through the remote whenever
//     they run at all.
func isRemoteWriteArgv(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	switch argv[0] {
	case "bootstrap", "sync":
		return !hasArgvFlag(argv, "--plan")
	case "db":
		return dbArgvWritesThroughRemote(argv)
	case "snapshot":
		return snapshotArgvWritesThroughRemote(argv)
	case "open":
		// `open` resolves "" and `local` to the local environment and everything
		// else to a configured remote (internal/cmd/open_targets.go:190-193); a
		// remote target is the class the design calls out for `open -e`, since
		// `open shell -e <remote>` probes SSH auth and can copy a key into the
		// remote's authorized_keys before it opens anything
		// (internal/cmd/open_targets.go:65).
		return namesARemoteEnvironment(argv)
	default:
		return false
	}
}

// dbArgvWritesThroughRemote narrows the table's `db import` entry: `db`'s
// environment defaults to `local` (internal/cmd/db.go:105) and the import stays
// local there, while a non-local one is the writing form — it builds an SSH
// import into the remote database, resolved for write
// (internal/cmd/db.go:684-693), and any non-local db run may first copy an SSH
// key into the remote's authorized_keys (internal/cmd/db.go:253-258, the
// mechanism the design cites for `db`/`open -e`). `--stream-db` only reads the
// remote's dump, but that key copy is decided by the environment alone, so the
// stream form stays in the same class.
func dbArgvWritesThroughRemote(argv []string) bool {
	return argvSubcommand(argv) == "import" && namesARemoteEnvironment(argv)
}

// snapshotArgvWritesThroughRemote narrows the table's `snapshot restore`/`push`
// entry — and covers `create`/`delete`, which share the branch structure:
//
//   - `push` fails without a remote ("push requires a remote --environment",
//     internal/cmd/snapshot.go:526), so it is a remote write unconditionally.
//   - `restore`, `create` and `delete` take the remote path only when
//     -e/--environment names a remote that is not `local`: restore restores the
//     local environment otherwise (internal/cmd/snapshot.go:259-263), and
//     create/delete build their SSH command in that same branch
//     (internal/cmd/snapshot.go:47-50, :356-362). P5-05 runs the bare local
//     restore and carries DESTRUCTIVE-LOCAL, so calling the verb alone a remote
//     write would make this fence demand REMOTE-WRITE on a local item the first
//     time a fixture seeded a phase-4 snapshot — a fence that can cry wolf is
//     worse than no fence.
//   - `pull`, `list` and `export` name a remote at most to read from it.
//
// Do not "simplify" the restore arm back to `argv[1] == "restore" ||
// argv[1] == "push"`.
func snapshotArgvWritesThroughRemote(argv []string) bool {
	switch argvSubcommand(argv) {
	case "push":
		return true
	case "create", "delete", "restore":
		return namesARemoteEnvironment(argv)
	default:
		return false
	}
}

// namesARemoteEnvironment reports whether an argv names a remote through the
// -e/--environment flag the classified verbs use for it. A value that is
// non-empty and not `local` is a remote: `snapshot` and `open` resolve "" and
// `local` to the local environment (internal/cmd/snapshot.go:259,
// internal/cmd/open_targets.go:192), and `db` defaults the flag to `local`
// (internal/cmd/db.go:105).
func namesARemoteEnvironment(argv []string) bool {
	switch strings.ToLower(strings.TrimSpace(argvFlagValue(argv, "-e", "--environment"))) {
	case "", "local":
		return false
	default:
		return true
	}
}

// argvSubcommand returns the word after the verb — `snapshot restore`,
// `db import`, `snapshot -e prod push` — reading past the flags that may precede
// it: cobra accepts a persistent flag before the subcommand, so the subcommand is
// not argv[1] in general. `-e/--environment` is the only such flag that takes a
// separate value token here (it is persistent on `snapshot`,
// internal/cmd/snapshot.go:603, and local to `db`, internal/cmd/db.go:105, where
// the form is read but does not parse); a `--flag=value` token is one word.
func argvSubcommand(argv []string) string {
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if strings.HasPrefix(arg, "-") {
			if !strings.Contains(arg, "=") && (arg == "-e" || arg == "--environment") {
				i++ // the next word is this flag's value, not the subcommand
			}
			continue
		}
		return arg
	}
	return ""
}

// argvFlagValue returns the value of the first flag in names that appears in
// argv, in either the `-e value` or the `-e=value` form cobra accepts. An absent
// flag, or one with no value, reports "".
func argvFlagValue(argv []string, names ...string) string {
	for i, arg := range argv {
		for _, name := range names {
			if arg == name {
				if i+1 < len(argv) {
					return argv[i+1]
				}
				return ""
			}
			if value, ok := strings.CutPrefix(arg, name+"="); ok {
				return value
			}
		}
	}
	return ""
}

// hasArgvFlag reports whether an argv carries the flag as its own token, which
// is how every item in this registry builds one (`"--plan"`, `"-e"`).
func hasArgvFlag(argv []string, flag string) bool {
	for _, arg := range argv {
		if arg == flag {
			return true
		}
	}
	return false
}

// The literal `staging` was a guess about what a project calls its remote, and
// the guess is gone: the remote comes from --remote alone. A capture that built
// no argv would satisfy "no argv says staging" without testing anything, so
// every READ-ONLY-REMOTE item must also have produced one — that is the exact
// vacuity an empty Remote introduces.
func TestNoReadOnlyRemoteItemResolvesTheStagingLiteral(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	cfg := engine.Config{Framework: "magento2"}
	argvs := captureItemArgvs(t, cfg, verify.VerifyOpts{
		Remote:      captureRemoteForTest,
		ProjectRoot: t.TempDir(),
	})

	items := verify.RegistryFor(cfg)

	readOnly := 0
	for _, it := range items {
		if it.Guard != verify.GuardReadOnlyRemote {
			continue
		}
		readOnly++
		// P4-16 (`remote list`) names no remote on purpose, so the assertion is
		// "this item produced an argv", not "that argv carries the remote name".
		if len(argvs[it.ID]) == 0 {
			t.Errorf("%s carries %s but produced no argv: the staging fence is vacuous for it", it.ID, verify.GuardReadOnlyRemote)
		}
	}
	if readOnly == 0 {
		t.Fatalf("no item carries %s: this fence has nothing to police", verify.GuardReadOnlyRemote)
	}

	for _, it := range items {
		joined := strings.Join(argvs[it.ID], " ")
		if strings.Contains(joined, "staging") {
			t.Errorf("%s invoked %q, which carries the removed staging default", it.ID, joined)
		}
	}
}

// The guard label is a rule only if it matches what the item's argv does: an
// item that writes through a remote without GuardRemoteWrite would run unasked
// (that was P2-05/P2-08 before Task 2 relabelled them).
func TestEveryRemoteWritingItemDeclaresRemoteWrite(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	cfg := engine.Config{Framework: "magento2"}
	argvs := captureItemArgvs(t, cfg, verify.VerifyOpts{
		Remote:      captureRemoteForTest,
		ProjectRoot: t.TempDir(),
	})

	remoteWrites := 0
	for _, it := range verify.RegistryFor(cfg) {
		argv := argvs[it.ID]
		if !isRemoteWriteArgv(argv) {
			continue
		}
		remoteWrites++
		if it.Guard == verify.GuardReadOnlyRemote {
			t.Errorf("%s carries %s but runs `%s`, which writes through a remote",
				it.ID, verify.GuardReadOnlyRemote, strings.Join(argv, " "))
			continue
		}
		if it.Guard != verify.GuardRemoteWrite {
			t.Errorf("%s runs `%s`, which writes through a remote, but carries Guard %q, want %q",
				it.ID, strings.Join(argv, " "), it.Guard, verify.GuardRemoteWrite)
		}
	}
	if remoteWrites == 0 {
		t.Fatal("no item's argv matched the remote-write table: the classifier and the registry have drifted apart, so this fence proves nothing")
	}
}

// The classifier is a pure function, so every arm of the table is pinned here
// directly instead of only through the registry: no item runs `db import`,
// `snapshot create/delete` or `snapshot restore -e <remote>` today, so without
// this the fence could rot on those verbs silently. Each arm has a true and a
// false row, and the flag/value forms (`-e prod`, `-e=prod`, `--environment
// prod`, a persistent flag before the subcommand) are the ones cobra accepts.
func TestRemoteWriteClassifierDrawsEveryArm(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want bool
	}{
		// bootstrap: a remote by default, read-only only with --plan.
		{"bootstrap writes unless it is planned", []string{"bootstrap", "-e", "sandbox", "--no-noise"}, true},
		{"bootstrap --plan is read-only", []string{"bootstrap", "-e", "sandbox", "--no-noise", "--plan"}, false},
		{"bootstrap --fresh --framework x --plan is read-only", []string{"bootstrap", "--fresh", "--framework", "magento2", "--plan"}, false},

		// sync: same shape.
		{"sync writes unless it is planned", []string{"sync", "-s", "sandbox", "--db", "--no-noise"}, true},
		{"sync --plan is read-only", []string{"sync", "-s", "sandbox", "--db", "--no-noise", "--plan"}, false},
		{"sync --media optimized --plan is read-only", []string{"sync", "-s", "sandbox", "--media", "optimized", "--plan"}, false},

		// db import: local unless the environment names a remote.
		{"db import without an environment is local", []string{"db", "import", "--file", "backup.sql"}, false},
		{"db import -e local is local", []string{"db", "import", "-e", "local", "--file", "backup.sql"}, false},
		{"db import -e prod writes through the remote", []string{"db", "import", "-e", "prod", "--file", "backup.sql"}, true},
		{"db import --environment prod writes through the remote", []string{"db", "import", "backup.sql", "--environment", "prod"}, true},
		{"db import -e=prod writes through the remote", []string{"db", "import", "-e=prod"}, true},
		{"db import --stream-db -e prod may still copy an SSH key", []string{"db", "import", "--stream-db", "-e", "prod"}, true},

		// snapshot create/delete/restore: local unless -e names a remote.
		{"snapshot create without an environment is local", []string{"snapshot", "create", "nightly"}, false},
		{"snapshot create -e prod writes through the remote", []string{"snapshot", "create", "nightly", "-e", "prod"}, true},
		{"snapshot delete without an environment is local", []string{"snapshot", "delete", "nightly"}, false},
		{"snapshot delete -e prod writes through the remote", []string{"snapshot", "delete", "-e", "prod", "nightly"}, true},
		{"snapshot restore nightly is local", []string{"snapshot", "restore", "nightly"}, false},
		{"snapshot restore -e local nightly is local", []string{"snapshot", "restore", "-e", "local", "nightly"}, false},
		{"snapshot restore -e prod nightly writes through the remote", []string{"snapshot", "restore", "-e", "prod", "nightly"}, true},
		{"snapshot restore --environment prod after the name", []string{"snapshot", "restore", "nightly", "--environment", "prod"}, true},
		{"snapshot restore -e=prod is the joined form", []string{"snapshot", "restore", "-e=prod", "nightly"}, true},

		// snapshot push: a remote whatever else the argv says.
		{"snapshot push refuses to run without a remote, so it only ever writes remotely", []string{"snapshot", "push", "nightly"}, true},
		{"snapshot push -e prod", []string{"snapshot", "push", "nightly", "-e", "prod"}, true},

		// The subcommand is the first word after the flags, not argv[1].
		{"a persistent -e before the subcommand still finds push", []string{"snapshot", "-e", "prod", "push", "nightly"}, true},
		{"a persistent -e before the subcommand still finds the local restore", []string{"snapshot", "-e", "local", "restore", "nightly"}, false},
		{"a persistent joined -e before the subcommand", []string{"snapshot", "-e=prod", "push", "nightly"}, true},

		// Anything else under snapshot reads at most.
		{"snapshot pull -e prod reads from a remote and writes nothing there", []string{"snapshot", "pull", "nightly", "-e", "prod"}, false},
		{"snapshot list -e prod only reads", []string{"snapshot", "list", "-e", "prod"}, false},

		// open: the environment decides, and `open -e local` is the local form.
		{"open --help names no environment", []string{"open", "--help"}, false},
		{"open -e local is local", []string{"open", "-e", "local", "db"}, false},
		{"open -e prod opens a remote target", []string{"open", "-e", "prod", "db"}, true},
		{"open -e=prod is the joined form", []string{"open", "-e=prod", "db"}, true},
		{"open -e prod works after the target too", []string{"open", "db", "-e", "prod"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRemoteWriteArgv(tc.argv); got != tc.want {
				t.Errorf("isRemoteWriteArgv(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}
}
