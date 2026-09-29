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

// runP209 drives the P2-09 row with a fake executor that records every argv and
// answers exit 1 — the same shape captureItemArgvs uses, but it also returns the
// Evidence, because a skip state is not visible through a capture that keeps
// argv alone.
func runP209(t *testing.T, projectRoot string) (verify.Evidence, [][]string) {
	t.Helper()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	item, ok := findItem("P2-09")
	if !ok {
		t.Fatal("P2-09 is missing from the registry")
	}

	var invocations [][]string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		invocations = append(invocations, append([]string(nil), args...))
		return verify.Evidence{ExitCode: 1, OutputExcerpt: "fake: " + strings.Join(args, " ")}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })

	ev := item.Run(context.Background(), engine.Config{Framework: "magento2"}, verify.VerifyOpts{ProjectRoot: projectRoot})
	return ev, invocations
}

// writeThemeDir creates one frontend theme directory.
func writeThemeDir(t *testing.T, root, vendor, theme string) string {
	t.Helper()
	dir := filepath.Join(root, "app", "design", "frontend", vendor, theme)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create theme %s/%s: %v", vendor, theme, err)
	}
	return dir
}

// writeTailwindMarker turns dir into a Tailwind root: the manifest is the whole
// marker the Hyva rule keys on.
func writeTailwindMarker(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create tailwind dir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{\n  \"scripts\": {\"build\": \"tailwindcss\"}\n}\n"), 0o644); err != nil {
		t.Fatalf("write package.json in %s: %v", dir, err)
	}
}

// writeHyvaTheme adds the marker the Hyva rule keys on.
func writeHyvaTheme(t *testing.T, root, vendor, theme string) string {
	t.Helper()
	tailwindDir := filepath.Join(writeThemeDir(t, root, vendor, theme), "web", "tailwind")
	writeTailwindMarker(t, tailwindDir)
	return tailwindDir
}

