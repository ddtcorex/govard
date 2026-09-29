package tests

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// P3-13 and P3-14 audit a *module*: their titles promise
// `--mode module_in_project` from `app/code/<Vendor>/<Module>` and
// `--mode standalone` from `/tmp/govard-audit-standalone/<Module>`. Neither
// promise held. `execGovard` runs every child with cmd.Dir set to the project
// root (internal/verify/exec.go:47-49) and `audit run` resolved its target from
// the process cwd alone (internal/cmd/audit.go:552-558), so both rows ran from
// the project root — where `--mode module_in_project` errors with "requires a
// Magento module inside a Magento project" even when the module exists — and the
// standalone fixture the second title names existed nowhere but this registry
// (issue #490).
//
// The fix has three parts, and these tests pin each: a framework-registered
// module discovery (`engine.RegisterVerifySupport`), a `--path` flag on
// `audit run`, and an item that either names the module directory or skips.
// A wrong argv here is invisible through the exit code — execGovard
// short-circuits to exit 0 under a test binary (internal/verify/exec.go:40-42) —
// so every assertion below reads the captured argv or the skip, never "the item
// failed".

// standaloneAuditFixtureRootForTest is the parent the P3-14 title names.
func standaloneAuditFixtureRootForTest() string {
	return filepath.Join(os.TempDir(), "govard-audit-standalone")
}

// freeStandaloneAuditFixture removes the fixture path for moduleName before and
// after a test, so the item's exclusive creation has somewhere to land: the item
// skips on a path it did not create, which means a test that wants the normal
// path has to start from an empty one. Each test names its own module, so no test
// clears a path another one is using.
func freeStandaloneAuditFixture(t *testing.T, moduleName string) string {
	t.Helper()
	fixtureDir := filepath.Join(standaloneAuditFixtureRootForTest(), moduleName)
	remove := func() { _ = os.RemoveAll(fixtureDir) }
	remove()
	t.Cleanup(remove)
	return fixtureDir
}

// magentoProjectWithAppCodeModule writes the tree the Magento 2 hook discovers:
// app/code/<Vendor>/<Module>/etc/module.xml below an otherwise empty project
// root. The item tests need nothing else — they capture argv instead of running
// a child — so the fixture stays smaller than a Magento project on purpose.
func magentoProjectWithAppCodeModule(t *testing.T, module string) (projectRoot, moduleRoot string) {
	t.Helper()
	projectRoot = t.TempDir()
	moduleRoot = filepath.Join(projectRoot, "app", "code", "Acme", module)
	if err := os.MkdirAll(filepath.Join(moduleRoot, "etc"), 0o700); err != nil {
		t.Fatal(err)
	}
	declaration := `<?xml version="1.0"?><config><module name="Acme_` + module + `"/></config>`
	if err := os.WriteFile(filepath.Join(moduleRoot, "etc", "module.xml"), []byte(declaration), 0o600); err != nil {
		t.Fatal(err)
	}
	return projectRoot, moduleRoot
}

// writeFixtureFile creates path with its parents, for the pre-existing-tree rows
// below.
func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// captureAuditTargetItem runs one registry item with a fake exec that records
// every argv it saw and answers exit 1, which ends P3-13/P3-14 at their single
// invocation. onExec observes the item's state at the moment the child would
// have started — the only point at which "the fixture existed while the item
// ran" can be asserted.
func captureAuditTargetItem(t *testing.T, id string, cfg engine.Config, opts verify.VerifyOpts, onExec func(argv []string)) (verify.Evidence, [][]string) {
	t.Helper()
	item, ok := findItem(id)
	if !ok {
		t.Fatalf("%s is missing from the registry", id)
	}
	var argvs [][]string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		argv := append([]string(nil), args...)
		argvs = append(argvs, argv)
		if onExec != nil {
			onExec(argv)
		}
		return verify.Evidence{ExitCode: 1, OutputExcerpt: "fake: " + strings.Join(argv, " ")}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return item.Run(context.Background(), cfg, opts), argvs
}

// auditsTheModule is the argv both items must build once a module is known:
// `audit run --checks lint --mode <mode> --format json --path <dir>`.
func auditsTheModule(mode, dir string) []string {
	return []string{"audit", "run", "--checks", "lint", "--mode", mode, "--format", "json", "--path", dir}
}

