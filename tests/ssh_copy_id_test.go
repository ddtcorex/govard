package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// `prod` is what blocks here, not a capability: engine.RemoteWriteBlocked reads
// only `Protected` and the normalised remote name
// (internal/engine/remote_policy.go:126-138), and "prod" normalises to the
// production environment. A capability map on the fixture would have said
// nothing about why the offer is refused, so this covers both arms of that
// predicate instead — the production name, and an explicit `--protected` flag
// on a remote the name alone would never protect.
func TestSSHKeyCopyOfferIsSkippedForAProtectedRemote(t *testing.T) {
	if cmd.ShouldOfferSSHKeyCopyForTest("prod", engine.RemoteConfig{}, true, "/tmp/id_rsa.pub") {
		t.Fatal("a remote whose name normalises to production must never be offered an authorized_keys write")
	}
	flagged := engine.RemoteConfig{Protected: engine.BoolPtr(true)}
	if cmd.ShouldOfferSSHKeyCopyForTest("staging", flagged, true, "/tmp/id_rsa.pub") {
		t.Fatal("a remote carrying the explicit protected flag must never be offered an authorized_keys write")
	}
}

func TestSSHKeyCopyOfferNeedsATerminalAndAKey(t *testing.T) {
	cfg := engine.RemoteConfig{}
	if cmd.ShouldOfferSSHKeyCopyForTest("staging", cfg, false, "/tmp/id_rsa.pub") {
		t.Fatal("a non-interactive run must never be offered a key copy")
	}
	if cmd.ShouldOfferSSHKeyCopyForTest("staging", cfg, true, "") {
		t.Fatal("no local public key means there is nothing to copy")
	}
	if !cmd.ShouldOfferSSHKeyCopyForTest("staging", cfg, true, "/tmp/id_rsa.pub") {
		t.Fatal("an unprotected remote in a terminal with a key is exactly the case the offer exists for")
	}
}

func TestSSHKeyCopyConfirmDefaultsToNo(t *testing.T) {
	if cmd.SSHKeyCopyConfirmPrinterForTest().DefaultValue {
		t.Fatal("Enter must not mean yes for a command that writes authorized_keys")
	}
}

// The protected-remote guard has to run before remote.ProbeSSHAuth, not after
// it: a protected remote must never be dialled at all, not merely refused a
// key copy once the probe already reported an auth failure. A pure predicate
// cannot see that ordering, so this drives the real helper with an `ssh` shim
// on PATH that records every invocation.
func TestSSHKeyCopyOfferNeverContactsAProtectedRemote(t *testing.T) {
	probeLog, copyLog := installSSHKeyCopyShimsForTest(t, false)
	restore := cmd.SetStdinIsTerminalForTest(func() bool { return true })
	t.Cleanup(restore)

	cfg := engine.RemoteConfig{Host: "prod.example.com", User: "deploy"}
	if err := cmd.OfferSSHKeyCopyOnAuthFailureForTest("prod", cfg); err != nil {
		t.Fatalf("a protected remote must not turn into a caller error, got %v", err)
	}
	assertNotRecordedForSSHKeyCopy(t, probeLog, "a protected remote was dialled over SSH")
	assertNotRecordedForSSHKeyCopy(t, copyLog, "a protected remote had a key copied into it")
}

// Fence, not a new behaviour: the non-tty gate predates this change and is the
// only thing that lets `govard remote test` (verify item P4-01) honestly carry
// the REMOTE-PROBE label — `offerSSHKeyCopyOnAuthFailure` must still return
// before any prompt when stdin is not a terminal. The internal/verify
// registry comment says the same thing and cites this gate by line number.
func TestSSHKeyCopyOfferCopiesNothingWithoutATerminal(t *testing.T) {
	probeLog, copyLog := installSSHKeyCopyShimsForTest(t, true)
	restore := cmd.SetStdinIsTerminalForTest(func() bool { return false })
	t.Cleanup(restore)

	cfg := engine.RemoteConfig{Host: "staging.example.com", User: "deploy"}
	if err := cmd.OfferSSHKeyCopyOnAuthFailureForTest("staging", cfg); err != nil {
		t.Fatalf("a non-interactive auth failure must not turn into a caller error, got %v", err)
	}
	if _, err := os.Stat(copyLog); !os.IsNotExist(err) {
		t.Fatalf("a non-interactive run copied a key anyway: %s", readSSHKeyCopyLog(t, copyLog))
	}
	if _, err := os.Stat(probeLog); os.IsNotExist(err) {
		t.Fatal("the SSH auth probe never ran, so this test no longer reaches the tty gate it fences")
	}
}

// installSSHKeyCopyShimsForTest puts a recording `ssh` and `ssh-copy-id` first on
// PATH. Nothing is dialled and nothing is copied: each shim only appends its
// argv to a log file, so "was this run?" is answered by the log's existence.
// authFails makes the probe report the auth failure `remote.IsAuthFailure`
// recognises, which is what drives the helper past the probe.
func installSSHKeyCopyShimsForTest(t *testing.T, authFails bool) (probeLog, copyLog string) {
	t.Helper()
	dir := t.TempDir()
	probeLog = filepath.Join(dir, "probe.log")
	copyLog = filepath.Join(dir, "copy.log")

	probe := "#!/bin/sh\nprintf 'ssh|%s\\n' \"$*\" >> " + probeLog + "\n"
	if authFails {
		probe += "printf 'deploy@staging.example.com: Permission denied (publickey).\\n' >&2\nexit 255\n"
	} else {
		probe += "exit 0\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(probe), 0o755); err != nil {
		t.Fatalf("write ssh shim: %v", err)
	}

	copier := "#!/bin/sh\nprintf 'ssh-copy-id|%s\\n' \"$*\" >> " + copyLog + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh-copy-id"), []byte(copier), 0o755); err != nil {
		t.Fatalf("write ssh-copy-id shim: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return probeLog, copyLog
}

func assertNotRecordedForSSHKeyCopy(t *testing.T, log, message string) {
	t.Helper()
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("%s: %s", message, readSSHKeyCopyLog(t, log))
	}
}

func readSSHKeyCopyLog(t *testing.T, log string) string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(data)
}
