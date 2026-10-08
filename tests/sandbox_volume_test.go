package tests

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// existingVolume makes the fake report that the project's database volume
// survived an earlier `down`, stamped with one series.
func existingVolume(fake *fakeSandboxRuntime, request deploy.SandboxRequest, series string) {
	fake.answers["volume ls --quiet --filter name="] = volumeFor(request) + "\n"
	fake.answers["volume inspect"] = series + "\n"
}

// volumeFor is the volume a request's sandbox keeps its database in.
func volumeFor(request deploy.SandboxRequest) string {
	return deploy.SandboxDBVolumeName(deploy.SandboxContainerName(request.ProjectName, request.ProjectRoot))
}

func TestSandboxDBVolumeNameFollowsTheContainerIdentity(t *testing.T) {
	a := deploy.SandboxDBVolumeName(deploy.SandboxContainerName("shop", "/work/a"))
	b := deploy.SandboxDBVolumeName(deploy.SandboxContainerName("shop", "/work/b"))
	if a == b {
		t.Fatal("two checkouts of one project must not share a database volume")
	}
	if deploy.SandboxDBVolumeName(deploy.SandboxContainerName("Shop_A", "/work/a")) ==
		deploy.SandboxDBVolumeName(deploy.SandboxContainerName("shop-a", "/work/b")) {
		t.Fatal("names the slug folds together must not share a volume across checkouts")
	}
	if !strings.HasPrefix(a, "govard-shop-sandbox-") || !strings.HasSuffix(a, "-db") {
		t.Fatalf("name = %q", a)
	}
}

func TestFullProfileMountsTheDBVolumeAndCreatesItLabelled(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.DBEngine, request.DBVersion = "mariadb", "10.6"
	fake := freshSandboxFake()
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	name := volumeFor(request)
	create := fake.call("volume create")
	if create == nil {
		t.Fatalf("the volume must be created before the container: %v", fake.calls)
	}
	joined := strings.Join(create, " ")
	for _, want := range []string{
		"--label " + deploy.SandboxDBVolumeLabel + "=" + deploy.SandboxContainerName(request.ProjectName, request.ProjectRoot),
		"--label govard.sandbox.db=mariadb:10.6",
		name,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("volume create %q is missing %q", joined, want)
		}
	}
	if !fake.has("--mount type=volume,source=" + name + ",target=/var/lib/mysql") {
		t.Fatalf("the container must mount the volume over the data directory: %v", fake.calls)
	}
}

func TestProfilesWithoutADatabaseMountNoVolume(t *testing.T) {
	for _, profile := range []string{deploy.SandboxProfileBasic, deploy.SandboxProfilePHP} {
		origin, _ := seedGitRepo(t)
		request := seedSandboxUpRequest(t, t.TempDir(), origin)
		request.Profile = profile
		fake := freshSandboxFake()
		if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
			t.Fatalf("%s: %v", profile, err)
		}
		if fake.has("volume create") || fake.has("type=volume") {
			t.Errorf("%s has no database, so it must not create or mount a volume: %v", profile, fake.calls)
		}
	}
}

func TestDownKeepsTheVolumeAndPurgeRemovesIt(t *testing.T) {
	root := t.TempDir()
	request := deploy.SandboxRequest{ProjectRoot: root, ProjectName: "volume-shop"}

	keep := sandboxFake()
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(keep.run), request); err != nil {
		t.Fatal(err)
	}
	if keep.has("volume ls --quiet --filter label=" + deploy.SandboxDBVolumeLabel) {
		t.Fatal("a plain down must keep the database volume")
	}

	purge := sandboxFake()
	volume := deploy.SandboxDBVolumeName(deploy.SandboxContainerName("volume-shop", root))
	purge.answers["volume ls --quiet --filter label="+deploy.SandboxDBVolumeLabel] = volume + "\n"
	request.Purge = true
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(purge.run), request); err != nil {
		t.Fatal(err)
	}
	if !purge.has("volume ls --quiet --filter label=" + deploy.SandboxDBVolumeLabel + "=" + deploy.SandboxContainerName("volume-shop", root)) {
		t.Fatalf("--purge must look the database volume up by its label: %v", purge.calls)
	}
	if !purge.has("volume rm " + volume) {
		t.Fatalf("--purge must remove the volume it found, %s: %v", volume, purge.calls)
	}
}