// lumaProjectRoot is a Magento 2 project with a theme but no Tailwind: the shape
// the defect wrote into.
func lumaProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	luma := writeThemeDir(t, root, "Magento", "luma")
	for name, body := range map[string]string{
		"theme.xml":            "<theme><title>luma</title></theme>\n",
		"web/css/styles-m.css": "/* luma styles */\n",
	} {
		path := filepath.Join(luma, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

// literalTailwindPrefix is the pre-fix argv element: the never-substituted
// `<hyva-theme>` placeholder, which made the prefix a path relative to the
// project root instead of to the theme.
const literalTailwindPrefix = "web/tailwind"

// hasLiteralTailwindPrefix matches the sequence the design names —
// `npm --prefix web/tailwind` — element by element, so a correct Hyva argv
// (`--prefix app/design/frontend/Acme/default/web/tailwind`) cannot trip it.
func hasLiteralTailwindPrefix(argv []string) bool {
	for i := 0; i+2 < len(argv); i++ {
		if argv[i] == "npm" && argv[i+1] == "--prefix" && argv[i+2] == literalTailwindPrefix {
			return true
		}
	}
	return false
}

// escapesProjectRoot reports a prefix that points outside the project: the `..`
// form a glob metacharacter in the root produces, or an absolute path. It is the
// property the metacharacter tests assert, checked on the argv element itself,
// because that element is what `npm install` would write into.
func escapesProjectRoot(arg string) bool {
	up := ".." + string(filepath.Separator)
	return arg == ".." || strings.HasPrefix(arg, up) || filepath.IsAbs(arg)
}

// P2-09 promises an install into a Hyva theme's Tailwind directory, but it was
// gated on `When: isMagento2` alone and ran `govard tool npm --prefix
// web/tailwind install` — a path that exists in no project, since `web/tailwind`
// is relative to the project root while a Hyva theme keeps it under
// `app/design/frontend/<Vendor>/<theme>/web/tailwind`. On a Luma project the row
// therefore ran and `npm install` wrote a stray `web/tailwind/package-lock.json`
// into the checkout (issue #494).
//
// `Item.When` cannot see the project root (it takes only `engine.Config`), so the
// applicability rule lives inside `Run` and reports `Skip` with the reason, per
// design §4.2/§5.5. The rule is a framework-neutral filesystem one — any
// `<root>/app/design/frontend/*/*/web/tailwind/package.json` — so nothing here
// imports a framework package.
//
// Every assertion is an argv or skip-state assertion on purpose: under a test
// binary `execGovard` short-circuits to exit 0 (internal/verify/exec.go:47), so a
// wrong argv is invisible through the exit code.
func TestP209SkipsOnALumaProject(t *testing.T) {
	root := lumaProjectRoot(t)

	ev, invocations := runP209(t, root)

	if !ev.Skipped {
		t.Fatalf("P2-09 on a Luma project (no web/tailwind/package.json): skipped=%v argv=%v; want a skip, because npm install there writes a stray package-lock.json into the checkout", ev.Skipped, invocations)
	}
	if len(invocations) != 0 {
		t.Fatalf("P2-09 spawned %v on a project with no Hyva theme; a skipped item must run nothing", invocations)
	}

	// The design's fence is registry-wide: no item may resolve the placeholder
	// literal, on this fixture or any other.
	captured := captureItemArgvs(t, engine.Config{Framework: "magento2"}, verify.VerifyOpts{ProjectRoot: root})
	for id, argv := range captured {
		if hasLiteralTailwindPrefix(argv) {
			t.Errorf("%s invoked %v: `--prefix web/tailwind` is the unsubstituted <hyva-theme> placeholder, a path no theme has", id, argv)
		}
		for _, arg := range argv {
			if arg == "<hyva-theme>" {
				t.Errorf("%s invoked %v: the <hyva-theme> placeholder reached the argv", id, argv)
			}
		}
	}
}

func TestP209RunsOnAHyvaProject(t *testing.T) {
	root := t.TempDir()
	writeHyvaTheme(t, root, "Acme", "default")

	ev, invocations := runP209(t, root)

	if ev.Skipped {
		t.Fatalf("P2-09 on a Hyva project (app/design/frontend/Acme/default/web/tailwind/package.json): skipped=%v reason=%q; want it to run", ev.Skipped, ev.SkipReason)
	}
	if len(invocations) != 1 {
		t.Fatalf("P2-09 made %d govard invocations, want 1: %v", len(invocations), invocations)
	}

	// The prefix is the discovered directory, root-relative with forward slashes
	// and no trailing separator: execGovard runs the child with cmd.Dir set to
	// the project root, so a relative path is what makes the artifact portable.
	// The property is asserted before the exact argv because building the prefix
	// with filepath.Join(ProjectRoot, …) is the tempting wrong fix, and it would
	// otherwise only show up as a diff against a string literal.
	for _, arg := range invocations[0] {
		if filepath.IsAbs(arg) || strings.Contains(arg, root) {
			t.Fatalf("P2-09 invoked %v: %q carries this machine's project root, so every run artifact would name the temp directory it was produced in", invocations[0], arg)
		}
	}
	want := []string{"tool", "npm", "--prefix", "app/design/frontend/Acme/default/web/tailwind", "install"}
	if got := invocations[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("P2-09 invoked %v, want %v", got, want)
	}
	if !strings.Contains(ev.OutputExcerpt, "app/design/frontend/Acme/default/web/tailwind") {
		t.Fatalf("P2-09 evidence does not name the theme it installed into: %q", ev.OutputExcerpt)
	}

	// Item.Title is static data (RunPhase copies it into RunItem.Command), so a
	// discovered path cannot appear in it; the title describes the command the
	// item really runs and drops both the placeholder and the "+ run build" half
	// it never performed.
	item, ok := findItem("P2-09")
	if !ok {
		t.Fatal("P2-09 is missing from the registry")
	}
	if wantTitle := "govard tool npm install in the Hyva theme's web/tailwind (Hyva only)"; item.Title != wantTitle {
		t.Fatalf("P2-09 title = %q, want %q", item.Title, wantTitle)
	}
}

func TestP209SkipReasonNamesWhatWasSearched(t *testing.T) {
	ev, _ := runP209(t, t.TempDir())

	if !ev.Skipped {
		t.Fatalf("P2-09 on a project with no app/design/frontend at all: skipped=%v; want a skip", ev.Skipped)
	}
	for _, want := range []string{"web/tailwind/package.json", "app/design/frontend/*/*"} {
		if !strings.Contains(ev.SkipReason, want) {
			t.Fatalf("P2-09 skip reason = %q, want it to name %q so an operator on an unusual theme path can see what was searched", ev.SkipReason, want)
		}
	}
}

// A project may carry several Hyva themes. The choice must be deterministic and
// disclosed rather than silent: the sorted-first match runs, and the evidence
// says the project had more than one.
func TestP209ChoosesTheFirstHyvaThemeInSortedOrder(t *testing.T) {
	root := t.TempDir()
	for _, theme := range []struct{ vendor, name string }{{"Beta", "default"}, {"Acme", "default"}} {
		writeHyvaTheme(t, root, theme.vendor, theme.name)
	}

	ev, invocations := runP209(t, root)

	if ev.Skipped {
		t.Fatalf("P2-09 with two Hyva themes: skipped=%v reason=%q; want it to run", ev.Skipped, ev.SkipReason)
	}
	want := []string{"tool", "npm", "--prefix", "app/design/frontend/Acme/default/web/tailwind", "install"}
	if len(invocations) != 1 || !reflect.DeepEqual(invocations[0], want) {
		t.Fatalf("P2-09 invoked %v, want %v: the first match in sorted order", invocations, want)
	}
	if !strings.Contains(ev.OutputExcerpt, "first of 2 Hyva themes") {
		t.Fatalf("P2-09 evidence = %q, want it to disclose that the project has more than one Hyva theme", ev.OutputExcerpt)
	}
}

// The design's risk list names a theme that is not a plain directory: a vendor or
// theme symlinked into app/design/frontend. The rule resolves it through the link
// rather than skipping — the prefix stays the path the project declares
// (`app/design/frontend/Acme/default/web/tailwind`), and npm writes through the
// symlink into the theme it points at, which is where a Tailwind install belongs.
// A hand-rolled walk that trusts DirEntry.IsDir() would regress this, because a
// symlink reports neither directory nor regular file.
func TestP209FindsAThemeBehindASymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	shared := t.TempDir()
	writeTailwindMarker(t, filepath.Join(shared, "web", "tailwind"))

	vendorDir := filepath.Join(root, "app", "design", "frontend", "Acme")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatalf("create vendor dir %s: %v", vendorDir, err)
	}
	if err := os.Symlink(shared, filepath.Join(vendorDir, "default")); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}

	ev, invocations := runP209(t, root)

	if ev.Skipped {
		t.Fatalf("P2-09 with the theme symlinked into app/design/frontend: skipped=%v reason=%q; want it to run", ev.Skipped, ev.SkipReason)
	}
	want := []string{"tool", "npm", "--prefix", "app/design/frontend/Acme/default/web/tailwind", "install"}
	if len(invocations) != 1 || !reflect.DeepEqual(invocations[0], want) {
		t.Fatalf("P2-09 invoked %v, want %v", invocations, want)
	}
}

