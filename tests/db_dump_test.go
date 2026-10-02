package tests

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// driveLocalDump runs the real `db dump --file` against a docker shim and
// returns the dump path, so the assertions hold for the wiring and not only for
// a helper.
func driveLocalDump(t *testing.T, dumpPath string) {
	t.Helper()
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, "project_name: sample-project\ndomain: sample.test\nframework: laravel\n")
	shimDir := t.TempDir()
	installDBCommandRuntimeDockerShim(t, shimDir)
	t.Setenv("DB_COMMAND_RUNTIME_LOG", filepath.Join(shimDir, "docker.log"))
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"db", "dump", "--file", dumpPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("db dump failed: %v", err)
	}
}

func TestDumpFileIsCreated0600(t *testing.T) {
	t.Run("new file", func(t *testing.T) {
		setPermissiveUmask(t)
		dumpPath := filepath.Join(t.TempDir(), "dump.sql")
		driveLocalDump(t, dumpPath)
		assertMode0600(t, dumpPath)
	})

	t.Run("overwrite of a pre-existing 0644 file", func(t *testing.T) {
		dumpPath := filepath.Join(t.TempDir(), "dump.sql")
		if err := os.WriteFile(dumpPath, []byte("stale and much longer previous content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dumpPath, 0o644); err != nil {
			t.Fatal(err)
		}
		driveLocalDump(t, dumpPath)
		assertMode0600(t, dumpPath)
		data, err := os.ReadFile(dumpPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "stale") || !strings.Contains(string(data), "CREATE TABLE demo") {
			t.Fatalf("the existing file was not truncated and rewritten: %q", data)
		}
	})
}

// setPermissiveUmask makes the process (and the sh it spawns) create files 0644
// unless the code under test asks for something tighter.
func setPermissiveUmask(t *testing.T) {
	t.Helper()
	previous := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(previous) })
}

func assertMode0600(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("%s mode = %o, want 600", path, got)
	}
}

// The remote dump command must make the file owner-only and keep a path with a
// space as one dirname argument. The command string is executed under a real sh
// with HOME pointing at a temp dir and fake dump binaries, so word splitting
// would create stray directories or fail.
func TestRemoteDumpCommandSetsUmaskAndQuotesDirname(t *testing.T) {
	setPermissiveUmask(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	for _, name := range []string{"mysqldump", "mariadb-dump"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho 'CREATE TABLE demo (id INT);'\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	config := engine.Config{
		Framework: "unknown",
		Remotes: map[string]engine.RemoteConfig{
			"dev": {
				Host:         "example.com",
				User:         "deploy",
				Capabilities: &engine.RemoteCapabilities{DB: engine.BoolPtr(true)},
			},
		},
	}
	args, err := cmd.BuildDBDumpCommandForTest(config, cmd.DBCommandOptions{Environment: "dev", File: "~/my backups/x.sql.gz"})
	if err != nil {
		t.Fatalf("BuildDBDumpCommandForTest: %v", err)
	}
	remoteCmd := args[len(args)-1]

	if !strings.HasPrefix(remoteCmd, "umask 077; ") {
		t.Fatalf("remote command must set the umask before anything else: %s", remoteCmd)
	}
	if !strings.Contains(remoteCmd, `mkdir -p "$(dirname `) {
		t.Fatalf("the dirname substitution must be double-quoted: %s", remoteCmd)
	}

	// `sh -n` proves the string parses; the real run proves there is no splitting.
	if out, err := exec.Command("sh", "-n", "-c", remoteCmd).CombinedOutput(); err != nil {
		t.Fatalf("sh -n rejected the command: %v\n%s", err, out)
	}
	if out, err := exec.Command("sh", "-c", remoteCmd).CombinedOutput(); err != nil {
		t.Fatalf("command failed under sh: %v\n%s", err, out)
	}

	target := filepath.Join(home, "my backups", "x.sql.gz")
	assertMode0600(t, target)
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	// .govard is the SSH multiplex socket directory the command builder creates.
	names := []string{}
	for _, e := range entries {
		if e.Name() != ".govard" {
			names = append(names, e.Name())
		}
	}
	if len(names) != 1 || names[0] != "my backups" {
		t.Fatalf("word splitting left stray entries in HOME: %v", names)
	}
}
