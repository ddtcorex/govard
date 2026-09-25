package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/frameworks/magento2"
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

	sc := deploy.StepContextForTest(host, deploy.Options{Publish: deploy.PublishSymlink})
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

// An in-place docroot is the live application, and its configuration is not the
// checkout's to replace.
//
// `app/etc/env.php` is untracked in a Magento project and holds the database
// credentials, so the docroot's own copy is the one the site reads. Nothing in
// the pipeline linked the configured shared entries into the docroot —
// `deploy:shared` links the release — so an in-place deploy finished with the
// live site reading whatever the checkout carried: on a real 2.4.9 target
// `maintenance:disable` failed with `Connection "default" is not defined` and the
// storefront answered every request with a redirect to `/setup/`.
//
// The docroot's copy is adopted into `shared/` rather than overwritten: the live
// configuration becomes the shared state, and the release that was built for this
// deploy links the same file.
func TestInPlaceActivationAdoptsTheDocrootsConfigurationIntoShared(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	const live = "<?php return ['db' => ['host' => 'the live database']];\n"
	writeFile(t, filepath.Join(host.CurrentPath, "app/etc/env.php"), live)

	sc := deploy.StepContextForTest(host, deploy.Options{
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings: map[string]any{
			"shared_files": []string{"app/etc/env.php"},
			"sync_paths":   []string{"vendor"},
		},
	})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	sc.Release.Path = host.ReleasePath("1")
	if err := deploy.CoreActivate(ctx, sc); err != nil {
		t.Fatalf("activate: %v", err)
	}

	shared := filepath.Join(host.SharedPath(), "app/etc/env.php")
	adopted, err := os.ReadFile(shared)
	if err != nil {
		t.Fatalf("the docroot's configuration must be adopted into shared/: %v", err)
	}
	if string(adopted) != live {
		t.Fatalf("shared/app/etc/env.php = %q, want the docroot's own file", adopted)
	}
	served, err := os.ReadFile(filepath.Join(host.CurrentPath, "app/etc/env.php"))
	if err != nil {
		t.Fatalf("read the served configuration: %v", err)
	}
	if string(served) != live {
		t.Fatalf("the served configuration = %q, want the adopted one", served)
	}
	resolved, err := runner.Run(ctx, "readlink -f "+filepath.Join(host.CurrentPath, "app/etc/env.php"), deploy.RunOptions{})
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if strings.TrimSpace(resolved.Stdout) != shared {
		t.Fatalf("the docroot must read the shared file, got %q, want %q", strings.TrimSpace(resolved.Stdout), shared)
	}
}

// The other half, and the one the rehearsal hit: `shared/` already holds the
// configuration, and the docroot carries something else at that path — the
// placeholder a checkout or a reset leaves. The shared file is canonical, so the
// docroot reads it again once the deploy is activated.
func TestInPlaceActivationRestoresTheDocrootsSharedLinks(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	const canonical = "<?php return ['db' => ['host' => 'the live database']];\n"
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), canonical)
	writeFile(t, filepath.Join(host.CurrentPath, "app/etc/env.php"), "<?php return ['cache_types' => []];\n")

	sc := deploy.StepContextForTest(host, deploy.Options{
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings:   map[string]any{"shared_files": []string{"app/etc/env.php"}},
	})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	sc.Release.Path = host.ReleasePath("1")
	if err := deploy.CoreActivate(ctx, sc); err != nil {
		t.Fatalf("activate: %v", err)
	}

	served, err := os.ReadFile(filepath.Join(host.CurrentPath, "app/etc/env.php"))
	if err != nil {
		t.Fatalf("read the served configuration: %v", err)
	}
	if string(served) != canonical {
		t.Fatalf("the served configuration = %q, want the shared one", served)
	}
}

