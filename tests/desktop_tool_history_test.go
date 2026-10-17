package tests

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"govard/internal/desktop"
)

// History entries are scrubbed on write: a command carrying a DB password or
// a credentialed URL must land in the file with the secret replaced, never
// raw. The entry also records the run's op ID and generation so later
// consumers (palette UI, operation center) can attribute terminal events
// idempotently keyed on (op, gen).
func TestToolHistoryScrubsSecretsOnWrite(t *testing.T) {
	project := seedToolRunnerProject(t, "laravel")
	fake := &toolRunFake{}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	const dbSecret = "s3cret-db-value"
	const urlSecret = "s3cret-url-value"
	if _, err := desktop.RunTool(context.Background(), p, project, "laravel-migrate",
		map[string]string{"database": "DB_PASSWORD=" + dbSecret}); err != nil {
		t.Fatalf("RunTool() error: %v", err)
	}
	waitForToolEvents(t, p, desktop.EventToolCompleted, 1, 5*time.Second)
	waitForToolLockFree(t, 5*time.Second)

	if _, err := desktop.RunTool(context.Background(), p, project, "laravel-migrate",
		map[string]string{"database": "https://deploy:" + urlSecret + "@registry.example.com/composer"}); err != nil {
		t.Fatalf("RunTool() error: %v", err)
	}
	waitForToolEvents(t, p, desktop.EventToolCompleted, 2, 5*time.Second)
	waitForToolLockFree(t, 5*time.Second)

	raw, err := os.ReadFile(desktop.ToolHistoryPathForTest(project))
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	for _, secret := range []string{dbSecret, urlSecret} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("history file stores raw secret %q; scrub-on-write must replace it", secret)
		}
	}
	if !strings.Contains(string(raw), "***") {
		t.Error("history file has no redaction marker; expected scrubbed values to carry ***")
	}

	entries, err := desktop.GetToolHistory(project)
	if err != nil {
		t.Fatalf("GetToolHistory() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("GetToolHistory() returned %d entries, want 2", len(entries))
	}
	for _, entry := range entries {
		for name, value := range entry.Args {
			if strings.Contains(value, dbSecret) || strings.Contains(value, urlSecret) {
				t.Errorf("history entry arg %q stores raw secret %q", name, value)
			}
		}
		if entry.OpID == "" {
			t.Error("history entry has an empty op ID; consumers key terminal events on (op, gen)")
		}
		if entry.Preset != "laravel-migrate" {
			t.Errorf("history entry preset = %q, want %q", entry.Preset, "laravel-migrate")
		}
		if entry.Timestamp.IsZero() {
			t.Error("history entry has a zero timestamp")
		}
		if entry.Outcome != desktop.ToolHistoryOutcomeSuccess {
			t.Errorf("history entry outcome = %q, want %q after completion", entry.Outcome, desktop.ToolHistoryOutcomeSuccess)
		}
	}

	// The recorded (op, gen) must match the terminal event payloads, or the
	// consumer cannot attribute them idempotently.
	completed := waitForToolEvents(t, p, desktop.EventToolCompleted, 2, 5*time.Second)
	seen := map[string]desktop.ToolHistoryEntry{}
	for _, entry := range entries {
		seen[entry.OpID+"/"+itoa(int(entry.Gen))] = entry
	}
	for _, payload := range completed {
		result, ok := payload.(desktop.ToolResultPayload)
		if !ok {
			t.Fatalf("tool:completed payload has type %T, want ToolResultPayload", payload)
		}
		if _, ok := seen[result.OpID+"/"+itoa(int(result.Gen))]; !ok {
			t.Errorf("terminal event (op %q, gen %d) has no matching history entry", result.OpID, result.Gen)
		}
	}
}

