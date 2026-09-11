package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// pushingToOrigin adds one commit on top of the origin's branch and returns it.
// The docroot clone made beforehand therefore does not contain it, which is what
// makes "the docroot must be fetched" observable.
func pushingToOrigin(t *testing.T, origin string) string {
	t.Helper()
	work := t.TempDir()
	run(t, work, "clone", "-q", "--branch", "main", origin, ".")
	writeFile(t, filepath.Join(work, "app.txt"), "a newer revision\n")
	run(t, work, "add", ".")
	run(t, work, "commit", "-q", "-m", "newer")
	revision := run(t, work, "rev-parse", "HEAD")
	run(t, work, "push", "-q", "origin", "main")
	return revision
}

// The in-place docroot fetch is the one network call an in-place publish needs.
// Spec 7.3/9.2 put it in prepare, before any maintenance window exists: a fetch
// that stalls while the site is down is how a deploy becomes an outage.
func TestInPlaceCodePrepFetchesTheDocrootDuringPrepare(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	revision := pushingToOrigin(t, origin)

	if _, err := runner.Run(ctx, "git -C "+host.CurrentPath+" cat-file -e "+revision, deploy.RunOptions{}); err == nil {
		t.Fatal("precondition: the docroot already holds the revision, so this test proves nothing")
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:     "local",
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
	})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	sc.Release.Path = host.ReleasePath("1")
	if _, err := runner.Run(ctx, "mkdir -p "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	if err := deploy.CoreCode(ctx, sc); err != nil {
		t.Fatalf("deploy:code: %v", err)
	}
	if _, err := runner.Run(ctx, "git -C "+host.CurrentPath+" cat-file -e "+revision, deploy.RunOptions{}); err != nil {
		t.Fatalf("deploy:code did not fetch the revision into the docroot: %v", err)
	}
}

func TestInPlaceCodePrepRefusesANonGitDocrootBeforeBuildingAnything(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.CurrentPath+" "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed docroot: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:     "local",
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
	})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	sc.Release.Path = host.ReleasePath("1")

	if err := deploy.CoreCode(ctx, sc); !errors.Is(err, deploy.ErrDocrootNotAGitCheckout) {
		t.Fatalf("err = %v, want ErrDocrootNotAGitCheckout", err)
	}
}

func TestCodePrepLeavesASymlinkTargetWithoutADocroot(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:     "local",
		Publish:    deploy.PublishAuto,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
	})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	sc.Release.Path = host.ReleasePath("1")
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	if err := deploy.CoreCode(ctx, sc); err != nil {
		t.Fatalf("deploy:code: %v", err)
	}
	// The docroot does not exist yet on a symlink target, and prepare must not
	// create it: `current` is the symlink the publish stage swaps.
	if _, err := host.Runner().Run(ctx, "test ! -e "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("a symlink target must not gain a docroot during prepare: %v", err)
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

// A record that does not say how the release was published cannot have its live
// revision checked, and the verify stage must say so instead of passing.
//
// This is the guard behind the resume bug: `CoreVerify` switches on
// `Release.Publish.Strategy` with no default, so a release reconstructed from a
// few stored fields verified nothing and still recorded status "ok".
func TestVerifyFailsWhenTheRecordDoesNotNameAPublishStrategy(t *testing.T) {
	for _, strategy := range []string{"", "by-hand"} {
		t.Run("strategy="+strategy, func(t *testing.T) {
			host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
			release := deploy.NewReleaseForTest("1", "abc", "local")
			release.Publish.Strategy = strategy
			sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Verify: true})
			sc.Release = release

			err := deploy.CoreVerify(context.Background(), sc)
			if err == nil {
				t.Fatal("verify passed without checking the live revision")
			}
			if release.Verify.Status != "failed" {
				t.Fatalf("verify status = %q, want failed", release.Verify.Status)
			}
			if len(release.Verify.Checks) == 0 || release.Verify.Checks[0].ID != "revision" {
				t.Fatalf("checks = %+v, want a failed revision check", release.Verify.Checks)
			}
		})
	}
}

