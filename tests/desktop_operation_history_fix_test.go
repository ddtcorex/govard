package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// TestOpHistory_ScrubsSecretArgsOnDisk pins the WS-D secret ruling: args
// carrying passwords, tokens, DSNs, or PEM blocks land scrubbed (***)
// everywhere — the running snapshot, the crash sidecar, and the history
// file — via the shared tool-history scrubber, never raw.
func TestOpHistory_ScrubsSecretArgsOnDisk(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	const kvSecret = "s3cret-kv-value"
	const envSecret = "s3cret-env-value"
	const dsnSecret = "s3cret-dsn-value"
	const nextSecret = "s3cret-next-value"
	const keyBody = "MIIBOgIBAAJBAKc2s2V5qwIDAQAB"
	args := []string{
		"sync", "--remote", "origin",
		"--password=" + kvSecret,
		"MYSQL_PWD=" + envSecret,
		"mysql://dbuser:" + dsnSecret + "@db:3306/shop",
		"-----BEGIN RSA PRIVATE KEY-----\n" + keyBody + "\n-----END RSA PRIVATE KEY-----",
		"--token", nextSecret,
	}
	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", args, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}

	// Crash sidecar must already be scrubbed while the op is in flight.
	if raw, err := os.ReadFile(desktop.OperationInflightPathForTest()); err != nil {
		t.Fatalf("read inflight sidecar: %v", err)
	} else {
		assertNoRawSecrets(t, "inflight sidecar", string(raw), kvSecret, envSecret, dsnSecret, nextSecret, keyBody)
	}

	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusCompleted, "ok") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}

	raw, err := os.ReadFile(desktop.OperationHistoryPathForTest())
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	assertNoRawSecrets(t, "history file", string(raw), kvSecret, envSecret, dsnSecret, nextSecret, keyBody)
	if !strings.Contains(string(raw), "***") {
		t.Error("history file has no *** marker; expected scrubbed values")
	}

	history := desktop.ListHistory(0)
	if len(history) != 1 {
		t.Fatalf("ListHistory = %d records, want 1", len(history))
	}
	stored := history[0].Args
	for _, want := range []string{"--password=***", "MYSQL_PWD=***", "mysql://***@db:3306/shop", "--token", "***"} {
		if !slicesContains(stored, want) {
			t.Errorf("stored args = %q, want element %q", stored, want)
		}
	}
	if !desktop.OpArgsContainScrubMarkerForTest(stored) {
		t.Errorf("stored args = %q, want scrub marker detected (fail-closed replay contract)", stored)
	}
}