// A real directory the docroot still owns is not deleted to make room for a
// shared one: media that exists in both places is somebody's data, and losing it
// is worse than not sharing it.
func TestInPlaceActivationKeepsADocrootDirectoryItWouldHaveToDelete(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	writeFile(t, filepath.Join(host.SharedPath(), "pub/media/from-shared.txt"), "shared\n")
	writeFile(t, filepath.Join(host.CurrentPath, "pub/media/local.txt"), "local\n")

	sc := deploy.StepContextForTest(host, deploy.Options{
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings:   map[string]any{"shared_dirs": []string{"pub/media"}},
	})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	sc.Release.Path = host.ReleasePath("1")
	if err := deploy.CoreActivate(ctx, sc); err != nil {
		t.Fatalf("activate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(host.CurrentPath, "pub/media/local.txt")); err != nil {
		t.Fatalf("the docroot's own media must survive the activation: %v", err)
	}
}

func TestInPlaceActivationRefusesADocrootThatIsNotAGitCheckout(t *testing.T) {
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.CurrentPath+" "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed docroot: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{Publish: deploy.PublishInPlace, Revision: "abc"})
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

// Adoption has to happen before the build, not only at activation: the build
// steps run in the release, and a release whose `app/etc/env.php` is the
// placeholder from the archive has no database connection — the deploy then dies
// minutes later at `db:migrate`, for a reason that reads like a target fault.
func TestInPlaceCodePrepAdoptsTheDocrootsConfigurationBeforeTheBuild(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	const live = "<?php return ['db' => ['host' => 'the live database']];\n"
	writeFile(t, filepath.Join(host.CurrentPath, "app/etc/env.php"), live)
	if _, err := runner.Run(ctx, "mkdir -p "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings:   map[string]any{"shared_files": []string{"app/etc/env.php"}},
	})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	sc.Release.Path = host.ReleasePath("1")

	if err := deploy.CoreCode(ctx, sc); err != nil {
		t.Fatalf("deploy:code: %v", err)
	}
	// `deploy:code` adopts the docroot's file into shared/; `deploy:shared`, the
	// step right after it, is what links it into the release — so everything
	// built there, the DI compile and the database migration included, reads the
	// live configuration instead of the archive's placeholder.
	if err := deploy.CoreShared(ctx, sc); err != nil {
		t.Fatalf("deploy:shared: %v", err)
	}
	served, err := os.ReadFile(filepath.Join(host.ReleasePath("1"), "app/etc/env.php"))
	if err != nil {
		t.Fatalf("read the release's configuration: %v", err)
	}
	if string(served) != live {
		t.Fatalf("the release's configuration = %q, want the docroot's own file", served)
	}
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

	sc := deploy.StepContextForTest(host, deploy.Options{Verify: true})
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
	sc := deploy.StepContextForTest(host, deploy.Options{Verify: false})
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
	if err := deploy.VerifyURL(context.Background(), server.URL, 5*time.Second, deploy.VerifyPolicy{}); err == nil {
		t.Fatal("want an error for a 418 response")
	}

	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	if err := deploy.VerifyURL(context.Background(), ok.URL, 5*time.Second, deploy.VerifyPolicy{}); err != nil {
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

	sc := deploy.StepContextForTest(host, deploy.Options{KeepReleases: 2})
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

	sc := deploy.StepContextForTest(host, deploy.Options{})
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
			sc := deploy.StepContextForTest(host, deploy.Options{Verify: true})
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

	sc := deploy.StepContextForTest(host, deploy.Options{Verify: true})
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

	sc := deploy.StepContextForTest(host, deploy.Options{Verify: true})
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

	sc := deploy.StepContextForTest(host, deploy.Options{Verify: true})
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

	sc := deploy.StepContextForTest(host, deploy.Options{KeepReleases: 2})
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

// A directory under shared/backups/deploy/ that govard did not name must not
// take a slot in the window. Release pruning already filters foreign names out of
// the candidates; the backup path did not, and an unparsable name sorted as the
// newest, so a stray directory survived while a real dump — the one a rollback
// would restore — was deleted to make room for it.
func TestCleanupKeepsForeignBackupDirectoriesOutOfTheWindow(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	for _, release := range []string{"1", "2", "3"} {
		if _, err := runner.Run(ctx, "mkdir -p "+host.SharedBackupPath(release), deploy.RunOptions{}); err != nil {
			t.Fatalf("seed backup %s: %v", release, err)
		}
	}
	stray := filepath.Join(host.BackupRootPath(), "before-manual-upgrade")
	if _, err := runner.Run(ctx, "mkdir -p "+stray+" && echo dump > "+filepath.Join(stray, "db.sql"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed a foreign backup directory: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{KeepReleases: 2})
	sc.Release = deploy.NewReleaseForTest("4", "abc", "local")
	if err := deploy.CoreCleanup(ctx, sc); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	for _, release := range []string{"2", "3"} {
		if _, err := runner.Run(ctx, "test -d "+host.SharedBackupPath(release), deploy.RunOptions{}); err != nil {
			t.Fatalf("backup %s is inside the window and must survive: a stray name must not consume a slot", release)
		}
	}
	if _, err := runner.Run(ctx, "test ! -e "+host.SharedBackupPath("1"), deploy.RunOptions{}); err != nil {
		t.Fatal("backup 1 is outside the window and must be pruned")
	}
	if _, err := runner.Run(ctx, "test -d "+stray, deploy.RunOptions{}); err != nil {
		t.Fatal("cleanup must not delete a directory govard did not name")
	}
}

// A deploy that never used --db-backup has no backup directory at all, and
// cleanup must not turn that into a failure.
func TestCleanupToleratesAMissingBackupRoot(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{KeepReleases: 2})
	sc.Release = deploy.NewReleaseForTest("1", "abc", "local")
	if err := deploy.CoreCleanup(context.Background(), sc); err != nil {
		t.Fatalf("cleanup without a backup root: %v", err)
	}
}

// `deploy:shared` links a shared entry only when the shared directory has one,
// because "the first deploy of a project legitimately has no shared state yet".
// The verify check demanded all of them unconditionally, so a first deploy of a
// recipe whose defaults name a file the project has not created yet failed AFTER
// activating the release.
func TestVerifyToleratesASharedFileWithNoSharedCopyYet(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	release := verifiedRelease(t, host)

	sc := deploy.StepContextForTest(host, deploy.Options{
		Verify:   true,
		Settings: map[string]any{"shared_files": []string{"app/etc/env.php", "var/.maintenance.ip"}},
	})
	sc.Release = release

	// Nothing in shared/ and nothing in the release: this is a first deploy, not
	// a broken link.
	if err := deploy.CoreVerify(ctx, sc); err != nil {
		t.Fatalf("a shared file the project has not created yet must not fail verification: %v", err)
	}

	// Once shared/ has the file, the release must be serving it: a missing or
	// broken link is exactly what this check exists to catch.
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")
	sc.Release.Verify = deploy.VerifyRecord{}
	if err := deploy.CoreVerify(ctx, sc); err == nil {
		t.Fatal("a shared file that exists in shared/ but not in the release must fail verification")
	}

	// And it passes once the release is linked to it.
	linked := filepath.Join(release.Path, "app/etc/env.php")
	if _, err := host.Runner().Run(ctx, "mkdir -p "+filepath.Dir(linked)+" && ln -sfn "+filepath.Join(host.SharedPath(), "app/etc/env.php")+" "+linked, deploy.RunOptions{}); err != nil {
		t.Fatalf("link the shared file: %v", err)
	}
	sc.Release.Verify = deploy.VerifyRecord{}
	if err := deploy.CoreVerify(ctx, sc); err != nil {
		t.Fatalf("a linked shared file must verify: %v", err)
	}
}

// An in-place activation resets the docroot to the revision and copies the
// configured paths. `git reset --hard` does not touch the gitignored directories
// of the previous deployment, so an empty `sync_paths` ships new code over the old
// vendor/, generated/ and pub/static/ and reports success — the warning has to
// arrive before anything runs.
func TestInPlaceActivationWarnsWhenNothingIsConfiguredToSync(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	docroot := filepath.Join(host.DeployPath, "public_html")
	if err := os.MkdirAll(docroot, 0o755); err != nil {
		t.Fatalf("mkdir docroot: %v", err)
	}
	host.CurrentPath = docroot

	sc := deploy.StepContextForTest(host, deploy.Options{Publish: deploy.PublishInPlace})
	sc.Release = deploy.NewReleaseForTest("1", "abcdef", "main")
	if err := deploy.NoteInPlaceSyncPathsForTest(context.Background(), sc); err != nil {
		t.Fatalf("note: %v", err)
	}
	joined := strings.Join(sc.Notes, "\n")
	if !strings.Contains(joined, "sync_paths") || !strings.Contains(joined, "in place") {
		t.Fatalf("notes = %q, want a warning naming sync_paths and the in-place strategy", joined)
	}

	// Configured: nothing to warn about.
	configured := deploy.StepContextForTest(host, deploy.Options{
		Publish:  deploy.PublishInPlace,
		Settings: map[string]any{"sync_paths": []string{"vendor", "generated"}},
	})
	configured.Release = sc.Release
	if err := deploy.NoteInPlaceSyncPathsForTest(context.Background(), configured); err != nil {
		t.Fatalf("note: %v", err)
	}
	if len(configured.Notes) != 0 {
		t.Fatalf("a configured sync_paths must not warn, got %q", configured.Notes)
	}

	// A symlink activation copies nothing by design: no warning either.
	symlink := deploy.StepContextForTest(host, deploy.Options{Publish: deploy.PublishSymlink})
	symlink.Release = sc.Release
	if err := deploy.NoteInPlaceSyncPathsForTest(context.Background(), symlink); err != nil {
		t.Fatalf("note: %v", err)
	}
	if len(symlink.Notes) != 0 {
		t.Fatalf("a symlink activation must not warn, got %q", symlink.Notes)
	}
}

// A build step is allowed to produce nothing: `generated/` only exists after
// `setup:di:compile`, and `pub/static/adminhtml` only when the admin area was
// deployed. A `sync_paths` entry whose source is absent must therefore be skipped,
// not fail the activation — the shipped default lists paths that some projects
// never build.
func TestInPlaceActivationSkipsPathsTheReleaseDidNotBuild(t *testing.T) {
	origin, revision := seedGitRepo(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	// The release built `vendor` and nothing else.
	if _, err := runner.Run(ctx, "mkdir -p "+host.ReleasePath("2")+"/vendor && echo built > "+host.ReleasePath("2")+"/vendor/autoload.php", deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings:   map[string]any{"sync_paths": []string{"generated", "vendor", "pub/static/adminhtml"}},
	})
	sc.Release = deploy.NewReleaseForTest("2", revision, "main")
	sc.Release.Path = host.ReleasePath("2")
	if err := deploy.CoreActivate(ctx, sc); err != nil {
		t.Fatalf("a path the release did not build must be skipped, not fatal: %v", err)
	}
	if _, err := runner.Run(ctx, "test ! -e "+host.CurrentPath+"/generated", deploy.RunOptions{}); err != nil {
		t.Fatal("a path the release did not build must not appear in the docroot")
	}
	if _, err := runner.Run(ctx, "cat "+host.CurrentPath+"/vendor/autoload.php", deploy.RunOptions{}); err != nil {
		t.Fatalf("the paths it did build must still be published: %v", err)
	}
}

// `deploy:shared` links a shared directory into the release with a *relative*
// symlink, which only resolves at the release's depth. Copying it into a docroot
// replaces the docroot's own directory with a link that points somewhere else — so
// an entry that is shared is not synced at all, and a shared path *inside* a synced
// entry is excluded from it.
func TestInPlaceActivationNeverPublishesASharedLink(t *testing.T) {
	origin, revision := seedGitRepo(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	if _, err := runner.Run(ctx, "git clone -q "+origin+" "+host.CurrentPath, deploy.RunOptions{}); err != nil {
		t.Fatalf("clone docroot: %v", err)
	}
	release := host.ReleasePath("2")
	shared := filepath.Join(host.SharedPath(), "pub", "static", "_cache")
	if _, err := runner.Run(ctx, "mkdir -p "+shared+" && echo shared > "+shared+"/shared.txt", deploy.RunOptions{}); err != nil {
		t.Fatalf("seed shared state: %v", err)
	}
	if _, err := runner.Run(ctx, "mkdir -p "+release+"/pub/static/adminhtml && echo new > "+release+"/pub/static/adminhtml/new.txt"+
		" && ln -s "+shared+" "+release+"/pub/static/_cache", deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}
	// The docroot's own copy of the shared path: a real directory with content that
	// a sync would replace or delete.
	docrootCache := filepath.Join(host.CurrentPath, "pub", "static", "_cache")
	if err := os.MkdirAll(docrootCache, 0o755); err != nil {
		t.Fatalf("mkdir docroot cache: %v", err)
	}
	writeFile(t, filepath.Join(docrootCache, "keep.txt"), "keep\n")

	sc := deploy.StepContextForTest(host, deploy.Options{
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings: map[string]any{
			"shared_dirs": []string{"pub/static/_cache"},
			"sync_paths":  []string{"pub/static"},
		},
	})
	sc.Release = deploy.NewReleaseForTest("2", revision, "main")
	sc.Release.Path = release
	if err := deploy.CoreActivate(ctx, sc); err != nil {
		t.Fatalf("activate: %v", err)
	}

	info, err := os.Lstat(docrootCache)
	if err != nil {
		t.Fatalf("the docroot's shared path must survive: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the release's shared symlink was copied into the docroot, where it resolves elsewhere")
	}
	if _, err := os.Stat(filepath.Join(docrootCache, "keep.txt")); err != nil {
		t.Fatalf("the docroot's own cache content was deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(docrootCache, "shared.txt")); err == nil {
		t.Fatal("the shared target's content was copied into the docroot")
	}
	if _, err := os.Stat(filepath.Join(host.CurrentPath, "pub/static/adminhtml/new.txt")); err != nil {
		t.Fatalf("the rest of the synced entry must still land: %v", err)
	}

	// An entry that *is* the shared path is skipped outright.
	sc2 := deploy.StepContextForTest(host, deploy.Options{
		Publish:    deploy.PublishInPlace,
		Repository: origin,
		Revision:   revision,
		Branch:     "main",
		Settings: map[string]any{
			"shared_dirs": []string{"pub/static/_cache"},
			"sync_paths":  []string{"pub/static/_cache"},
		},
	})
	sc2.Release = deploy.NewReleaseForTest("2", revision, "main")
	sc2.Release.Path = release
	if err := deploy.CoreActivate(ctx, sc2); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(docrootCache, "keep.txt")); err != nil {
		t.Fatalf("a shared entry must not be synced at all: %v", err)
	}
	if _, err := os.Stat(filepath.Join(docrootCache, "shared.txt")); err == nil {
		t.Fatal("a shared entry must not be synced at all")
	}
}

// The recipe's default has to be the list the strategy needs: paths the release
// *built*, and none of them shared (a shared path is a relative symlink, and
// syncing it into a docroot at another depth points it elsewhere).
func TestMagento2RecipeShipsTheInPlaceSyncPaths(t *testing.T) {
	recipe := magento2.DeployRecipe()
	raw, ok := recipe.Defaults["sync_paths"].([]string)
	if !ok || len(raw) == 0 {
		t.Fatalf("sync_paths default = %#v, want the paths an in-place activation must publish", recipe.Defaults["sync_paths"])
	}

	shared := map[string]bool{}
	for _, entry := range defaultsStringList(recipe, "shared_dirs") {
		shared[entry] = true
	}
	for _, entry := range defaultsStringList(recipe, "shared_files") {
		shared[entry] = true
	}
	for _, entry := range raw {
		if shared[entry] {
			t.Errorf("sync_paths default lists %q, which the release links from shared/", entry)
		}
		for candidate := range shared {
			if strings.HasPrefix(candidate, entry+"/") {
				t.Errorf("sync_paths default lists %q, which contains the shared path %q", entry, candidate)
			}
		}
	}
}

// defaultsStringList reads a list default from a recipe, whichever list shape it
// was written in.
func defaultsStringList(recipe deploy.Recipe, key string) []string {
	switch typed := recipe.Defaults[key].(type) {
	case []string:
		return typed
	case []any:
		rendered := make([]string, 0, len(typed))
		for _, entry := range typed {
			if text, ok := entry.(string); ok {
				rendered = append(rendered, text)
			}
		}
		return rendered
	default:
		return nil
	}
}

// The preflight says which `sync_paths` entries cannot travel before anything runs:
// a shared path is a relative symlink in the release, and a shared path inside an
// entry is excluded rather than deleted.
func TestInPlacePreflightNamesTheSharedPathsInSyncPaths(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	docroot := filepath.Join(host.DeployPath, "public_html")
	if err := os.MkdirAll(docroot, 0o755); err != nil {
		t.Fatalf("mkdir docroot: %v", err)
	}
	host.CurrentPath = docroot

	sc := deploy.StepContextForTest(host, deploy.Options{
		Publish: deploy.PublishInPlace,
		Settings: map[string]any{
			"shared_dirs": []string{"pub/static/_cache"},
			"sync_paths":  []string{"pub/static/_cache", "pub/static", "vendor"},
		},
	})
	sc.Release = deploy.NewReleaseForTest("1", "abcdef", "main")
	if err := deploy.NoteInPlaceSyncPathsForTest(context.Background(), sc); err != nil {
		t.Fatalf("note: %v", err)
	}

	joined := strings.Join(sc.Notes, "\n")
	if !strings.Contains(joined, "pub/static/_cache, which the release links from shared/") {
		t.Fatalf("notes = %q, want the shared entry named as not copied", joined)
	}
	if !strings.Contains(joined, "pub/static, which contains the shared path pub/static/_cache") {
		t.Fatalf("notes = %q, want the shared child named as excluded", joined)
	}
	if strings.Contains(joined, "vendor") {
		t.Fatalf("notes = %q, want no warning for a plain built path", joined)
	}
}

// A redirect followed to a 200 is not proof that the release is serving: the
// engine's own notes record a deploy where every request was redirected to
// Magento's installer while the check passed. By default the first response is
// the answer.
func TestVerifyURLRefusesARedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setup/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/setup/", http.StatusFound)
	}))
	defer server.Close()

	err := deploy.VerifyURL(context.Background(), server.URL, 5*time.Second, deploy.VerifyPolicy{})
	if err == nil {
		t.Fatal("a 302 must fail the check")
	}
	for _, want := range []string{"302", "/setup/", "follow_redirects"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error must name %q, got: %v", want, err)
		}
	}
}