func TestChangedSeriesOnAnExistingVolumeIsRefused(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.DBEngine, request.DBVersion = "mariadb", "10.11"
	fake := freshSandboxFake()
	// The volume survived an earlier `down`, stamped with another series.
	existingVolume(fake, request, "mariadb:10.6")
	_, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request)
	if err == nil {
		t.Fatal("a data directory written by 10.6 must not be opened by 10.11")
	}
	for _, want := range []string{"mariadb:10.6", "mariadb:10.11", "--purge"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name %q", err, want)
		}
	}
	if fake.has("run --detach") {
		t.Fatal("the container must not start on a refused volume")
	}
	if fake.has("build") {
		t.Fatal("a refused volume must be reported before the image is built, not after minutes of building")
	}
}

func TestSameSeriesOnAnExistingVolumeIsReused(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.DBEngine, request.DBVersion = "mariadb", "10.6"
	fake := freshSandboxFake()
	existingVolume(fake, request, "mariadb:10.6")
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if !fake.has("type=volume,source=" + volumeFor(request)) {
		t.Fatalf("the existing volume must be mounted: %v", fake.calls)
	}
}

func TestCheckSandboxDBVolumeSeries(t *testing.T) {
	for _, tc := range []struct {
		volume, requested string
		wantErr           bool
	}{
		{"mariadb:10.6", "mariadb:10.6", false},
		{"", "", false},
		{"mariadb:10.6", "mariadb:10.11", true},
		{"mariadb:10.6", "", true},
		{"", "mariadb:10.6", true},
	} {
		err := deploy.CheckSandboxDBVolumeSeries(tc.volume, tc.requested)
		if (err != nil) != tc.wantErr {
			t.Errorf("(%q,%q) err = %v, wantErr %v", tc.volume, tc.requested, err, tc.wantErr)
		}
	}
}

// reusedFullSandbox makes the fake describe a running full-profile container
// that already exists, which is what a second `sandbox up` meets.
func reusedFullSandbox() *fakeSandboxRuntime {
	fake := sandboxFake().containerProfile(deploy.SandboxProfileFull).containerPHP("")
	fake.labels["govard.sandbox.db"] = "\n"
	fake.answers["image inspect"] = "sha256:abc\n"
	fake.answers["22/tcp"] = "127.0.0.1:2222\n"
	fake.answers["mysqladmin ping"] = "mysqld is alive\n"
	fake.answers["command -v mariadb-dump"] = "/usr/bin/mariadb-dump\n"
	fake.answers["command -v mysql"] = "/usr/bin/mysql\n"
	return fake
}

func seededMediaRequest(t *testing.T) deploy.SandboxRequest {
	t.Helper()
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.SeedAppContainer = "seed-shop-php-1"
	request.SeedMediaSource = "/var/www/html/pub/media"
	request.SeedMediaTarget = "/home/deployer/media-seed"
	return request
}

func TestNewContainerOnAPopulatedVolumeSkipsTheDatabaseSeedButSeedsFiles(t *testing.T) {
	request := seededMediaRequest(t)
	var out bytes.Buffer
	request.Out = &out
	fake := freshSandboxFake()
	fake.answers["information_schema.tables"] = "371\n"
	existingVolume(fake, request, "")
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if fake.has("command -v mariadb-dump") {
		t.Fatalf("a populated volume must not be re-seeded: %v", fake.calls)
	}
	if !fake.has("tar -C /home/deployer/media-seed -xf -") {
		t.Fatalf("a new container still gets the files seed: %v", fake.calls)
	}
	if note := out.String(); !strings.Contains(note, "database seed skipped") || !strings.Contains(note, "--reseed") {
		t.Fatalf("want one note naming --reseed, got %q", note)
	}
}

func TestEmptyVolumeSeedsTheDatabase(t *testing.T) {
	request := seededMediaRequest(t)
	fake := freshSandboxFake()
	fake.answers["information_schema.tables"] = "0\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if !fake.has("command -v mariadb-dump") {
		t.Fatalf("an empty database must be seeded: %v", fake.calls)
	}
}