// TestOpHistory_ScrubbedReplayFailsValidation pins the fail-closed half of
// the secret ruling: clean records replay, scrubbed records must be refused
// by Task 3's RetryOp validation. The detector is the contract both sides
// share.
func TestOpHistory_ScrubbedReplayFailsValidation(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	if !desktop.RegisterOpWithArgs("op-clean", "proj-a", "sync", []string{"sync", "--remote", "origin"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(clean) = false, want true")
	}
	if !desktop.FinishOpWithStatus("op-clean", desktop.OpStatusFailed, "nope") {
		t.Fatalf("FinishOpWithStatus(clean) = false, want true")
	}
	if !desktop.RegisterOpWithArgs("op-secret", "proj-a", "sync", []string{"sync", "--password=s3cret"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(secret) = false, want true")
	}
	if !desktop.FinishOpWithStatus("op-secret", desktop.OpStatusFailed, "nope") {
		t.Fatalf("FinishOpWithStatus(secret) = false, want true")
	}

	history := desktop.ListHistory(0)
	if len(history) != 2 {
		t.Fatalf("ListHistory = %d records, want 2", len(history))
	}
	byID := map[string]desktop.OpRecord{}
	for _, record := range history {
		byID[record.ID] = record
	}
	if desktop.OpArgsContainScrubMarkerForTest(byID["op-clean"].Args) {
		t.Errorf("clean args %q flagged scrubbed, want replayable", byID["op-clean"].Args)
	}
	if !desktop.OpArgsContainScrubMarkerForTest(byID["op-secret"].Args) {
		t.Errorf("scrubbed args %q not flagged, want retry refused (fail closed)", byID["op-secret"].Args)
	}
	for _, secret := range []string{"s3cret"} {
		raw, _ := os.ReadFile(desktop.OperationHistoryPathForTest())
		if strings.Contains(string(raw), secret) {
			t.Errorf("history file stores raw secret %q", secret)
		}
	}
}

// TestOpHistory_CrashAfterAppendNoDuplicate covers the crash window between
// terminal-append and sidecar-delete: the terminal record already landed,
// so recovery must drop the stale sidecar entry, not duplicate history.
func TestOpHistory_CrashAfterAppendNoDuplicate(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	args := []string{"sync", "--remote", "origin"}
	if !desktop.RegisterOpWithArgs("sync:proj-a", "proj-a", "sync", args, func() {}) {
		t.Fatalf("RegisterOpWithArgs(new id) = false, want true")
	}
	snap, err := os.ReadFile(desktop.OperationInflightPathForTest())
	if err != nil {
		t.Fatalf("snapshot sidecar: %v", err)
	}
	if !desktop.FinishOpWithStatus("sync:proj-a", desktop.OpStatusCompleted, "done") {
		t.Fatalf("FinishOpWithStatus(known id) = false, want true")
	}
	if _, err := os.Stat(desktop.OperationInflightPathForTest()); !os.IsNotExist(err) {
		t.Fatalf("sidecar still present after clean finish, want removed")
	}

	// Crash replay: the sidecar write never landed, memory is gone.
	if err := os.WriteFile(desktop.OperationInflightPathForTest(), snap, 0o600); err != nil {
		t.Fatalf("restore sidecar snapshot: %v", err)
	}
	desktop.ClearOperationRegistryMemoryForTest()

	if got := desktop.RecoverInterruptedOps(); got != 0 {
		t.Fatalf("RecoverInterruptedOps = %d, want 0 (terminal already recorded)", got)
	}
	history := desktop.ListHistory(0)
	if len(history) != 1 {
		t.Fatalf("ListHistory after recovery = %d records, want 1 (no duplicate)", len(history))
	}
	if history[0].Status != desktop.OpStatusCompleted {
		t.Errorf("record Status = %q, want completed (no mislabel)", history[0].Status)
	}
}

// TestOpHistory_CorruptSidecarRenamedAside covers the corrupt-sidecar
// window: recovery must quarantine the bytes with a logged warning, record
// nothing, and invent no interruptions.
func TestOpHistory_CorruptSidecarRenamedAside(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()

	garbage := []byte("{not json,,,")
	if err := os.MkdirAll(filepath.Dir(desktop.OperationInflightPathForTest()), 0o700); err != nil {
		t.Fatalf("mkdir sidecar dir: %v", err)
	}
	if err := os.WriteFile(desktop.OperationInflightPathForTest(), garbage, 0o600); err != nil {
		t.Fatalf("write corrupt sidecar: %v", err)
	}

	if got := desktop.RecoverInterruptedOps(); got != 0 {
		t.Fatalf("RecoverInterruptedOps = %d, want 0 for corrupt sidecar", got)
	}
	if history := desktop.ListHistory(0); len(history) != 0 {
		t.Fatalf("ListHistory after corrupt recovery = %d records, want 0 (no invented interruptions)", len(history))
	}
	if _, err := os.Stat(desktop.OperationInflightPathForTest()); !os.IsNotExist(err) {
		t.Fatalf("corrupt sidecar still in place, want quarantined aside")
	}
	backups, err := filepath.Glob(desktop.OperationInflightPathForTest() + ".corrupt-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("quarantine backups = %v, err %v; want exactly one .corrupt-* file", backups, err)
	}
	kept, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("read quarantine backup: %v", err)
	}
	if string(kept) != string(garbage) {
		t.Errorf("quarantine backup = %q, want original bytes %q preserved", kept, garbage)
	}
}

// TestOpHistory_CapCountsValidRecordsOnly pins retention accounting: blank
// and corrupt lines never consume cap budget, and the overflow rewrite
// drops them while evicting the oldest valid records.
func TestOpHistory_CapCountsValidRecordsOnly(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	seedHistoryProjectForTest(t, "proj-a")

	path := desktop.OperationHistoryPathForTest()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir history dir: %v", err)
	}
	var sb strings.Builder
	now := time.Now().UTC()
	for i := 1; i <= 199; i++ {
		line, err := json.Marshal(desktop.OpRecord{
			ID:         fmt.Sprintf("cap-%03d", i),
			Project:    "proj-a",
			Kind:       "sync",
			Status:     desktop.OpStatusCompleted,
			StartedAt:  now,
			EndedAt:    now,
			DurationMS: 0,
		})
		if err != nil {
			t.Fatalf("marshal seed record: %v", err)
		}
		sb.Write(line)
		sb.WriteByte('\n')
	}
	sb.WriteString("\n   \n{corrupt json\n")
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("seed history file: %v", err)
	}

	if !desktop.RegisterOpWithArgs("op-new", "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs = false, want true")
	}
	if !desktop.FinishOpWithStatus("op-new", desktop.OpStatusCompleted, "") {
		t.Fatalf("FinishOpWithStatus = false, want true")
	}

	// 199 valid + 1 new = 200 valid: still under cap, so the junk lines stay
	// on disk but never render or consume budget.
	if history := desktop.ListHistory(0); len(history) != 200 {
		t.Fatalf("ListHistory = %d records, want 200 valid (junk excluded)", len(history))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	if got := len(strings.Split(strings.TrimSpace(string(raw)), "\n")); got <= 200 {
		t.Fatalf("raw history file = %d lines, want > 200 (junk retained on the append path, budget counts valid only)", got)
	}
	if history := desktop.ListHistory(0); history[0].ID != "op-new" {
		t.Fatalf("newest record = %q, want op-new", history[0].ID)
	}

	if !desktop.RegisterOpWithArgs("op-overflow", "proj-a", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs = false, want true")
	}
	if !desktop.FinishOpWithStatus("op-overflow", desktop.OpStatusCompleted, "") {
		t.Fatalf("FinishOpWithStatus = false, want true")
	}

	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != desktop.OperationHistoryCap {
		t.Fatalf("history file = %d lines, want exactly cap %d (junk dropped)", len(lines), desktop.OperationHistoryCap)
	}
	for _, line := range lines {
		var record desktop.OpRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil || strings.TrimSpace(record.ID) == "" {
			t.Fatalf("history file line does not parse after overflow rewrite: %q", line)
		}
	}
	history := desktop.ListHistory(0)
	if len(history) != desktop.OperationHistoryCap {
		t.Fatalf("ListHistory = %d records, want cap %d", len(history), desktop.OperationHistoryCap)
	}
	if history[0].ID != "op-overflow" {
		t.Errorf("newest record = %q, want op-overflow", history[0].ID)
	}
	if history[len(history)-1].ID != "cap-002" {
		t.Errorf("oldest kept record = %q, want cap-002 (cap-001 evicted)", history[len(history)-1].ID)
	}
}

