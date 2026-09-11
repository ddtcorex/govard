package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

func TestSymlinkActivationIsAnAtomicRenameAndKeepsTheOldReleaseUntilThen(t *testing.T) {
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	for _, n := range []string{"1", "2"} {
		if _, err := runner.Run(ctx, "mkdir -p "+host.ReleasePath(n), deploy.RunOptions{}); err != nil {
			t.Fatalf("seed release %s: %v", n, err)
		}
	}
	if _, err := runner.Run(ctx, "ln -sfn "+host.ReleasePath("1")+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("seed current: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Publish: deploy.PublishSymlink})
	sc.Release = deploy.NewReleaseForTest("2", "abc", "local")
	sc.Release.Path = host.ReleasePath("2")
	if err := deploy.CoreActivate(ctx, sc); err != nil {
		t.Fatalf("activate: %v", err)
	}

	resolved, err := runner.Run(ctx, "readlink -f "+host.CurrentPath, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if strings.TrimSpace(resolved.Stdout) != host.ReleasePath("2") {
		t.Fatalf("current -> %q, want %q", strings.TrimSpace(resolved.Stdout), host.ReleasePath("2"))
	}
	if _, err := runner.Run(ctx, "test ! -e "+host.CurrentPath+".govard-tmp", deploy.RunOptions{}); err != nil {
		t.Fatalf("temporary symlink left behind: %v", err)
	}
	if sc.Release.Publish.Strategy != deploy.PublishSymlink {
		t.Fatalf("recorded strategy = %q, want symlink", sc.Release.Publish.Strategy)
	}
}

func TestAutoPublishStrategyFollowsTheExistingLayout(t *testing.T) {
	ctx := context.Background()

	absent := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	got, err := deploy.ResolvePublishStrategy(absent, deploy.Options{Publish: deploy.PublishAuto})
	if err != nil {
		t.Fatalf("resolve for absent current: %v", err)
	}
	if got != deploy.PublishSymlink {
		t.Fatalf("absent current -> %q, want symlink", got)
	}

	real := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	if _, err := real.Runner().Run(ctx, "mkdir -p "+real.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("seed real docroot: %v", err)
	}
	got, err = deploy.ResolvePublishStrategy(real, deploy.Options{Publish: deploy.PublishAuto})
	if err != nil {
		t.Fatalf("resolve for real docroot: %v", err)
	}
	if got != deploy.PublishInPlace {
		t.Fatalf("real docroot -> %q, want in_place", got)
	}
}

func TestInPlaceActivationResetsToTheExactRevisionAndSyncsConfiguredPaths(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	if _, err := runner.Run(ctx, "mkdir -p "+host.ReleasePath("2")+"/vendor && echo built > "+host.ReleasePath("2")+"/vendor/autoload.php", deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:     "local",
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings:   map[string]any{"sync_paths": []string{"vendor"}},
	})
	sc.Release = deploy.NewReleaseForTest("2", revision, "main")
	sc.Release.Path = host.ReleasePath("2")
	if err := deploy.CoreActivate(ctx, sc); err != nil {
		t.Fatalf("activate: %v", err)
	}

	head, err := runner.Run(ctx, "git -C "+host.CurrentPath+" rev-parse HEAD", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if strings.TrimSpace(head.Stdout) != revision {
		t.Fatalf("docroot HEAD = %q, want the deployed revision %q", strings.TrimSpace(head.Stdout), revision)
	}
	synced, err := runner.Run(ctx, "cat "+host.CurrentPath+"/vendor/autoload.php", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("read synced vendor: %v", err)
	}
	if strings.TrimSpace(synced.Stdout) != "built" {
		t.Fatalf("vendor/autoload.php = %q, want the release's content", synced.Stdout)
	}
}

func TestInPlaceActivationRefusesADocrootThatIsNotAGitCheckout(t *testing.T) {
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.CurrentPath+" "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed docroot: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Publish: deploy.PublishInPlace, Revision: "abc"})
	sc.Release = deploy.NewReleaseForTest("1", "abc", "main")
	sc.Release.Path = host.ReleasePath("1")
	if err := deploy.CoreActivate(ctx, sc); !errors.Is(err, deploy.ErrDocrootNotAGitCheckout) {
		t.Fatalf("err = %v, want ErrDocrootNotAGitCheckout", err)
	}
}

func TestVerifyChecksTheLiveRevision(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	if _, err := runner.Run(ctx, "git -C "+host.CurrentPath+" checkout -q "+revision, deploy.RunOptions{}); err != nil {
		t.Fatalf("checkout revision: %v", err)
	}

	release := deploy.NewReleaseForTest("1", revision, "main")
	release.Path = host.ReleasePath("1")
	release.Publish.Strategy = deploy.PublishInPlace
	release.Publish.Docroot = host.CurrentPath

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Verify: true})
	sc.Release = release
	if err := deploy.CoreVerify(ctx, sc); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if release.Verify.Status != "ok" {
		t.Fatalf("verify status = %q, want ok", release.Verify.Status)
	}

	release.Revision = "different"
	if err := deploy.CoreVerify(ctx, sc); err == nil {
		t.Fatal("verify must fail when the docroot revision does not match")
	}
}