func TestDatabaseProbeAsksAboutTheApplicationSchemaOnly(t *testing.T) {
	request := seededMediaRequest(t)
	fake := freshSandboxFake()
	// Other schemas hold tables, the application database holds none: the probe
	// asks about the application schema only, and its answer is zero.
	fake.answers["information_schema.tables"] = "0\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if !fake.has("table_schema='" + request.SeedDBName + "'") {
		t.Fatalf("the probe must name the application database %q: %v", request.SeedDBName, fake.calls)
	}
	if !fake.has("command -v mariadb-dump") {
		t.Fatalf("a missing application database must be seeded: %v", fake.calls)
	}
}

func TestReseedForcesBothSeedsOnAReusedContainer(t *testing.T) {
	request := seededMediaRequest(t)
	request.Reseed = true
	fake := reusedFullSandbox()
	fake.answers["information_schema.tables"] = "371\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if !fake.has("command -v mariadb-dump") || !fake.has("tar -C /home/deployer/media-seed -xf -") {
		t.Fatalf("--reseed must refresh the database and the files: %v", fake.calls)
	}
}

func TestReusedContainerWithoutReseedSeedsNothing(t *testing.T) {
	request := seededMediaRequest(t)
	fake := reusedFullSandbox()
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if fake.has("command -v mariadb-dump") || fake.has("tar -C /home/deployer/media-seed") {
		t.Fatalf("a reused container keeps what it has: %v", fake.calls)
	}
}

func TestReseedNeedsASeedToRun(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.Reseed, request.NoSeed = true, true
	_, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(freshSandboxFake().run), deploy.LocalRunner{}, request)
	if err == nil || !strings.Contains(err.Error(), "--reseed") || !strings.Contains(err.Error(), "--no-seed") {
		t.Fatalf("err = %v, want a usage error naming both flags", err)
	}
	request = seedSandboxUpRequest(t, t.TempDir(), origin)
	request.Reseed, request.SeedOrigin = true, ""
	_, err = deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(freshSandboxFake().run), deploy.LocalRunner{}, request)
	if err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("err = %v, want a refusal that names the missing origin", err)
	}
}

// A framework that seeds neither media nor an env file never creates the deploy
// tree, so the ownership hand-over must create it first instead of failing on a
// directory that does not exist.
func TestSeedCreatesTheDeployTreeBeforeHandingItOver(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	fake := freshSandboxFake()
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	mkdirAt, chownAt := -1, -1
	for i, call := range fake.calls {
		line := strings.Join(call, " ")
		if strings.Contains(line, "mkdir -p /home/deployer/.deployer") && mkdirAt < 0 {
			mkdirAt = i
		}
		if strings.Contains(line, "chown -R") && strings.Contains(line, "/home/deployer/.deployer") {
			chownAt = i
		}
	}
	if chownAt < 0 || mkdirAt < 0 || mkdirAt > chownAt {
		t.Fatalf("the deploy tree must be created before the chown (mkdir at %d, chown at %d): %v", mkdirAt, chownAt, fake.calls)
	}
}

// A seed that died midway must not be forgiven by the next plain `up`: the
// container exists then, and a reused container normally seeds nothing.
func TestHalfFailedSeedIsRetriedByThePlainUpThatFollows(t *testing.T) {
	request := seededMediaRequest(t)

	failing := freshSandboxFake()
	failing.fail["chown -R 1000:1000"] = "chown: cannot access"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(failing.run), deploy.LocalRunner{}, request); err == nil {
		t.Fatal("the seed failure must fail the first up")
	}

	reused := reusedFullSandbox()
	reused.answers["information_schema.tables"] = "371\n"
	state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(reused.run), deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reused.has("tar -C /home/deployer/media-seed -xf -") {
		t.Fatalf("the plain up after a failed seed must finish the seed: %v", reused.calls)
	}
	if state.DerivedFrom == nil {
		t.Fatal("the completed seed must be recorded")
	}

	// Once the seed completed, the following plain up is a plain reuse again.
	again := reusedFullSandbox()
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(again.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if again.has("tar -C /home/deployer/media-seed") {
		t.Fatalf("a completed seed must not repeat: %v", again.calls)
	}
}
