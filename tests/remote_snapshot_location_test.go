package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// deployLayout builds a deployer-style tree under a temp dir and returns the
// deploy root and a remote config whose Path is the served `current` link.
func deployLayout(t *testing.T) (string, engine.RemoteConfig) {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"releases/1", "shared"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "releases", "1"), filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}
	return root, engine.RemoteConfig{Host: "example.com", User: "deploy", Path: filepath.Join(root, "current")}
}

func runSh(t *testing.T, command string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s\n%s", err, command, out)
	}
	return string(out)
}

func TestSnapshotDeployPathDerivation(t *testing.T) {
	cfg := engine.RemoteConfig{Path: "/srv/app/current"}
	if got := remote.SnapshotDeployPath(cfg, ""); got != "/srv/app" {
		t.Fatalf("current link parent expected, got %q", got)
	}
	if got := remote.SnapshotDeployPath(engine.RemoteConfig{Path: "/srv/app"}, ""); got != "/srv/app" {
		t.Fatalf("plain path expected, got %q", got)
	}
	if got := remote.SnapshotDeployPath(cfg, "/data/deploy"); got != "/data/deploy" {
		t.Fatalf("configured deploy path must win, got %q", got)
	}
	override := engine.RemoteConfig{Path: "/srv/app/current", Deploy: &engine.DeployConfig{DeployPath: "/x/y"}}
	if got := remote.SnapshotDeployPath(override, "/data/deploy"); got != "/x/y" {
		t.Fatalf("remote override must win, got %q", got)
	}
}

func TestSnapshotLayoutProbeAndRoot(t *testing.T) {
	root, cfg := deployLayout(t)
	deployPath := remote.SnapshotDeployPath(cfg, "")
	if deployPath != root {
		t.Fatalf("deploy path = %q, want %q", deployPath, root)
	}
	layout := strings.TrimSpace(runSh(t, remote.SnapshotLayoutProbeCommand(deployPath)))
	if layout != remote.SnapshotLayoutDeploy {
		t.Fatalf("layout = %q, want deploy", layout)
	}
	if got, want := remote.SnapshotRootForLayout(cfg, deployPath, layout), filepath.Join(root, "shared", ".govard", "snapshots"); got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}

	plain := engine.RemoteConfig{Path: t.TempDir()}
	layout = strings.TrimSpace(runSh(t, remote.SnapshotLayoutProbeCommand(remote.SnapshotDeployPath(plain, ""))))
	if layout != remote.SnapshotLayoutLegacy {
		t.Fatalf("layout = %q, want legacy", layout)
	}
	if got, want := remote.SnapshotRootForLayout(plain, plain.Path, layout), filepath.Join(plain.Path, ".govard", "snapshots"); got != want {
		t.Fatalf("legacy root = %q, want %q", got, want)
	}
}

func TestSnapshotCreateAtSharedRootIsOwnerOnly(t *testing.T) {
	setPermissiveUmask(t)
	root, cfg := deployLayout(t)
	sharedRoot := filepath.Join(root, "shared", ".govard", "snapshots")
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(sharedRoot, "snap", "magento2", "echo dump", ""))
	for _, p := range []string{filepath.Join(root, "shared", ".govard"), sharedRoot, filepath.Join(sharedRoot, "snap")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %o, want 700", p, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(cfg.Path, ".govard")); err == nil {
		t.Fatal("snapshot must not be written inside the served release")
	}
}

func TestSnapshotListLookupSpansBothLocations(t *testing.T) {
	setPermissiveUmask(t)
	root, cfg := deployLayout(t)
	newRoot := filepath.Join(root, "shared", ".govard", "snapshots")
	legacyRoot := remote.RemoteSnapshotRoot(cfg)
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(newRoot, "fresh", "magento2", "echo dump", ""))
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(legacyRoot, "old", "magento2", "echo dump", ""))

	out := runSh(t, remote.BuildRemoteSnapshotListCommand(cfg))
	entries, err := remote.ParseRemoteSnapshotListEntries(out)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, entry := range entries {
		got[entry.Meta.Name] = entry.Root
	}
	if got["fresh"] != newRoot || got["old"] != legacyRoot {
		t.Fatalf("entries = %#v, want fresh in %s and old in %s", got, newRoot, legacyRoot)
	}
	if len(entries) != 2 || entries[0].Meta.Name != "fresh" {
		t.Fatalf("new location must be listed first: %#v", entries)
	}
}

