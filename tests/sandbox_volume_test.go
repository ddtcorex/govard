package tests

import (
	"context"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// existingVolume makes the fake report that the project's database volume
// survived an earlier `down`, stamped with one series.
func existingVolume(fake *fakeSandboxRuntime, project, series string) {
	fake.answers["volume ls --quiet --filter name="] = deploy.SandboxDBVolumeName(project) + "\n"
	fake.answers["volume inspect"] = series + "\n"
}

func TestSandboxDBVolumeNameDerivesFromProject(t *testing.T) {
	if got := deploy.SandboxDBVolumeName("Seed_Shop"); got != "govard-sandbox-seed_shop-db" {
		t.Fatalf("name = %q", got)
	}
	if deploy.SandboxDBVolumeName("a") == deploy.SandboxDBVolumeName("b") {
		t.Fatal("two projects must not share a volume")
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
	name := deploy.SandboxDBVolumeName(request.ProjectName)
	create := fake.call("volume create")
	if create == nil {
		t.Fatalf("the volume must be created before the container: %v", fake.calls)
	}
	joined := strings.Join(create, " ")
	for _, want := range []string{
		"--label " + deploy.SandboxDBVolumeLabel + "=" + request.ProjectName,
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
	request.Purge = true
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(purge.run), request); err != nil {
		t.Fatal(err)
	}
	if !purge.has("volume ls --quiet --filter label=" + deploy.SandboxDBVolumeLabel + "=volume-shop") {
		t.Fatalf("--purge must look the database volume up by its label: %v", purge.calls)
	}
}

func TestChangedSeriesOnAnExistingVolumeIsRefused(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.DBEngine, request.DBVersion = "mariadb", "10.11"
	fake := freshSandboxFake()
	// The volume survived an earlier `down`, stamped with another series.
	existingVolume(fake, request.ProjectName, "mariadb:10.6")
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
	existingVolume(fake, request.ProjectName, "mariadb:10.6")
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatal(err)
	}
	if !fake.has("type=volume,source=" + deploy.SandboxDBVolumeName(request.ProjectName)) {
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