func TestVerifyIsSkippedWhenDisabled(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Verify: false})
	sc.Release = deploy.NewReleaseForTest("1", "abc", "main")
	if err := deploy.CoreVerify(context.Background(), sc); err != nil {
		t.Fatalf("--no-verify must skip verification entirely: %v", err)
	}
}

func TestVerifyURLChecksTheStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()
	if err := deploy.VerifyURL(context.Background(), server.URL, 5*time.Second); err == nil {
		t.Fatal("want an error for a 418 response")
	}

	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	if err := deploy.VerifyURL(context.Background(), ok.URL, 5*time.Second); err != nil {
		t.Fatalf("200 must pass: %v", err)
	}
}

func TestCleanupKeepsTheNewestReleasesAndNeverTouchesForeignOnes(t *testing.T) {
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	for _, n := range []string{"1", "2", "3", "4"} {
		if err := deploy.WriteRelease(ctx, host, deploy.NewReleaseForTest(n, "rev"+n, "local")); err != nil {
			t.Fatalf("seed release %s: %v", n, err)
		}
	}
	// A release created by another tool: a directory with no govard record.
	if _, err := runner.Run(ctx, "mkdir -p "+host.ReleasePath("deployer-made"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed foreign release: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", KeepReleases: 2})
	if err := deploy.CoreCleanup(ctx, sc); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	for _, n := range []string{"1", "2"} {
		if _, err := runner.Run(ctx, "test -d "+host.ReleasePath(n), deploy.RunOptions{}); err == nil {
			t.Errorf("release %s should have been pruned", n)
		}
	}
	for _, n := range []string{"3", "4", "deployer-made"} {
		if _, err := runner.Run(ctx, "test -d "+host.ReleasePath(n), deploy.RunOptions{}); err != nil {
			t.Errorf("release %s must survive cleanup: %v", n, err)
		}
	}
}

func TestRecordWritesTheReleaseAndTheHistory(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	release := deploy.NewReleaseForTest("5", "abc", "main")
	release.Path = host.ReleasePath("5")

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local"})
	sc.Release = release
	if err := deploy.CoreRecord(ctx, sc); err != nil {
		t.Fatalf("record: %v", err)
	}

	stored, err := deploy.ReadRelease(ctx, host, "5")
	if err != nil {
		t.Fatalf("read release: %v", err)
	}
	if stored.Revision != "abc" {
		t.Fatalf("stored revision = %q, want abc", stored.Revision)
	}
	if _, err := host.Runner().Run(ctx, "test -s "+host.HistoryPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("history must be written: %v", err)
	}
}