// Redirects are followable on request — http to https is a real layout — but only
// within the host that serves the site, and a recipe's never-healthy landing path
// still fails.
func TestVerifyURLFollowsSameHostRedirectsWhenAllowed(t *testing.T) {
	sameHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/home" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/home", http.StatusMovedPermanently)
	}))
	defer sameHost.Close()

	if err := deploy.VerifyURL(context.Background(), sameHost.URL, 5*time.Second, deploy.VerifyPolicy{FollowRedirects: true}); err != nil {
		t.Fatalf("a same-host redirect must be followable with follow_redirects: %v", err)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()
	offHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer offHost.Close()

	if err := deploy.VerifyURL(context.Background(), offHost.URL, 5*time.Second, deploy.VerifyPolicy{FollowRedirects: true}); err == nil {
		t.Fatal("a redirect off the verify URL's host must fail")
	}
}

func TestVerifyURLRejectsARecipeDeclaredLandingPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setup/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/setup/", http.StatusFound)
	}))
	defer server.Close()

	policy := deploy.VerifyPolicy{FollowRedirects: true, RejectPaths: []string{"/setup/"}}
	if err := deploy.VerifyURL(context.Background(), server.URL, 5*time.Second, policy); err == nil {
		t.Fatal("landing on a recipe's never-healthy path must fail even when following")
	}
}

