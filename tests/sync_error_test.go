package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestHandleRsyncError(t *testing.T) {
	// Helper to get real exec.ExitError
	getExitErr := func(code int) error {
		cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
		return cmd.Run()
	}

	tests := []struct {
		name          string
		err           error
		scope         string
		expectedCont  bool
		expectedError bool
	}{
		{"nil error", nil, "Media", true, false},
		{"Media exit 23", getExitErr(23), "Media", true, false},
		{"Media exit 24", getExitErr(24), "Media", true, false},
		{"Media fatal 1", getExitErr(1), "Media", false, true},
		{"Files exit 23", getExitErr(23), "Files", false, true},
		{"Files exit 24", getExitErr(24), "Files", false, true},
		{"Files fatal 1", getExitErr(1), "Files", false, true},
		{"Generic error", fmt.Errorf("random error"), "Media", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cont, err := cmd.HandleRsyncErrorForTest(tt.err, tt.scope)
			if cont != tt.expectedCont {
				t.Errorf("cont = %v, want %v", cont, tt.expectedCont)
			}
			if (err != nil) != tt.expectedError {
				t.Errorf("err = %v, want error: %v", err, tt.expectedError)
			}
		})
	}
}

func TestHandleRsyncErrorMissingMediaRoot(t *testing.T) {
	exit23 := exec.Command("sh", "-c", "exit 23").Run()
	exit24 := exec.Command("sh", "-c", "exit 24").Run()

	missing := "rsync: [sender] change_dir \"/srv/app/public/media\" failed: No such file or directory (2)\nrsync error: some files/attrs were not transferred (code 23)"
	cont, err := cmd.HandleRsyncErrorWithOutputForTest(exit23, "Media", missing)
	if cont || err == nil {
		t.Fatalf("missing media root must fail, got cont=%v err=%v", cont, err)
	}
	if !strings.Contains(err.Error(), "media directory") {
		t.Errorf("error should explain the missing media directory, got %v", err)
	}

	partial := "rsync: [sender] send_files failed to open \"a.jpg\": Permission denied (13)"
	cont, err = cmd.HandleRsyncErrorWithOutputForTest(exit23, "Media", partial)
	if !cont || err != nil {
		t.Errorf("partial media errors stay tolerated, got cont=%v err=%v", cont, err)
	}
	cont, err = cmd.HandleRsyncErrorWithOutputForTest(exit24, "Media", "")
	if !cont || err != nil {
		t.Errorf("vanished files (24) stay tolerated, got cont=%v err=%v", cont, err)
	}
}

func rsyncArgsFor(t *testing.T, sourceLocal bool) []string {
	t.Helper()
	remoteEP := cmd.SyncEndpoint{Name: "staging", RemoteCfg: engine.RemoteConfig{Host: "example.test", User: "deploy", Port: 22}}
	localEP := cmd.SyncEndpoint{Name: "local", IsLocal: true}
	src, dst := remoteEP, localEP
	if sourceLocal {
		src, dst = localEP, remoteEP
	}
	c, _, err := cmd.BuildRsyncForEndpointsForTest(src, dst, "/srv/app/etc", "/work/etc", true)
	if err != nil {
		t.Fatal(err)
	}
	return c.Args
}

func TestPullRsyncIgnoresOutsideSymlinksByDefault(t *testing.T) {
	cmd.SetSyncResolveSymlinksForTest(false)
	args := rsyncArgsFor(t, false)
	if !slices.Contains(args, "--safe-links") || slices.Contains(args, "--copy-unsafe-links") {
		t.Fatalf("a pull must ignore symlinks that leave the synced tree by default, args: %v", args)
	}
	push := rsyncArgsFor(t, true)
	if slices.Contains(push, "--safe-links") || slices.Contains(push, "--copy-unsafe-links") {
		t.Fatalf("push must keep symlinks as they are, args: %v", push)
	}
}

func TestPullRsyncResolvesOutsideSymlinksOnlyWhenAsked(t *testing.T) {
	cmd.SetSyncResolveSymlinksForTest(true)
	t.Cleanup(func() { cmd.SetSyncResolveSymlinksForTest(false) })
	args := rsyncArgsFor(t, false)
	if !slices.Contains(args, "--copy-unsafe-links") || slices.Contains(args, "--safe-links") {
		t.Fatalf("--resolve-symlinks must switch a pull to --copy-unsafe-links, args: %v", args)
	}
}

// runPullRsync runs the real rsync with the flags a pull builds, minus the ssh
// transport, from a release tree whose env.php is a symlink into shared/.
func runPullRsync(t *testing.T, resolve bool) (dst string, shared string) {
	t.Helper()
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not installed")
	}
	cmd.SetSyncResolveSymlinksForTest(resolve)
	t.Cleanup(func() { cmd.SetSyncResolveSymlinksForTest(false) })
	root := t.TempDir()
	shared = filepath.Join(root, "shared")
	src := filepath.Join(root, "release", "app", "etc")
	dst = filepath.Join(root, "local", "etc")
	for _, d := range []string{shared, src, dst} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(shared, "env.php"), []byte("remote-shared"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "env.php"), []byte("local-original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(shared, "env.php"), filepath.Join(src, "env.php")); err != nil {
		t.Fatal(err)
	}
	args := rsyncArgsFor(t, false)
	var flags []string
	for _, a := range args[1:] {
		if a == "-e" {
			break
		}
		flags = append(flags, a)
	}
	if out, err := exec.Command("rsync", append(flags, src+"/", dst+"/")...).CombinedOutput(); err != nil {
		t.Fatalf("rsync: %v\n%s", err, out)
	}
	return dst, shared
}

func TestDefaultPullKeepsTheLocalFileAndReadsNothingOutsideThePath(t *testing.T) {
	dst, _ := runPullRsync(t, false)
	info, err := os.Lstat(filepath.Join(dst, "env.php"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("a link that leaves the tree must not be recreated locally (it would dangle)")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "env.php")); string(b) != "local-original" {
		t.Fatalf("local file was replaced: %q (the remote file outside the synced path must not be read)", b)
	}
}

func TestResolveSymlinksPullCopiesTheContentNotALink(t *testing.T) {
	dst, _ := runPullRsync(t, true)
	info, err := os.Lstat(filepath.Join(dst, "env.php"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("env.php was copied as a symlink")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "env.php")); string(b) != "remote-shared" {
		t.Fatalf("content = %q", b)
	}
}

func TestCappedBufferIsSafeForConcurrentWritersAndKeepsTheTail(t *testing.T) {
	buf := cmd.NewCappedBufferForTest(16)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = buf.Write([]byte("0123456789"))
			}
		}()
	}
	wg.Wait()
	if got := buf.String(); len(got) != 16 {
		t.Fatalf("buffer kept %d bytes, want the 16-byte tail", len(got))
	}
}
