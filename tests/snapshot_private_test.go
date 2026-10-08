package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"gopkg.in/yaml.v3"
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
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
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

// The whole generated create command must run to completion under a real sh
// and leave a parseable metadata.yml whose values round-trip verbatim, even
// when the name and framework contain shell-significant characters.
func TestRemoteSnapshotCreateCommandRunsToCompletion(t *testing.T) {
	setPermissiveUmask(t)
	root := t.TempDir()
	cfg := engine.RemoteConfig{Host: "example.com", User: "deploy", Path: filepath.Join(root, "app")}
	name := "it's a $x snap"
	framework := "my 'fw' $y"
	command := remote.BuildRemoteSnapshotCreateCommand(cfg, name, framework, "echo dump", "")
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("whole create command failed under sh: %v\n%s", err, out)
	}
	snapDir := filepath.Join(root, "app", ".govard", "snapshots", name)
	assertMode0600(t, filepath.Join(snapDir, "db.sql.gz"))
	raw, err := os.ReadFile(filepath.Join(snapDir, "metadata.yml"))
	if err != nil {
		t.Fatalf("read metadata.yml: %v", err)
	}
	var meta map[string]any
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("metadata.yml is not valid YAML: %v\n%s", err, raw)
	}
	// No media path was given, so the metadata must not claim a media archive.
	if meta["name"] != name || meta["framework"] != framework || meta["db"] != true || meta["media"] != false {
		t.Fatalf("metadata values did not round-trip: %#v\n%s", meta, raw)
	}
	// An unquoted ISO-8601 UTC timestamp decodes to time.Time, the type the
	// snapshot metadata struct uses; the raw text pins the exact format.
	if _, ok := meta["created_at"].(time.Time); !ok {
		t.Fatalf("created_at decoded as %T, want time.Time", meta["created_at"])
	}
	if !regexp.MustCompile(`(?m)^created_at: \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).Match(raw) {
		t.Fatalf("created_at is not ISO-8601 UTC in:\n%s", raw)
	}
}