// A project with no module cannot support either mode, and neither can a
// framework that registers no module discovery at all. Both rows must survive as
// skips — a `When` would have dropped them from the report (§4.1) — and neither
// may run an audit against a directory that cannot answer it.
func TestP313AndP314SkipWithoutAModule(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	for _, tc := range []struct {
		name      string
		framework string
		project   string
	}{
		{name: "magento project with no app/code module", framework: "magento2", project: t.TempDir()},
		{name: "framework that registers no module discovery", framework: "laravel", project: t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, id := range []string{"P3-13", "P3-14"} {
				ev, argvs := captureAuditTargetItem(t, id, engine.Config{Framework: tc.framework},
					verify.VerifyOpts{ProjectRoot: tc.project}, nil)

				if !ev.Skipped {
					t.Fatalf("%s Skipped = false (exit %d, excerpt %q); with no module to audit the row must skip",
						id, ev.ExitCode, ev.OutputExcerpt)
				}
				if !strings.Contains(ev.SkipReason, "module") {
					t.Fatalf("%s skip reason = %q, want it to name the module requirement", id, ev.SkipReason)
				}
				for _, argv := range argvs {
					if len(argv) > 1 && argv[0] == "audit" && argv[1] == "run" {
						t.Fatalf("%s skipped but still ran %v", id, argv)
					}
				}
			}
		})
	}
}

// The module the mode requires is named on the argv: from a project root whose
// app/code carries one module, P3-13 points the audit at that module instead of
// at the project it was launched from.
func TestP313AuditsFromTheModuleDirectory(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	projectRoot, moduleRoot := magentoProjectWithAppCodeModule(t, "Demo")

	ev, argvs := captureAuditTargetItem(t, "P3-13", engine.Config{Framework: "magento2"},
		verify.VerifyOpts{ProjectRoot: projectRoot}, nil)

	if ev.Skipped {
		t.Fatalf("P3-13 skipped on a project with app/code/Acme/Demo: %q", ev.SkipReason)
	}
	want := auditsTheModule("module_in_project", moduleRoot)
	if len(argvs) != 1 || !reflect.DeepEqual(argvs[0], want) {
		t.Fatalf("P3-13 invoked %v, want %v", argvs, want)
	}
}