// The glob that finds the theme is built from the project root, so a root
// carrying a glob metacharacter used to match a *sibling*: `proj[1]` is a
// character class that matches `proj1`, `filepath.Rel` then yields
// `../proj1/app/design/frontend/...` as the prefix, and `npm install --prefix`
// would write into a different checkout — the "#494 writes where it should not"
// class, on a project that has no theme of its own at all. Low likelihood, so it
// is pinned here, one row per magic character the escaper has to know about
// (`*`, `?`, `[` and the escape character `\`).
//
// No theme may be found through the metacharacter, and no argv element may point
// outside the root.
func TestP209DoesNotMatchASiblingThroughAGlobMetacharacter(t *testing.T) {
	rows := []struct {
		name string
		// rootSuffix carries the metacharacter; sibling is the real directory the
		// unescaped pattern would match instead.
		rootSuffix string
		sibling    string
		// posixOnly marks the row whose metacharacter is a path separator on
		// Windows, where a directory name cannot contain it.
		posixOnly bool
	}{
		{"character class", "proj[1]", "proj1", false},
		{"star", "proj*", "proj1", false},
		{"question mark", "proj?", "proj1", false},
		{"escape character", `proj\1`, "proj1", true},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if row.posixOnly && filepath.Separator == '\\' {
				t.Skip("a backslash is a path separator on this platform")
			}
			base := t.TempDir()
			// The sibling holds the theme; this project has no marker of its own.
			writeHyvaTheme(t, filepath.Join(base, row.sibling), "Acme", "default")
			root := filepath.Join(base, row.rootSuffix)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatalf("create project root %s: %v", root, err)
			}

			ev, invocations := runP209(t, root)

			// The escape check runs first: it is the harm, and the message has to
			// name the prefix that would have written into the sibling.
			for _, argv := range invocations {
				for _, arg := range argv {
					if escapesProjectRoot(arg) {
						t.Fatalf("P2-09 invoked %v: %q escapes the project root %s, so npm install would write into another checkout", argv, arg, root)
					}
				}
			}
			if !ev.Skipped {
				t.Fatalf("P2-09 on %s (no theme of its own, a sibling with a theme): skipped=%v argv=%v; want a skip", root, ev.Skipped, invocations)
			}
			if len(invocations) != 0 {
				t.Fatalf("P2-09 spawned %v on a project with no theme of its own: a skipped item must run nothing", invocations)
			}
		})
	}
}

