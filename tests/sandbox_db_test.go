package tests

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestResolveSandboxDBDefault(t *testing.T) {
	cases := []struct {
		engine, version, want string
		wantErr               string
	}{
		{"mariadb", "10.6", "mariadb:10.6", ""},
		{"mariadb", "11.4", "mariadb:11.4", ""},
		{"MariaDB", "10.11.4", "mariadb:10.11", ""},
		{"mariadb", "", "", ""},
		{"none", "", "", ""},
		{"", "", "", ""},
		{"mysql", "8.0", "", "can only provide MariaDB"},
		{"mariadb", "latest", "", "not a series"},
		{"mariadb", "5.5", "", "available: 10.6, 10.11, 11.4, 11.8"},
		{"mariadb", "10.5", "", "installable form"},
	}
	for _, tc := range cases {
		got, err := deploy.ResolveSandboxDBDefault(tc.engine, tc.version)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("(%q,%q) err = %v, want %q", tc.engine, tc.version, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("(%q,%q) = %q, %v; want %q", tc.engine, tc.version, got, err, tc.want)
		}
	}
}

func TestParseSandboxDBIsAClosedSet(t *testing.T) {
	for in, want := range map[string]string{"": "", "default": "", "mariadb:10.6": "mariadb:10.6", " MariaDB:11.4 ": "mariadb:11.4"} {
		if got, err := deploy.ParseSandboxDB(in); err != nil || got != want {
			t.Errorf("ParseSandboxDB(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"mariadb", "mariadb:10", "mariadb:10.6; rm -rf /", "mariadb:10.6\nRUN x", "postgres:16", "mysql:8.0"} {
		if _, err := deploy.ParseSandboxDB(in); err == nil {
			t.Errorf("ParseSandboxDB(%q) accepted", in)
		}
	}
}

func TestSandboxDockerfileInstallsTheRequestedMariaDB(t *testing.T) {
	df, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileFull, DB: "mariadb:10.6"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"dlm.mariadb.com/repo/mariadb-server/10.6/repo/ubuntu jammy main",
		"signed-by=/usr/share/keyrings/mariadb-release.asc",
		"Pin: origin dlm.mariadb.com",
		"'mariadb-server'",
		"Ver 10\\.6[.-]",
	} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile lacks %q", want)
		}
	}
}

func TestSandboxDockerfileDefaultDBIsUnchangedAndSeriesChangesTheTag(t *testing.T) {
	base := deploy.SandboxSpec{Project: "p", Profile: deploy.SandboxProfileFull}
	df, _ := deploy.SandboxDockerfile(base)
	if strings.Contains(df, "mariadb.com") {
		t.Fatal("the default database must not add the MariaDB repository")
	}
	tags := map[string]bool{}
	for _, db := range []string{"", "mariadb:10.6", "mariadb:10.11"} {
		spec := base
		spec.DB = db
		tag, err := deploy.SandboxImageTag(spec)
		if err != nil {
			t.Fatal(err)
		}
		tags[tag] = true
	}
	if len(tags) != 3 {
		t.Fatalf("each database series must be its own image, got %v", tags)
	}
	// Profiles without a database ignore the request.
	php := deploy.SandboxSpec{Project: "p", Profile: deploy.SandboxProfilePHP}
	a, _ := deploy.SandboxImageTag(php)
	php.DB = "mariadb:10.6"
	b, _ := deploy.SandboxImageTag(php)
	if a != b {
		t.Fatal("the php profile has no database; the tag must not change")
	}
}

func TestSandboxUpRefusesAMySQLStackOnTheFullProfile(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.DBEngine, request.DBVersion = "mysql", "8.0"
	runtime := deploy.NewDockerCLIForTest(freshSandboxFake().run)
	_, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, request)
	if err == nil || !strings.Contains(err.Error(), "mysql 8.0") {
		t.Fatalf("err = %v, want a refusal naming mysql 8.0", err)
	}
	// An explicit --db is the escape hatch, and the php profile never asks.
	request.DB = "mariadb:10.6"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(freshSandboxFake().run), deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("--db override: %v", err)
	}
}

func TestSandboxUpReportsTheInstalledDatabase(t *testing.T) {
	origin, _ := seedGitRepo(t)
	request := seedSandboxUpRequest(t, t.TempDir(), origin)
	request.DBEngine, request.DBVersion = "mariadb", "10.6"
	fake := freshSandboxFake()
	fake.answers["mariadbd --version"] = "mariadbd  Ver 10.6.25-MariaDB for debian-linux-gnu\n"
	state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if state.DB != "mariadb:10.6" || !strings.Contains(state.DBServer, "Ver 10.6.25") {
		t.Fatalf("state db = %q server = %q", state.DB, state.DBServer)
	}
	if !fake.has("govard.sandbox.db=mariadb:10.6") {
		t.Fatalf("the container must record its database series: %v", fake.calls)
	}
}

func TestSandboxUpSkipsTheSeedWhenTheProfileHasNoDatabase(t *testing.T) {
	for _, profile := range []string{deploy.SandboxProfileBasic, deploy.SandboxProfilePHP} {
		origin, _ := seedGitRepo(t)
		request := seedSandboxUpRequest(t, t.TempDir(), origin)
		request.Profile = profile
		var out bytes.Buffer
		request.Out = &out
		fake := freshSandboxFake()
		state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request)
		if err != nil {
			t.Fatalf("%s: a profile with no database must not fail the seed: %v", profile, err)
		}
		if fake.has("mysqladmin") || fake.has("mariadb-dump") || state.DerivedFrom != nil {
			t.Errorf("%s: seeding must not be attempted", profile)
		}
		note := out.String()
		if strings.Count(note, "skipping the database seed") != 1 || !strings.Contains(note, profile) || !strings.Contains(note, "--profile full") {
			t.Errorf("%s: want one note naming the profile and --profile full, got %q", profile, note)
		}
	}
}