// TestOpHistory_RegisterUnknownNameNeverTouchesDocker pins the registry
// path off Docker: with `docker` shadowed by a sentinel script and
// DOCKER_HOST poisoned, registering an unknown identifier must still
// succeed fast under a fail-consistent cleaned-input key.
func TestOpHistory_RegisterUnknownNameNeverTouchesDocker(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	desktop.ResetOperationRegistryForTest()
	t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))

	bindir := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "docker-calls.log")
	script := "#!/bin/sh\necho called \"$@\" >> \"" + sentinel + "\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bindir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-govard-docker-down.sock")

	if !desktop.RegisterOpWithArgs("sync:mystery", "unknown-project-xyz", "sync", []string{"sync"}, func() {}) {
		t.Fatalf("RegisterOpWithArgs(unknown name) = false, want true (fail-consistent)")
	}
	ops := desktop.ListOps()
	if len(ops) != 1 {
		t.Fatalf("ListOps = %d ops, want 1", len(ops))
	}
	if ops[0].Project != "unknown-project-xyz" {
		t.Errorf("op Project = %q, want fail-consistent cleaned-input key %q", ops[0].Project, "unknown-project-xyz")
	}
	if !desktop.FinishOpWithStatus("sync:mystery", desktop.OpStatusCompleted, "") {
		t.Fatalf("FinishOpWithStatus = false, want true")
	}
	if history := desktop.ListHistory(0); len(history) != 1 {
		t.Fatalf("ListHistory = %d records, want 1", len(history))
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("fake docker was executed (sentinel present); RegisterOp must never shell out")
	}
}

func assertNoRawSecrets(t *testing.T, where string, haystack string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(haystack, secret) {
			t.Errorf("%s stores raw secret %q; scrub-on-write must replace it", where, secret)
		}
	}
}

func slicesContains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
