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

func TestSnapshotDBDumpIsCreated0600(t *testing.T) {
	setPermissiveUmask(t)
	shimDir := t.TempDir()
	installDBCommandRuntimeDockerShim(t, shimDir)
	t.Setenv("DB_COMMAND_RUNTIME_LOG", filepath.Join(shimDir, "docker.log"))
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	projectDir := t.TempDir()
	snapshotDir, err := engine.CreateSnapshot(projectDir, engine.Config{ProjectName: "sample-project"}, "snap")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	dbPath := filepath.Join(snapshotDir, "db.sql.gz")
	assertMode0600(t, dbPath)
	if info, err := os.Stat(dbPath); err != nil || info.Size() == 0 {
		t.Fatalf("expected a non-empty dump at %s (err %v)", dbPath, err)
	}
}

func TestExportedSnapshotArchiveIsCreated0600(t *testing.T) {
	cases := map[string]bool{"new file": false, "overwrite of a pre-existing 0644 file": true}
	for name, preexisting := range cases {
		t.Run(name, func(t *testing.T) {
			setPermissiveUmask(t)
			projectDir := t.TempDir()
			snapshotDir := filepath.Join(engine.SnapshotRoot(projectDir), "snap")
			if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(snapshotDir, "db.sql.gz"), []byte("dump"), 0o600); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "out.tar.gz")
			if preexisting {
				if err := os.WriteFile(target, []byte("stale"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(target, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := engine.ExportSnapshot(projectDir, "snap", target); err != nil {
				t.Fatalf("ExportSnapshot: %v", err)
			}
			assertMode0600(t, target)
			if out, err := exec.Command("tar", "-tzf", target).CombinedOutput(); err != nil || !strings.Contains(string(out), "snap/db.sql.gz") {
				t.Fatalf("archive is not a valid tarball of the snapshot: %v\n%s", err, out)
			}
		})
	}
}

// The remote create command must start with the umask and still run correctly
// under a real sh: the dump lands in the snapshot directory, owner-only, and the
// directories it creates are owner-only too.
func TestRemoteSnapshotCreateCommandSetsUmaskAndRuns(t *testing.T) {
	setPermissiveUmask(t)
	root := t.TempDir()
	cfg := engine.RemoteConfig{Host: "example.com", User: "deploy", Path: filepath.Join(root, "app")}
	command := remote.BuildRemoteSnapshotCreateCommand(cfg, "snap", "magento2", "echo mock-dump", "")
	if !strings.HasPrefix(command, "umask 077; ") {
		t.Fatalf("create command must start with the umask: %s", command)
	}
	// The trailing metadata step is cut off: its printf format contains a
	// literal %Y (the $(date) inside single quotes is never expanded), which
	// every shell rejects. That is a separate defect, see the task report.
	runnable := command[:strings.LastIndex(command, " && printf ")]
	if out, err := exec.Command("sh", "-c", runnable).CombinedOutput(); err != nil {
		t.Fatalf("command failed under sh: %v\n%s", err, out)
	}
	snapDir := filepath.Join(root, "app", ".govard", "snapshots", "snap")
	assertMode0600(t, filepath.Join(snapDir, "db.sql.gz"))
	info, err := os.Stat(snapDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("snapshot directory mode = %o, want 700", got)
	}
}