func TestSnapshotDeleteAndRestoreFindLegacyAndNew(t *testing.T) {
	setPermissiveUmask(t)
	root, cfg := deployLayout(t)
	newRoot := filepath.Join(root, "shared", ".govard", "snapshots")
	legacyRoot := remote.RemoteSnapshotRoot(cfg)
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(newRoot, "fresh", "none", "", ""))
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(legacyRoot, "old", "none", "", ""))

	// restore must resolve either location (no db import or media: just the lookup)
	runSh(t, remote.BuildRemoteSnapshotRestoreCommand(cfg, "old", "none", "", "", false, false))
	runSh(t, remote.BuildRemoteSnapshotRestoreCommand(cfg, "fresh", "none", "", "", false, false))

	if out := strings.TrimSpace(runSh(t, remote.BuildRemoteSnapshotLocateCommand(cfg, "old"))); out != filepath.Join(legacyRoot, "old") {
		t.Fatalf("locate old = %q", out)
	}
	if out := strings.TrimSpace(runSh(t, remote.BuildRemoteSnapshotLocateCommand(cfg, "fresh"))); out != filepath.Join(newRoot, "fresh") {
		t.Fatalf("locate fresh = %q", out)
	}

	runSh(t, remote.BuildRemoteSnapshotDeleteCommand(cfg, "old"))
	runSh(t, remote.BuildRemoteSnapshotDeleteCommand(cfg, "fresh"))
	for _, p := range []string{filepath.Join(legacyRoot, "old"), filepath.Join(newRoot, "fresh")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s should have been deleted", p)
		}
	}
}

func TestSnapshotRestoreMissingSnapshotFails(t *testing.T) {
	_, cfg := deployLayout(t)
	if err := exec.Command("sh", "-c", remote.BuildRemoteSnapshotRestoreCommand(cfg, "ghost", "none", "", "", false, false)).Run(); err == nil {
		t.Fatal("restoring a missing snapshot must fail")
	}
	if err := exec.Command("sh", "-c", remote.BuildRemoteSnapshotLocateCommand(cfg, "ghost")).Run(); err == nil {
		t.Fatal("locating a missing snapshot must fail")
	}
}

func TestSnapshotListNoDeployLayoutOnlyLegacy(t *testing.T) {
	setPermissiveUmask(t)
	cfg := engine.RemoteConfig{Path: t.TempDir()}
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(remote.RemoteSnapshotRoot(cfg), "old", "none", "", ""))
	entries, err := remote.ParseRemoteSnapshotListEntries(runSh(t, remote.BuildRemoteSnapshotListCommand(cfg)))
	if err != nil || len(entries) != 1 || entries[0].Meta.Name != "old" {
		t.Fatalf("entries = %#v err=%v", entries, err)
	}
}