// In place the release directory and the served docroot are different trees, so a
// shared file linked into the release while missing from the docroot is a broken
// site — and the check has to look where the site reads.
func TestVerifySharedFileIsCheckedInTheDocrootInPlace(t *testing.T) {
	root := t.TempDir()
	host := deploy.HostForTest(root, deploy.LocalRunner{})

	release := inPlaceReleaseAtTheDocrootRevision(t, host)
	if err := os.MkdirAll(filepath.Join(release.Path, "app", "etc"), 0o755); err != nil {
		t.Fatalf("mkdir release tree: %v", err)
	}
	shared := filepath.Join(host.SharedPath(), "app", "etc", "env.php")
	if err := os.MkdirAll(filepath.Dir(shared), 0o755); err != nil {
		t.Fatalf("mkdir shared tree: %v", err)
	}
	if err := os.WriteFile(shared, []byte("config"), 0o600); err != nil {
		t.Fatalf("write the shared file: %v", err)
	}
	// Linked and readable in the release …
	if err := os.Symlink(shared, filepath.Join(release.Path, "app", "etc", "env.php")); err != nil {
		t.Fatalf("link the shared file into the release: %v", err)
	}
	// … and absent from the docroot, which is the state a broken activation leaves.

	sc := deploy.StepContextForTest(host, deploy.Options{
		Verify:   true,
		Settings: map[string]any{"shared_files": []string{"app/etc/env.php"}},
	})
	sc.Release = release

	err := deploy.CoreVerify(context.Background(), sc)
	if err == nil {
		t.Fatal("a shared file missing from the docroot must fail verification")
	}
	if !strings.Contains(err.Error(), "shared:app/etc/env.php") {
		t.Fatalf("the failure must name the shared file, got: %v", err)
	}
}