// P3-14's title names a fixture directory, and that directory has to be built
// before the child runs: the copy is complete at the moment execGovard would have
// started the audit, and removed once the item is done. A `--path` pointing at a
// directory that never existed is the red this item shipped with.
func TestP314CreatesTheStandaloneFixtureItNames(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	const module = "Demo"
	projectRoot, moduleRoot := magentoProjectWithAppCodeModule(t, module)
	registration := filepath.Join(moduleRoot, "registration.php")
	if err := os.WriteFile(registration, []byte("<?php\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	wantFixture := freeStandaloneAuditFixture(t, module)

	fixtureExisted := false
	copyComplete := false
	ev, argvs := captureAuditTargetItem(t, "P3-14", engine.Config{Framework: "magento2"},
		verify.VerifyOpts{ProjectRoot: projectRoot}, func(argv []string) {
			dir := argvFlagValue(argv, "--path")
			if dir == "" {
				return
			}
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				fixtureExisted = true
			}
			if _, err := os.Stat(filepath.Join(dir, "etc", "module.xml")); err == nil {
				if _, err := os.Stat(filepath.Join(dir, "registration.php")); err == nil {
					copyComplete = true
				}
			}
		})

	if ev.Skipped {
		t.Fatalf("P3-14 skipped on a project with app/code/Acme/%s: %q", module, ev.SkipReason)
	}
	want := auditsTheModule("standalone", wantFixture)
	if len(argvs) != 1 || !reflect.DeepEqual(argvs[0], want) {
		t.Fatalf("P3-14 invoked %v, want %v", argvs, want)
	}
	if !fixtureExisted {
		t.Fatalf("the fixture %s did not exist while P3-14 ran", wantFixture)
	}
	if !copyComplete {
		t.Fatalf("the fixture %s was not a complete copy of %s when the audit started", wantFixture, moduleRoot)
	}
	if _, err := os.Stat(wantFixture); !os.IsNotExist(err) {
		t.Fatalf("P3-14 left %s behind (stat err = %v); the copy is scratch state", wantFixture, err)
	}
	if _, err := os.Stat(filepath.Join(moduleRoot, "etc", "module.xml")); err != nil {
		t.Fatalf("P3-14 removed the project's own module: %v", err)
	}
}

// The titled path is shared machine state, so the item may only ever consume what
// it created itself. The first item never removes a directory it did not create:
// a tree the operator already keeps at that path is reported as occupied and left
// exactly as it was. The second row is the same mechanism read as a race — two
// concurrent checklist runs whose projects contain a module of the same name —
// where the loser must skip with an actionable reason instead of reddening (its
// copy would have nothing to write to) or deleting the winner's fixture.
func TestP314SkipsWhenTheFixturePathIsAlreadyTaken(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	for _, tc := range []struct {
		name    string
		module  string
		prepare func(t *testing.T, fixtureDir string)
		assert  func(t *testing.T, fixtureDir string)
	}{
		{
			name:   "a directory the item did not create is left untouched",
			module: "DemoOccupied",
			prepare: func(t *testing.T, fixtureDir string) {
				writeFixtureFile(t, filepath.Join(fixtureDir, "keep.txt"), "not the item's")
			},
			assert: func(t *testing.T, fixtureDir string) {
				content, err := os.ReadFile(filepath.Join(fixtureDir, "keep.txt"))
				if err != nil {
					t.Fatalf("the pre-existing file is gone: %v", err)
				}
				if string(content) != "not the item's" {
					t.Fatalf("the pre-existing file was rewritten: %q", content)
				}
			},
		},
		{
			name:   "the loser of a same-named race skips instead of reddening",
			module: "DemoRace",
			prepare: func(t *testing.T, fixtureDir string) {
				writeFixtureFile(t, filepath.Join(fixtureDir, "etc", "module.xml"), "owned by the winning run")
			},
			assert: func(t *testing.T, fixtureDir string) {
				content, err := os.ReadFile(filepath.Join(fixtureDir, "etc", "module.xml"))
				if err != nil {
					t.Fatalf("the winner's fixture is gone: %v", err)
				}
				if string(content) != "owned by the winning run" {
					t.Fatalf("the losing run overwrote the winner's copy: %q", content)
				}
				if _, err := os.Stat(filepath.Join(fixtureDir, "registration.php")); !os.IsNotExist(err) {
					t.Fatal("the losing run copied into the winning run's fixture")
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot, moduleRoot := magentoProjectWithAppCodeModule(t, tc.module)
			if err := os.WriteFile(filepath.Join(moduleRoot, "registration.php"), []byte("<?php\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			fixtureDir := freeStandaloneAuditFixture(t, tc.module)
			if err := os.MkdirAll(fixtureDir, 0o700); err != nil {
				t.Fatal(err)
			}
			tc.prepare(t, fixtureDir)

			ev, argvs := captureAuditTargetItem(t, "P3-14", engine.Config{Framework: "magento2"},
				verify.VerifyOpts{ProjectRoot: projectRoot}, nil)

			if !ev.Skipped {
				t.Fatalf("P3-14 ran against a fixture path it did not create (exit %d, excerpt %q, argv %v); it must skip",
					ev.ExitCode, ev.OutputExcerpt, argvs)
			}
			if ev.ExitCode != 0 {
				t.Fatalf("P3-14 reported exit %d for a taken fixture path; a taken path is a skip, never a red", ev.ExitCode)
			}
			if !strings.Contains(ev.SkipReason, fixtureDir) {
				t.Fatalf("P3-14 skip reason = %q, want it to name the occupied path %s", ev.SkipReason, fixtureDir)
			}
			if !strings.Contains(ev.SkipReason, "remove it") {
				t.Fatalf("P3-14 skip reason = %q, want it to tell the operator what to do with the directory", ev.SkipReason)
			}
			if len(argvs) != 0 {
				t.Fatalf("P3-14 invoked %v; a taken fixture path must not run an audit", argvs)
			}
			if _, err := os.Stat(fixtureDir); err != nil {
				t.Fatalf("the directory at the titled path %s was removed: %v", fixtureDir, err)
			}
			tc.assert(t, fixtureDir)
		})
	}
}

// The rider this task carries: P3-14 omitted the conditional `--allow-xdebug`
// that its siblings build (P3-10/P3-11/P3-12/P3-13/P5-04). Without it an
// xdebug-enabled project's audit exits 1 with the guard error and no JSON, so
// the row was a false red of the same class as #490.
//
// The module name differs from the other P3-14 tests so each one owns its own
// titled path — and it proves the fixture path is the *module's* basename rather
// than a name baked into the item.
func TestP313AndP314ForwardAllowXdebug(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	const module = "DemoXdebug"
	projectRoot, moduleRoot := magentoProjectWithAppCodeModule(t, module)
	fixtureDir := freeStandaloneAuditFixture(t, module)

	for _, tc := range []struct {
		id   string
		want []string
	}{
		{id: "P3-13", want: append(auditsTheModule("module_in_project", moduleRoot), "--allow-xdebug")},
		{id: "P3-14", want: append(auditsTheModule("standalone", fixtureDir), "--allow-xdebug")},
	} {
		t.Run(tc.id, func(t *testing.T) {
			_, argvs := captureAuditTargetItem(t, tc.id, engine.Config{Framework: "magento2"},
				verify.VerifyOpts{ProjectRoot: projectRoot, AllowXdebug: true}, nil)

			if len(argvs) != 1 || !reflect.DeepEqual(argvs[0], tc.want) {
				t.Fatalf("%s with AllowXdebug invoked %v, want %v", tc.id, argvs, tc.want)
			}
		})
	}
}
