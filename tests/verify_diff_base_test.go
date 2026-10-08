package tests

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// gitRepoForBaseTest makes a real repository on a branch named trunk (so the
// default branch name never matches a candidate by accident) with one commit.
func gitRepoForBaseTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	initGitForBaseTest(t, root)
	return root
}

// initGitForBaseTest turns root into a repository on a branch named trunk with
// one commit.
func initGitForBaseTest(t *testing.T, root string) {
	t.Helper()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "trunk")
	git("commit", "-q", "--allow-empty", "-m", "init")
}

func gitRefForBaseTest(t *testing.T, root, ref string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", root, "update-ref", ref, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git update-ref %s: %v\n%s", ref, err, out)
	}
}

func TestResolveDiffBase(t *testing.T) {
	ctx := context.Background()

	t.Run("an explicit base wins and is not checked", func(t *testing.T) {
		root := gitRepoForBaseTest(t)
		gitRefForBaseTest(t, root, "refs/remotes/origin/master")
		got, ok := verify.ResolveDiffBase(ctx, root, "release/1.2")
		if !ok || got != "release/1.2" {
			t.Fatalf("got %q ok=%v, want the explicit base", got, ok)
		}
	})

	for _, tc := range []struct {
		name string
		refs []string
		want string
	}{
		{"origin/master comes first", []string{"refs/remotes/origin/master", "refs/remotes/origin/main", "refs/heads/master", "refs/heads/main"}, "origin/master"},
		{"origin/main when origin/master is absent", []string{"refs/remotes/origin/main", "refs/heads/master", "refs/heads/main"}, "origin/main"},
		{"local master when no origin ref exists", []string{"refs/heads/master", "refs/heads/main"}, "master"},
		{"local main as the last resort", []string{"refs/heads/main"}, "main"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := gitRepoForBaseTest(t)
			for _, ref := range tc.refs {
				gitRefForBaseTest(t, root, ref)
			}
			got, ok := verify.ResolveDiffBase(ctx, root, "")
			if !ok || got != tc.want {
				t.Fatalf("got %q ok=%v, want %q", got, ok, tc.want)
			}
		})
	}

	t.Run("no candidate exists", func(t *testing.T) {
		if got, ok := verify.ResolveDiffBase(ctx, gitRepoForBaseTest(t), ""); ok {
			t.Fatalf("resolved %q in a repo with none of the candidate refs", got)
		}
	})

	t.Run("not a git repository", func(t *testing.T) {
		if got, ok := verify.ResolveDiffBase(ctx, t.TempDir(), ""); ok {
			t.Fatalf("resolved %q outside a git repository", got)
		}
	})
}

func runP311ForTest(t *testing.T, root, base string) (verify.Evidence, [][]string) {
	t.Helper()
	item, ok := findItem("P3-11")
	if !ok {
		t.Fatal("P3-11 is missing from the registry")
	}
	var argvs [][]string
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		argvs = append(argvs, append([]string(nil), args...))
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake"}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return item.Run(context.Background(), engine.Config{Framework: "laravel"}, verify.VerifyOpts{ProjectRoot: root, BaseRef: base}), argvs
}

func baseArg(argv []string) string {
	for i, a := range argv {
		if a == "--base" && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

func TestDiffRowUsesAResolvedBaseAndSkipsWhenThereIsNone(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	t.Run("a repo without origin/master gets the ref that exists", func(t *testing.T) {
		root := gitRepoForBaseTest(t)
		gitRefForBaseTest(t, root, "refs/heads/main")
		ev, argvs := runP311ForTest(t, root, "")
		if ev.Skipped || len(argvs) != 1 || baseArg(argvs[0]) != "main" {
			t.Fatalf("skipped=%v argvs=%v, want one run with --base main", ev.Skipped, argvs)
		}
	})

	t.Run("an explicit --base wins", func(t *testing.T) {
		root := gitRepoForBaseTest(t)
		gitRefForBaseTest(t, root, "refs/heads/main")
		_, argvs := runP311ForTest(t, root, "feature/x")
		if len(argvs) != 1 || baseArg(argvs[0]) != "feature/x" {
			t.Fatalf("argvs=%v, want --base feature/x", argvs)
		}
	})

	t.Run("no base anywhere is a reported skip that says to pass --base", func(t *testing.T) {
		ev, argvs := runP311ForTest(t, gitRepoForBaseTest(t), "")
		if !ev.Skipped || !strings.Contains(ev.SkipReason, "--base") {
			t.Fatalf("skipped=%v reason=%q, want a skip naming --base", ev.Skipped, ev.SkipReason)
		}
		if len(argvs) != 0 {
			t.Fatalf("ran %v without a base: govard would widen the diff scope to the whole project", argvs)
		}
	})

	t.Run("the rest of the argv is unchanged", func(t *testing.T) {
		root := gitRepoForBaseTest(t)
		gitRefForBaseTest(t, root, "refs/remotes/origin/master")
		_, argvs := runP311ForTest(t, root, "")
		want := []string{"audit", "run", "--checks", "lint", "--scope", "diff", "--base", "origin/master", "--format", "json"}
		if len(argvs) != 1 || !reflect.DeepEqual(argvs[0][:len(want)], want) {
			t.Fatalf("argv = %v, want prefix %v", argvs, want)
		}
	})
}

// gitRepoWithBaseForTest is a project root whose diff row has a base to use:
// P3-11 skips in a root without one, so fences that need its argv seed this.
func gitRepoWithBaseForTest(t *testing.T) string {
	t.Helper()
	root := gitRepoForBaseTest(t)
	gitRefForBaseTest(t, root, "refs/remotes/origin/master")
	return root
}

func TestPlanTitleOfTheDiffRowNamesTheResolvedBase(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	commandOf := func(root, base string) string {
		res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "laravel"}, 3, verify.VerifyOpts{Plan: true, ProjectRoot: root, BaseRef: base})
		if err != nil {
			t.Fatalf("RunPhase: %v", err)
		}
		for _, it := range res.Items {
			if it.ID == "P3-11" {
				return it.Command
			}
		}
		t.Fatal("P3-11 missing from the plan")
		return ""
	}
	root := gitRepoForBaseTest(t)
	gitRefForBaseTest(t, root, "refs/heads/main")
	if got := commandOf(root, ""); strings.Contains(got, "{{") || !strings.Contains(got, "--base main ") {
		t.Fatalf("title = %q, want the resolved base main and no placeholder", got)
	}
	if got := commandOf(root, "release/1"); !strings.Contains(got, "--base release/1 ") {
		t.Fatalf("title = %q, want the explicit base", got)
	}
	if got := commandOf(gitRepoForBaseTest(t), ""); strings.Contains(got, "{{") {
		t.Fatalf("title = %q, an unresolvable base must not leave the placeholder", got)
	}
}