// A run whose terminal event never arrives (crash) must read as
// unknown/interrupted, never success: the entry is written at admission with
// the unknown outcome, and only a terminal event promotes it.
func TestToolHistoryInterruptedUntilTerminalArrives(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	fake := &toolRunFake{block: true}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	opID, err := desktop.RunTool(context.Background(), p, project, "npm-install", nil)
	if err != nil {
		t.Fatalf("RunTool() error: %v", err)
	}

	midFlight, err := desktop.GetToolHistory(project)
	if err != nil {
		t.Fatalf("GetToolHistory() mid-run error: %v", err)
	}
	if len(midFlight) != 1 {
		t.Fatalf("GetToolHistory() mid-run returned %d entries, want 1 (admission write)", len(midFlight))
	}
	if midFlight[0].OpID != opID {
		t.Errorf("mid-run entry op ID = %q, want %q", midFlight[0].OpID, opID)
	}
	if midFlight[0].Outcome == desktop.ToolHistoryOutcomeSuccess {
		t.Errorf("mid-run entry outcome = %q; a run with no terminal event must never read success", midFlight[0].Outcome)
	}
	if midFlight[0].Outcome != desktop.ToolHistoryOutcomeUnknown {
		t.Errorf("mid-run entry outcome = %q, want %q", midFlight[0].Outcome, desktop.ToolHistoryOutcomeUnknown)
	}

	if !desktop.CancelToolRun(opID) {
		t.Fatalf("CancelToolRun(%q) = false, want true", opID)
	}
	waitForToolEvents(t, p, desktop.EventToolFailed, 1, 5*time.Second)
	waitForToolLockFree(t, 5*time.Second)

	after, err := desktop.GetToolHistory(project)
	if err != nil {
		t.Fatalf("GetToolHistory() after cancel error: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("GetToolHistory() after cancel returned %d entries, want 1 (same run updated)", len(after))
	}
	if after[0].Gen != midFlight[0].Gen {
		t.Errorf("cancel rewrote the run as gen %d, want the admission gen %d (same run, outcome updated)", after[0].Gen, midFlight[0].Gen)
	}
	if after[0].Outcome != desktop.ToolHistoryOutcomeCancelled {
		t.Errorf("cancelled run outcome = %q, want %q", after[0].Outcome, desktop.ToolHistoryOutcomeCancelled)
	}
}

// The per-project history caps at 50: the 51st run evicts the oldest.
func TestToolHistoryEvictsOldestBeyondCap(t *testing.T) {
	project := seedToolRunnerProject(t, "laravel")
	fake := &toolRunFake{}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	const runs = 51
	for i := 0; i < runs; i++ {
		db := "db" + itoaPad(i)
		if _, err := desktop.RunTool(context.Background(), p, project, "laravel-migrate",
			map[string]string{"database": db}); err != nil {
			t.Fatalf("RunTool(%d) error: %v", i, err)
		}
		waitForToolEvents(t, p, desktop.EventToolCompleted, i+1, 10*time.Second)
		waitForToolLockFree(t, 5*time.Second)
	}

	entries, err := desktop.GetToolHistory(project)
	if err != nil {
		t.Fatalf("GetToolHistory() error: %v", err)
	}
	if len(entries) != 50 {
		t.Fatalf("GetToolHistory() returned %d entries, want the 50-entry cap", len(entries))
	}
	for i, entry := range entries {
		want := "db" + itoaPad(i+1)
		if entry.Args["database"] != want {
			t.Errorf("entry %d database = %q, want %q (oldest evicted first)", i, entry.Args["database"], want)
		}
	}
}

// History is file-backed under GOVARD_HOME: it survives the in-memory service
// lifetime (registry reset), unlike op registrations and generations.
func TestToolHistorySurvivesRegistryReset(t *testing.T) {
	project := seedToolRunnerProject(t, "magento2")
	fake := &toolRunFake{}
	installToolRunFake(t, fake)
	p := &desktop.FakePlatform{}

	if _, err := desktop.RunTool(context.Background(), p, project, "magento-cache-flush",
		map[string]string{"type": "full_page"}); err != nil {
		t.Fatalf("RunTool() error: %v", err)
	}
	waitForToolEvents(t, p, desktop.EventToolCompleted, 1, 5*time.Second)
	waitForToolLockFree(t, 5*time.Second)

	desktop.ResetOperationRegistryForTest()

	entries, err := desktop.GetToolHistory(project)
	if err != nil {
		t.Fatalf("GetToolHistory() after registry reset error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("GetToolHistory() after reset returned %d entries, want 1 (file-backed)", len(entries))
	}
	if entries[0].Outcome != desktop.ToolHistoryOutcomeSuccess {
		t.Errorf("surviving entry outcome = %q, want %q", entries[0].Outcome, desktop.ToolHistoryOutcomeSuccess)
	}
}

func TestToolHistoryEmptyAndUnresolvable(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	entries, err := desktop.GetToolHistory(project)
	if err != nil {
		t.Fatalf("GetToolHistory(fresh project) error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("GetToolHistory(fresh project) returned %d entries, want none", len(entries))
	}
	if _, err := desktop.GetToolHistory(""); err == nil {
		t.Error("GetToolHistory(\"\") = nil error, want the resolution failure surfaced")
	}
}

func itoaPad(n int) string {
	s := itoa(n)
	for len(s) < 2 {
		s = "0" + s
	}
	return s
}

// A bare secret under a secret-named key has no KEY=/URL/PEM marker in its
// value, so value-shape scrubbing alone leaks it. The key name drives
// scrubbing too: {"password": "s3cret"} stores {"password": "***"}.
// RunTool's preset validator would reject such keys at the gate, so this
// drives the store through the same scrub+append path RunTool uses.
func TestToolHistoryScrubsSecretKeyNames(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	args := map[string]string{
		"password":    "s3cret-bare",
		"csrf-token":  "tok-csrf-1",
		"db-password": "db-pw-1",
		"apiKey":      "ak-access-1",
		"access-key":  "ak-secret-1",
		"database":    "mysql",
		"type":        "full_page",
	}
	scrubbed := desktop.ScrubToolHistoryArgsForTest(args)
	for _, name := range []string{"password", "csrf-token", "db-password", "apiKey", "access-key"} {
		if scrubbed[name] != "***" {
			t.Errorf("scrubbed arg %q = %q, want the whole value replaced regardless of shape", name, scrubbed[name])
		}
	}
	for name, want := range map[string]string{"database": "mysql", "type": "full_page"} {
		if scrubbed[name] != want {
			t.Errorf("scrubbed arg %q = %q, want plain keys left alone", name, scrubbed[name])
		}
	}
	if args["password"] != "s3cret-bare" {
		t.Error("scrubbing mutated the caller's map; it must copy")
	}

	entry := desktop.ToolHistoryEntry{
		OpID: "tool:key-names", Gen: 1, Preset: "npm-install",
		Args:      scrubbed,
		Timestamp: time.Now().UTC(), Outcome: desktop.ToolHistoryOutcomeSuccess,
	}
	if err := desktop.AppendToolHistoryLineForTest(project, entry); err != nil {
		t.Fatalf("append history line: %v", err)
	}
	raw, err := os.ReadFile(desktop.ToolHistoryPathForTest(project))
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	for _, secret := range []string{"s3cret-bare", "tok-csrf-1", "db-pw-1", "ak-access-1", "ak-secret-1"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("history file stores raw secret %q; key-name scrubbing must replace it", secret)
		}
	}
	if !strings.Contains(string(raw), "mysql") {
		t.Error("history file lost the plain arg value; only secret keys are replaced")
	}
	stored, err := desktop.GetToolHistory(project)
	if err != nil {
		t.Fatalf("GetToolHistory() error: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("GetToolHistory() returned %d entries, want 1", len(stored))
	}
	if stored[0].Args["password"] != "***" || stored[0].Args["database"] != "mysql" {
		t.Errorf("stored args = %q, want password *** and database mysql", stored[0].Args)
	}
}

// The URL matcher is scheme-generic: any scheme://user:pass@ shape is
// credentialed (amqp://, not just http/mysql). A bare user@host and a
// passwordless URL survive; PEM blocks are replaced wholesale.
func TestToolHistoryScrubsGenericCredentialedURLsAndPEM(t *testing.T) {
	const keyBody = "MIIBOgIBAAJBAKc2s2V5qwIDAQAB"
	args := map[string]string{
		"uri":   "amqp://deploy:s3cret-amqp@mq.example.com/vhost",
		"plain": "https://registry.example.com/composer",
		"bare":  "https://deploy@registry.example.com/composer",
		"key":   "-----BEGIN RSA PRIVATE KEY-----\n" + keyBody + "\n-----END RSA PRIVATE KEY-----",
	}
	scrubbed := desktop.ScrubToolHistoryArgsForTest(args)
	if strings.Contains(scrubbed["uri"], "s3cret-amqp") {
		t.Errorf("amqp credentialed URL not scrubbed: %q", scrubbed["uri"])
	}
	if !strings.Contains(scrubbed["uri"], "amqp://***@mq.example.com/vhost") {
		t.Errorf("amqp URL scrub shape = %q, want userinfo replaced keeping host", scrubbed["uri"])
	}
	if scrubbed["plain"] != args["plain"] {
		t.Errorf("passwordless URL changed: %q", scrubbed["plain"])
	}
	if scrubbed["bare"] != args["bare"] {
		t.Errorf("bare user@host changed (no credential to scrub): %q", scrubbed["bare"])
	}
	if strings.Contains(scrubbed["key"], keyBody) || strings.Contains(scrubbed["key"], "PRIVATE KEY") {
		t.Errorf("PEM block not replaced wholesale: %q", scrubbed["key"])
	}
}

// Concurrent admissions must stay whole lines: the hot path writes one entry
// with one O_APPEND call, so no writer can interleave a partial line.
func TestToolHistoryConcurrentAppendsStayWholeLines(t *testing.T) {
	project := seedToolRunnerProject(t, "custom")
	const writers = 8
	const perWriter = 25
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				entry := desktop.ToolHistoryEntry{
					OpID: "tool:concurrent", Gen: uint64(w*perWriter + i),
					Preset:    "marker-" + itoa(w) + "-" + itoa(i),
					Timestamp: time.Now().UTC(), Outcome: desktop.ToolHistoryOutcomeUnknown,
				}
				if err := desktop.AppendToolHistoryLineForTest(project, entry); err != nil {
					t.Errorf("concurrent append: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	raw, err := os.ReadFile(desktop.ToolHistoryPathForTest(project))
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != writers*perWriter {
		t.Fatalf("history file holds %d lines, want %d (entries lost or merged)", len(lines), writers*perWriter)
	}
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		var entry desktop.ToolHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line is not one whole entry (partial interleave): %q: %v", line, err)
		}
		seen[entry.Preset] = true
	}
	if len(seen) != writers*perWriter {
		t.Errorf("history holds %d distinct entries, want %d (entries lost)", len(seen), writers*perWriter)
	}
}