// `deploy:shared` is what makes a release read the state that outlives it, and a
// shared *directory* it failed to link is invisible to every other check: the
// files check looks at the served path, and the release keeps a real directory
// that looks healthy from the outside. Measured on three live targets
// (2026-09-25), where `ln -sfn` had answered a real `pub/media` by creating the
// link inside it (`pub/media/media`) and exiting 0, so the deploy verified green
// while the release read its own media tree.
func TestVerifySharedDirectoryMustBeLinkedInTheRelease(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := verifiedRelease(t, host)

	shared := filepath.Join(host.SharedPath(), "pub", "media")
	if err := os.MkdirAll(filepath.Join(shared, "catalog"), 0o755); err != nil {
		t.Fatalf("mkdir the shared directory: %v", err)
	}
	// The release's own copy, which is what the broken link left behind.
	if err := os.MkdirAll(filepath.Join(release.Path, "pub", "media"), 0o755); err != nil {
		t.Fatalf("mkdir the release's directory: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Verify:   true,
		Settings: map[string]any{"shared_dirs": []string{"pub/media"}},
	})
	sc.Release = release

	err := deploy.CoreVerify(context.Background(), sc)
	if err == nil {
		t.Fatal("a shared directory the release does not link must fail verification")
	}
	if !strings.Contains(err.Error(), "shared:pub/media") {
		t.Fatalf("the failure must name the shared directory, got: %v", err)
	}
}