// verifiedRelease seeds a release that its own revision check passes for, so a
// test can reach the later checks.
func verifiedRelease(t *testing.T, host deploy.Host) *deploy.Release {
	t.Helper()
	release := deploy.NewReleaseForTest("1", "abc", "local")
	release.Path = host.ReleasePath("1")
	if err := os.MkdirAll(release.Path, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.Symlink(release.Path, host.CurrentPath); err != nil {
		t.Fatalf("symlink current: %v", err)
	}
	release.Publish.Strategy = deploy.PublishSymlink
	return release
}

// Spec 11: the checks are "recipe-provided Check values plus the generic HTTP
// check". The core cannot know how to touch a framework's real dependencies, so
// the recipe supplies the command and the engine runs it in the verify stage.
func TestVerifyRunsTheRecipeProvidedChecks(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	marker := filepath.Join(t.TempDir(), "app-check-ran")
	release := verifiedRelease(t, host)

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Verify: true})
	sc.Release = release
	sc.Checks = []deploy.Check{{ID: "app", Title: "the application answers", Command: "touch " + marker}}

	if err := deploy.CoreVerify(context.Background(), sc); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the recipe's check did not run")
	}
	recorded := false
	for _, check := range release.Verify.Checks {
		if check.ID == "app" && check.Status == "ok" {
			recorded = true
		}
	}
	if !recorded {
		t.Fatalf("the check result was not recorded: %+v", release.Verify.Checks)
	}
}

func TestVerifyFailsWhenARecipeCheckFails(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := verifiedRelease(t, host)

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Verify: true})
	sc.Release = release
	sc.Checks = []deploy.Check{{ID: "app", Title: "the application answers", Command: "exit 3"}}

	err := deploy.CoreVerify(context.Background(), sc)
	if err == nil {
		t.Fatal("a failed recipe check must fail the deploy")
	}
	if !strings.Contains(err.Error(), "app") {
		t.Fatalf("the failure must name the check, got %q", err.Error())
	}
	if release.Verify.Status != "failed" {
		t.Fatalf("verify status = %q, want failed", release.Verify.Status)
	}
}

// A check that only makes sense for one publish strategy declares it, so an
// in-place check is not run against a symlink target where it cannot mean
// anything.
func TestVerifySkipsARecipeCheckThatDoesNotApplyToTheStrategy(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	marker := filepath.Join(t.TempDir(), "in-place-only")
	release := verifiedRelease(t, host)

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Verify: true})
	sc.Release = release
	sc.Checks = []deploy.Check{{
		ID: "artifact", Command: "touch " + marker, OnlyForPublishStrategy: deploy.PublishInPlace,
	}}

	if err := deploy.CoreVerify(context.Background(), sc); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("an in-place-only check ran against a symlink release")
	}
}

// Spec 12.4: backups under shared/backups/deploy/ are pruned on the same window
// as releases. A dump exists to roll that release back, so keeping more dumps
// than releases keeps nothing useful and fills the disk instead.
func TestCleanupPrunesBackupsOnTheSameWindowAsReleases(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	for _, release := range []string{"1", "2", "3", "4", "5", "6"} {
		if _, err := runner.Run(ctx, "mkdir -p "+host.SharedBackupPath(release)+" && echo dump > "+host.SharedBackupPath(release)+"/db.sql", deploy.RunOptions{}); err != nil {
			t.Fatalf("seed backup %s: %v", release, err)
		}
	}

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", KeepReleases: 2})
	sc.Release = deploy.NewReleaseForTest("7", "abc", "local")
	if err := deploy.CoreCleanup(ctx, sc); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	for _, release := range []string{"1", "2", "3", "4"} {
		if _, err := runner.Run(ctx, "test ! -e "+host.SharedBackupPath(release), deploy.RunOptions{}); err != nil {
			t.Errorf("backup %s must be pruned on the keep_releases window", release)
		}
	}
	for _, release := range []string{"5", "6"} {
		if _, err := runner.Run(ctx, "test -d "+host.SharedBackupPath(release), deploy.RunOptions{}); err != nil {
			t.Errorf("backup %s is inside the window and must survive", release)
		}
	}
}

// A deploy that never used --db-backup has no backup directory at all, and
// cleanup must not turn that into a failure.
func TestCleanupToleratesAMissingBackupRoot(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", KeepReleases: 2})
	sc.Release = deploy.NewReleaseForTest("1", "abc", "local")
	if err := deploy.CoreCleanup(context.Background(), sc); err != nil {
		t.Fatalf("cleanup without a backup root: %v", err)
	}
}