// The metadata must describe what the snapshot holds: the old command wrote
// `db: true` and `media: true` unconditionally, so a snapshot of a project with
// no media directory (or no dump command) claimed archives that do not exist.
func TestSnapshotMetadataReflectsWhatWasCaptured(t *testing.T) {
	setPermissiveUmask(t)
	cases := []struct {
		name      string
		dump      string
		mediaDir  bool
		wantDB    string
		wantMedia string
	}{
		{"db and media", "echo dump", true, "db: true", "media: true"},
		{"no media directory", "echo dump", false, "db: true", "media: false"},
		{"no dump command", "", true, "db: false", "media: true"},
		{"neither", "", false, "db: false", "media: false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "snapshots")
			media := filepath.Join(t.TempDir(), "media")
			if tc.mediaDir {
				if err := os.MkdirAll(media, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(media, "a.txt"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(root, "snap", "laravel", tc.dump, media))
			meta, err := os.ReadFile(filepath.Join(root, "snap", "metadata.yml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tc.wantDB, tc.wantMedia} {
				if !strings.Contains(string(meta), want) {
					t.Fatalf("metadata missing %q:\n%s", want, meta)
				}
			}
			_, mediaErr := os.Stat(filepath.Join(root, "snap", "media.tar.gz"))
			if (mediaErr == nil) != tc.mediaDir {
				t.Fatalf("media archive presence = %v, want %v", mediaErr == nil, tc.mediaDir)
			}
		})
	}
}

// fakeClientBin writes a database client named `name` that records stdin into
// out and fails unless the password reached it through the environment.
func fakeClientBin(t *testing.T, name string, out string) string {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$MYSQL_PWD\" = 'secret' ] || { echo 'Access denied (using password: NO)' >&2; exit 3; }\ncat > '" + out + "'\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestSnapshotRestorePassesThePasswordToTheClient(t *testing.T) {
	setPermissiveUmask(t)
	root, cfg := deployLayout(t)
	sharedRoot := filepath.Join(root, "shared", ".govard", "snapshots")
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(sharedRoot, "snap", "laravel", "echo 'SELECT 1;' | gzip", ""))

	out := filepath.Join(t.TempDir(), "imported.sql")
	bin := fakeClientBin(t, "mysql", out)
	importCmd := "export MYSQL_PWD='secret'; mysql --no-defaults -uapp appdb -f"
	command := remote.BuildRemoteSnapshotRestoreCommand(cfg, "snap", "laravel", importCmd, "", true, false)
	runSh(t, "PATH="+bin+":$PATH; "+command)

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the client never received the dump: %v", err)
	}
	if strings.TrimSpace(string(got)) != "SELECT 1;" {
		t.Fatalf("client stdin = %q, want the decompressed dump", got)
	}
}

func TestSnapshotRestoreFallsBackToTheMariaDBClient(t *testing.T) {
	setPermissiveUmask(t)
	if _, err := exec.LookPath("mysql"); err == nil {
		t.Skip("a real mysql client is installed, the fallback cannot be isolated")
	}
	root, cfg := deployLayout(t)
	sharedRoot := filepath.Join(root, "shared", ".govard", "snapshots")
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(sharedRoot, "snap", "laravel", "echo 'SELECT 2;' | gzip", ""))

	out := filepath.Join(t.TempDir(), "imported.sql")
	bin := fakeClientBin(t, "mariadb", out)
	importCmd := "export MYSQL_PWD='secret'; mysql --no-defaults -uapp appdb -f"
	command := remote.BuildRemoteSnapshotRestoreCommand(cfg, "snap", "laravel", importCmd, "", true, false)
	runSh(t, "PATH="+bin+":$PATH; "+command)

	got, err := os.ReadFile(out)
	if err != nil || strings.TrimSpace(string(got)) != "SELECT 2;" {
		t.Fatalf("mariadb client did not receive the dump: %q %v", got, err)
	}
}

func TestSnapshotCreateFailureLeavesNothingBehind(t *testing.T) {
	setPermissiveUmask(t)
	root := filepath.Join(t.TempDir(), "snapshots")

	// A good snapshot first, so a failed re-create of the same name can be
	// shown to leave it untouched.
	runSh(t, remote.BuildRemoteSnapshotCreateCommandAtRoot(root, "keep", "laravel", "echo good", ""))
	before, err := os.ReadFile(filepath.Join(root, "keep", "db.sql.gz"))
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"fresh", "keep"} {
		command := remote.BuildRemoteSnapshotCreateCommandAtRoot(root, name, "laravel", "echo partial; exit 2", "")
		if err := exec.Command("sh", "-c", command).Run(); err == nil {
			t.Fatalf("create %q must fail when the dump fails", name)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "keep" {
		t.Fatalf("a failed create left %v behind, want only the earlier good snapshot", names)
	}
	after, err := os.ReadFile(filepath.Join(root, "keep", "db.sql.gz"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("the existing snapshot was changed by a failed re-create: %q vs %q (%v)", after, before, err)
	}
	if _, err := os.Stat(filepath.Join(root, "keep", "metadata.yml")); err != nil {
		t.Fatalf("the existing snapshot lost its metadata: %v", err)
	}
}