// A link that resolves to nothing is not the shared directory either: the release
// reads an empty path, so the check has to prove both halves — that the entry is a
// link, and that it resolves. `test -L` alone passes a dangling link.
func TestVerifyRejectsASharedDirectoryLinkedToNothing(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := verifiedRelease(t, host)

	shared := filepath.Join(host.SharedPath(), "pub", "media")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatalf("mkdir the shared directory: %v", err)
	}
	// What a moved deploy path, a hand-made link or a partial cleanup leaves: the
	// link exists, and it points at nothing.
	if err := os.MkdirAll(filepath.Join(release.Path, "pub"), 0o755); err != nil {
		t.Fatalf("mkdir the release's pub: %v", err)
	}
	if err := os.Symlink(filepath.Join(host.SharedPath(), "pub", "gone"), filepath.Join(release.Path, "pub", "media")); err != nil {
		t.Fatalf("link the shared directory somewhere that does not exist: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Verify:   true,
		Settings: map[string]any{"shared_dirs": []string{"pub/media"}},
	})
	sc.Release = release

	err := deploy.CoreVerify(context.Background(), sc)
	if err == nil {
		t.Fatal("a shared directory linked to nothing must fail verification")
	}
	if !strings.Contains(err.Error(), "shared:pub/media") {
		t.Fatalf("the failure must name the shared directory, got: %v", err)
	}
}