// The rule's boundary is the glob itself: `app/design/frontend/<Vendor>/<theme>`
// exactly one vendor and one theme deep. A marker anywhere else is not this
// project's theme, and the row must skip rather than install into it — the
// layouts below were previously a coincidence of the fixture, not a promise.
func TestP209SkipsOnLayoutsTheGlobDoesNotCover(t *testing.T) {
	rows := []struct {
		name   string
		marker string
	}{
		{"theme only under vendor", "vendor/hyva-themes/magento2-default-theme/web/tailwind/package.json"},
		{"theme deeper than the glob", "app/design/frontend/Acme/child/default/web/tailwind/package.json"},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			root := t.TempDir()
			writeTailwindMarker(t, filepath.Join(root, filepath.FromSlash(filepath.Dir(row.marker))))

			ev, invocations := runP209(t, root)

			if !ev.Skipped {
				t.Fatalf("P2-09 with a marker at %s: skipped=%v argv=%v; want a skip, because the theme is not at app/design/frontend/<Vendor>/<theme>/web/tailwind", row.marker, ev.Skipped, invocations)
			}
			if len(invocations) != 0 {
				t.Fatalf("P2-09 spawned %v for a marker at %s; want no invocation", invocations, row.marker)
			}
		})
	}
}

// The escaping half has a value of its own, in the direction nobody filed: a
// project whose own checkout path carries a metacharacter must still find the
// theme sitting inside it. Unescaped, `<base>/proj[1]/app/...` is a pattern whose
// class matches only `proj1`, so a project named `proj[1]` silently skips the
// theme it actually has — a false negative that no containment guard can see,
// because no match is produced at all.
func TestP209FindsItsOwnThemeWhenThePathHasAGlobMetacharacter(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "proj[1]")
	writeHyvaTheme(t, root, "Acme", "default")

	ev, invocations := runP209(t, root)

	if ev.Skipped {
		t.Fatalf("P2-09 on %s with its own theme: skipped=%v reason=%q; want it to run", root, ev.Skipped, ev.SkipReason)
	}
	want := []string{"tool", "npm", "--prefix", "app/design/frontend/Acme/default/web/tailwind", "install"}
	if len(invocations) != 1 || !reflect.DeepEqual(invocations[0], want) {
		t.Fatalf("P2-09 invoked %v, want %v", invocations, want)
	}
}