// The other half of the same rule, and the reason the check reads the release
// rather than the served path: an in-place docroot that owns its own copy of a
// shared directory keeps it, and that is a correct target, not a broken one.
func TestVerifyAcceptsASharedDirectoryLinkedInTheRelease(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := inPlaceReleaseAtTheDocrootRevision(t, host)

	shared := filepath.Join(host.SharedPath(), "pub", "media")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatalf("mkdir the shared directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(release.Path, "pub"), 0o755); err != nil {
		t.Fatalf("mkdir the release's pub: %v", err)
	}
	if err := os.Symlink(shared, filepath.Join(release.Path, "pub", "media")); err != nil {
		t.Fatalf("link the shared directory into the release: %v", err)
	}
	// The docroot keeps the directory it owns: `ensureInPlaceShared` leaves a real
	// directory alone rather than deleting data, and verification must not read
	// that as a missing link.
	if err := os.MkdirAll(filepath.Join(host.CurrentPath, "pub", "media"), 0o755); err != nil {
		t.Fatalf("mkdir the docroot's directory: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Verify:   true,
		Settings: map[string]any{"shared_dirs": []string{"pub/media"}},
	})
	sc.Release = release

	if err := deploy.CoreVerify(context.Background(), sc); err != nil {
		t.Fatalf("a linked shared directory must verify: %v", err)
	}
}

// A nested shared directory is read through its parent's link, so it is not a
// link of its own in the release, and that is correct rather than broken.
func TestVerifyAcceptsANestedSharedDirectoryReachedThroughItsParent(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := verifiedRelease(t, host)

	shared := filepath.Join(host.SharedPath(), "pub", "media")
	if err := os.MkdirAll(filepath.Join(shared, "catalog"), 0o755); err != nil {
		t.Fatalf("mkdir the shared directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(release.Path, "pub"), 0o755); err != nil {
		t.Fatalf("mkdir the release's pub: %v", err)
	}
	if err := os.Symlink(shared, filepath.Join(release.Path, "pub", "media")); err != nil {
		t.Fatalf("link the shared directory into the release: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Verify:   true,
		Settings: map[string]any{"shared_dirs": []string{"pub/media", "pub/media/catalog"}},
	})
	sc.Release = release

	if err := deploy.CoreVerify(context.Background(), sc); err != nil {
		t.Fatalf("a nested shared directory reached through its parent must verify: %v", err)
	}
}

// A deploy path that is itself a symlink (`~` is a built-in discovered layout)
// makes `readlink -f current` and the recorded release path two spellings of one
// directory. Comparing them as strings failed verification *after* the site had
// switched, and the verify stage keeps the lock, so the deploy stayed failed with
// the release live.
func TestVerifySymlinkAcceptsASymlinkedDeployPath(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink the deploy path: %v", err)
	}
	host := deploy.HostForTest(link, deploy.LocalRunner{})
	release := verifiedRelease(t, host)

	sc := deploy.StepContextForTest(host, deploy.Options{Verify: true})
	sc.Release = release
	if err := deploy.CoreVerify(context.Background(), sc); err != nil {
		t.Fatalf("a symlinked deploy path must verify: %v", err)
	}
}

// inPlaceReleaseAtTheDocrootRevision builds an in-place target whose docroot is a
// git checkout at the revision the release records, so verification reaches the
// content checks instead of stopping at the revision comparison.
func inPlaceReleaseAtTheDocrootRevision(t *testing.T, host deploy.Host) *deploy.Release {
	t.Helper()
	docroot := host.CurrentPath
	if err := os.MkdirAll(docroot, 0o755); err != nil {
		t.Fatalf("mkdir docroot: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		command := exec.Command("git", append([]string{"-C", docroot}, args...)...)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	head, err := exec.Command("git", "-C", docroot, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("read the docroot revision: %v", err)
	}
	release := deploy.NewReleaseForTest("1", strings.TrimSpace(string(head)), "main")
	release.Path = host.ReleasePath("1")
	release.Publish.Strategy = deploy.PublishInPlace
	if err := os.MkdirAll(release.Path, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	return release
}

// The in-place activation copies each sync path into the docroot. Comparing one
// marker file with the copy the activation just made from it proves nothing about
// the tree; a dry run of the same copy proves the whole thing, and catches a hook
// or a process that rewrote the docroot after the activation — including a stale
// static-content version file, which is the scenario the recipe's removed
// artifact check used to cover as a tautology.
func TestVerifyInPlaceCatchesADocrootThatDiffersFromTheRelease(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := inPlaceReleaseAtTheDocrootRevision(t, host)
	settings := map[string]any{"sync_paths": []string{"generated", "pub/static"}}

	write := func(root, relative, content string) {
		t.Helper()
		target := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", target, err)
		}
	}
	// What the activation leaves behind: both paths built in the release and
	// copied into the docroot.
	write(release.Path, "generated/code.php", "built\n")
	write(release.Path, "pub/static/deployed_version.txt", "abc123\n")
	write(host.CurrentPath, "generated/code.php", "built\n")
	write(host.CurrentPath, "pub/static/deployed_version.txt", "abc123\n")

	verify := func() error {
		sc := deploy.StepContextForTest(host, deploy.Options{Verify: true, Settings: settings})
		sc.Release = release
		return deploy.CoreVerify(context.Background(), sc)
	}
	if err := verify(); err != nil {
		t.Fatalf("a docroot that matches the release must verify: %v", err)
	}

	// A hook, or a process the release started, rewrote a file afterwards.
	write(host.CurrentPath, "generated/code.php", "rewritten\n")
	err := verify()
	if err == nil {
		t.Fatal("a docroot that differs from the release must fail verification")
	}
	if !strings.Contains(err.Error(), "sync:generated") || !strings.Contains(err.Error(), "code.php") {
		t.Fatalf("the failure must name the path and the file, got: %v", err)
	}

	// Restore it and leave only a stale static content version behind, which is
	// the state the removed artifact check existed for.
	write(host.CurrentPath, "generated/code.php", "built\n")
	write(host.CurrentPath, "pub/static/deployed_version.txt", "stale\n")
	err = verify()
	if err == nil {
		t.Fatal("a stale static content version in the docroot must fail verification")
	}
	if !strings.Contains(err.Error(), "deployed_version.txt") {
		t.Fatalf("the failure must name the version file, got: %v", err)
	}
}

// A path the running application wrote into the served tree is not a failed
// deploy. The activation's own `--delete` removes it — exactly what the dry run
// reports — and the steps that run in the docroot while the window is open
// (cache flush, cron, a framework that generates classes on demand) create them
// as a matter of course, which is why `generated` is a sync path in the first
// place. Failing on one fails a release that is healthy and already serving, with
// the lock held; it is reported instead, so an operator sees it without losing the
// deploy.
func TestVerifyInPlaceToleratesAFileTheApplicationCreated(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := inPlaceReleaseAtTheDocrootRevision(t, host)
	settings := map[string]any{"sync_paths": []string{"generated"}}

	writeFile(t, filepath.Join(release.Path, "generated", "code.php"), "built\n")
	writeFile(t, filepath.Join(host.CurrentPath, "generated", "code.php"), "built\n")
	// Runtime state: what the application produced after the activation copied
	// the tree, and what the next activation would clean up.
	writeFile(t, filepath.Join(host.CurrentPath, "generated", "Extra.php"), "generated at runtime\n")

	sc := deploy.StepContextForTest(host, deploy.Options{Verify: true, Settings: settings})
	sc.Release = release
	if err := deploy.CoreVerify(context.Background(), sc); err != nil {
		t.Fatalf("a file the running application created must not fail verification: %v", err)
	}

	reported := ""
	for _, check := range sc.Release.Verify.Checks {
		reported += check.ID + ": " + check.Detail + " | "
	}
	if !strings.Contains(reported, "Extra.php") {
		t.Fatalf("the report must name the path the next activation would remove, got %q", reported)
	}

	// The tolerance is only for files the release does not have. A release file the
	// docroot has changed is still the failure the comparison exists for.
	writeFile(t, filepath.Join(host.CurrentPath, "generated", "code.php"), "rewritten\n")
	if err := deploy.CoreVerify(context.Background(), sc); err == nil {
		t.Fatal("a changed release file must still fail verification")
	}
}
