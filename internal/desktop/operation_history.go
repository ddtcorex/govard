package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/conventions"
)

// OpStatus is the terminal state of a desktop operation history record.
// Retry (Task 3) replays the stored args of failed records only; every
// other status is informational.
type OpStatus string

const (
	OpStatusCompleted   OpStatus = "completed"
	OpStatusFailed      OpStatus = "failed"
	OpStatusCancelled   OpStatus = "cancelled"
	OpStatusInterrupted OpStatus = "interrupted"
)

// OperationHistoryCap bounds the persisted JSONL history: appending past
// the cap evicts the oldest records so the file never grows without bound
// while the newest entries stay intact.
const OperationHistoryCap = 200

// opHistoryMessageMax caps stored messages so one verbose failure cannot
// bloat the history file; over-long input is cut with a truncation marker.
const opHistoryMessageMax = 2000

// opHistoryInterruptedMessage marks records recovered at startup: the op
// was in flight when the app last exited and never reached a terminal
// state, so recovery records it instead of staying silent.
const opHistoryInterruptedMessage = "Interrupted by app restart"

// OpRecord is one terminal desktop operation: what ran, under which
// canonical project key (see canonicalProjectIDForRegistry), with which
// replay args, and how it ended. Args are scrubbed at write (see
// scrubToolHistoryArgv) so the history file never stores raw secrets; a
// scrubbed record carries "***" placeholders that no longer authenticate,
// so Task 3's RetryOp must refuse such records at arg validation (fail
// closed with an actionable error, asking for fresh credentials) instead
// of replaying a doomed invocation. Only Message is length-capped; Args
// stay element-complete so non-secret replays keep their exact shape.
type OpRecord struct {
	ID         string    `json:"id"`
	Project    string    `json:"project"`
	Kind       string    `json:"kind"`
	Args       []string  `json:"args,omitempty"`
	Status     OpStatus  `json:"status"`
	Message    string    `json:"message,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMS int64     `json:"duration_ms"`
}

// inflightEntry is the crash-recovery sidecar: every registered op is
// mirrored here until it reaches a terminal state. A startup scan finds
// entries no live process finished and marks them interrupted.
type inflightEntry struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Kind      string    `json:"kind"`
	Args      []string  `json:"args,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// operationHistoryDir resolves the desktop state dir. It honors
// GOVARD_HOME_DIR through conventions, so tests stay hermetic.
func operationHistoryDir() string {
	return filepath.Join(conventions.GetGovardHome(), "desktop")
}

func operationHistoryPath() string {
	return filepath.Join(operationHistoryDir(), "operation-history.jsonl")
}

func operationInflightPath() string {
	return filepath.Join(operationHistoryDir(), "operation-inflight.json")
}

// OperationHistoryPathForTest exposes the history file location for tests.
func OperationHistoryPathForTest() string {
	return operationHistoryPath()
}

// OperationInflightPathForTest exposes the crash-recovery sidecar location
// for tests.
func OperationInflightPathForTest() string {
	return operationInflightPath()
}

// OpArgsContainScrubMarkerForTest exposes scrub-placeholder detection for
// tests: Task 3's RetryOp must refuse records whose args carry "***".
func OpArgsContainScrubMarkerForTest(args []string) bool {
	return opArgsContainScrubMarker(args)
}

// RegisterOpWithArgs records a cancellable op like RegisterOp and keeps a
// scrubbed copy of args for history replay. The project is stored under
// its Docker-free canonical registry ID (see
// canonicalProjectIDForRegistry), so later lookups by name, domain, or
// path hit one key. That key can diverge from the sync-guard key (see
// canonicalProjectID) for Docker-only projects the registry never learned:
// the guard resolves through container labels and orphan listings while this
// path must never shell out, so such a project keys here by cleaned input
// and at the guard by its resolved root. The divergence is accepted because
// the registry path stays hermetic and Docker-down safe; when the WS-A/B/C
// replay handlers register here they must resolve through the same
// Docker-free helper, never by importing the guard's Docker-backed probes.
// Registering an id that is already running is rejected
// (false) exactly like RegisterOp.
func RegisterOpWithArgs(id string, project string, kind string, args []string, cancel context.CancelFunc) bool {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	if _, ok := operationRegistry.running[id]; ok {
		return false
	}
	operationRegistry.running[id] = operationEntry{
		info: OpInfo{
			ID:        strings.TrimSpace(id),
			Project:   canonicalProjectIDForRegistry(project),
			Kind:      strings.TrimSpace(kind),
			Args:      scrubToolHistoryArgv(args),
			StartedAt: time.Now(),
		},
		cancel: cancel,
	}
	persistInflightLocked()
	return true
}

// FinishOpWithStatus unregisters a finished op and appends its terminal
// record (status plus sanitized, secret-scrubbed message) to history. It reports false when
// no running op exists for the id, in which case nothing is recorded, so a
// terminal state is appended exactly once no matter how finish and cancel
// race: only the side that claims the running entry records.
func FinishOpWithStatus(id string, status OpStatus, message string) bool {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	entry, ok := operationRegistry.running[id]
	if !ok {
		return false
	}
	delete(operationRegistry.running, id)
	now := time.Now()
	// Terminal-append BEFORE sidecar-delete: a crash between the two must
	// leave a terminal record plus a stale sidecar entry (which recovery
	// dedupes, see RecoverInterruptedOps), never a deleted entry with no
	// record (which recovery would mislabel as interrupted).
	appendHistoryRecord(OpRecord{
		ID:         entry.info.ID,
		Project:    entry.info.Project,
		Kind:       entry.info.Kind,
		Args:       scrubToolHistoryArgv(entry.info.Args),
		Status:     normalizeOpStatus(status),
		Message:    sanitizeUIText(scrubToolHistoryText(strings.TrimSpace(message)), opHistoryMessageMax),
		StartedAt:  entry.info.StartedAt,
		EndedAt:    now,
		DurationMS: now.Sub(entry.info.StartedAt).Milliseconds(),
	})
	persistInflightLocked()
	return true
}

// ListHistory returns persisted terminal records, newest first, capped at
// limit (limit <= 0 means all). Corrupt lines are skipped; a missing or
// unreadable file reads as empty history, never an error, so the history
// view renders on a fresh machine.
func ListHistory(limit int) []OpRecord {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	records := readHistoryRecords()
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records
}

// RecoverInterruptedOps marks orphan in-flight ops as interrupted in
// history: entries the on-disk sidecar holds that no live op finished
// (the app exited mid-op). A sidecar entry that already has a terminal
// record for the same (op ID, start time) is dropped, not re-marked: the
// terminal-append runs before the sidecar-delete, so a crash between them
// leaves exactly this state, and recording it again would duplicate the
// history. IDs alone do not disambiguate (ids recycle across register
// cycles), hence the start-time pairing. Live running ids are left
// untouched. It returns how many orphans were marked. Call once at startup
// before registering ops; it is idempotent (a second call finds no orphans).
func RecoverInterruptedOps() int {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	entries, err := readInflightFile()
	if err != nil {
		quarantineInflightFile(err)
		return 0
	}
	if len(entries) == 0 {
		return 0
	}
	terminals := readTerminalKeys()
	now := time.Now()
	marked := 0
	kept := make(map[string]inflightEntry, len(entries))
	for id, entry := range entries {
		if _, live := operationRegistry.running[id]; live {
			kept[id] = entry
			continue
		}
		if terminals[opTerminalKey(entry.ID, entry.StartedAt)] {
			continue
		}
		appendHistoryRecord(OpRecord{
			ID:         entry.ID,
			Project:    entry.Project,
			Kind:       entry.Kind,
			Args:       scrubToolHistoryArgv(entry.Args),
			Status:     OpStatusInterrupted,
			Message:    sanitizeUIText(opHistoryInterruptedMessage, opHistoryMessageMax),
			StartedAt:  entry.StartedAt,
			EndedAt:    now,
			DurationMS: now.Sub(entry.StartedAt).Milliseconds(),
		})
		marked++
	}
	writeInflightFile(kept)
	return marked
}

// opTerminalKey identifies one op generation for recovery dedupe: the id
// plus the exact start instant, since ids recycle across register cycles
// and the id alone could drop a fresh in-flight op behind an old record.
func opTerminalKey(id string, started time.Time) string {
	return id + "\x00" + started.UTC().Format(time.RFC3339Nano)
}

// readTerminalKeys indexes the (id, start time) pairs that already have a
// terminal record, so recovery never duplicates a crash-orphaned sidecar
// entry whose terminal-append already landed.
func readTerminalKeys() map[string]bool {
	keys := map[string]bool{}
	for _, record := range readHistoryRecords() {
		keys[opTerminalKey(record.ID, record.StartedAt)] = true
	}
	return keys
}

// quarantineInflightFile renames an unreadable sidecar aside instead of
// treating it as zero orphans: silently dropping it would lose the evidence
// of what was in flight, and inventing interruptions from unreadable bytes
// would lie. The corrupt copy keeps its bytes for forensics; the next
// register starts a fresh sidecar. Best-effort: failures are logged, never
// fatal to startup.
func quarantineInflightFile(cause error) {
	backup := operationInflightPath() + ".corrupt-" + time.Now().UTC().Format("20060102T150405.999999999Z")
	if err := os.Rename(operationInflightPath(), backup); err != nil {
		log.Printf("warning: unreadable op inflight sidecar (%v); quarantine failed: %v", cause, err)
		return
	}
	log.Printf("warning: unreadable op inflight sidecar (%v); moved aside to %s", cause, backup)
}

// ClearOperationRegistryMemoryForTest drops running state without recording
// terminal states, simulating a process restart where the on-disk
// in-flight set survives but memory does not.
func ClearOperationRegistryMemoryForTest() {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	operationRegistry.running = map[string]operationEntry{}
}

// scrubToolHistoryArgv copies CLI argv with every secret scrubbed through
// the shared tool-history primitives (see tool_history.go), so
// the history file and the crash sidecar never store raw secrets. Each
// element passes through scrubToolHistoryText (secret KEY=value pairs,
// credentialed-URL userinfo, PEM blocks); additionally a bare secret flag
// (`--password`, `--token`, `MYSQL_PWD`, ...) taints the following element,
// which is the value position. Scrubbing is idempotent: "***" carries no
// secret shape, so scrubbing an already-scrubbed slice is a fixed point.
func scrubToolHistoryArgv(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	scrubbed := make([]string, len(args))
	taintNext := false
	for i, arg := range args {
		if taintNext {
			// Over-scrub is deliberate (safe direction): a bare secret flag
			// taints the next element whatever it holds.
			scrubbed[i] = "***"
			taintNext = false
			continue
		}
		if flag, hasValue := splitArgFlag(arg); !hasValue && isToolHistorySecretKey(flag) {
			scrubbed[i] = arg
			taintNext = true
			continue
		}
		scrubbed[i] = scrubToolHistoryText(arg)
	}
	return scrubbed
}

// splitArgFlag strips leading dashes from one argv element and reports the
// flag name plus whether it carries an inline value (`--password=x`).
func splitArgFlag(arg string) (string, bool) {
	name := strings.TrimLeft(strings.TrimSpace(arg), "-")
	if name == "" {
		return "", false
	}
	if idx := strings.Index(name, "="); idx >= 0 {
		return name[:idx], true
	}
	return name, false
}

// opArgsContainScrubMarker reports whether argv carries a "***" scrub
// placeholder left by scrubToolHistoryArgv. Task 3's RetryOp must refuse
// such records at arg validation (fail closed with an actionable error
// asking for fresh credentials): the placeholder no longer authenticates,
// so replaying it can only fail opaquely deep in the service path, and the
// refusal itself must never print the args. Non-secret args contain no
// "***" by construction of the scrubber output.
func opArgsContainScrubMarker(args []string) bool {
	for _, arg := range args {
		// Substring match over-scrubs by design (safe direction): a legit
		// "***" in non-secret args only costs a replay refusal, never a leak.
		if strings.Contains(arg, "***") {
			return true
		}
	}
	return false
}

// normalizeOpStatus keeps the four known terminal states and folds empty
// or unknown input to failed (fail closed): a lost outcome must never read
// as done, which is the most misleading choice.
func normalizeOpStatus(status OpStatus) OpStatus {
	switch status {
	case OpStatusCompleted, OpStatusFailed, OpStatusCancelled, OpStatusInterrupted:
		return status
	default:
		return OpStatusFailed
	}
}

// appendHistoryRecord persists one terminal record, evicting the oldest
// past the cap. The common path is a single O_APPEND line write; only an
// append that would overflow the cap pays for a full read-trim-rewrite.
// The cap counts valid records only: blank and corrupt lines never
// consume budget, and the overflow rewrite drops them.
// Errors are swallowed: memory stays the source of truth for running ops
// and history is best-effort durable state, so a full disk must never
// fail a finish/cancel path.
//
// Cross-process stance (mirroring tool history): the registry mutex
// serializes in-process writers only. Admissions are single O_APPEND line
// writes and rewrites are atomic temp+rename publishes, so residual races
// with another process can only lose an entry or briefly exceed the cap,
// never corrupt a line. No flock.
func appendHistoryRecord(record OpRecord) {
	payload, err := json.Marshal(record)
	if err != nil {
		return
	}
	payload = append(payload, '\n')
	if countValidHistoryRecords()+1 <= OperationHistoryCap {
		if err := os.MkdirAll(operationHistoryDir(), conventions.SecretDirPerm); err != nil {
			return
		}
		file, err := os.OpenFile(operationHistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, conventions.SecretFilePerm)
		if err != nil {
			return
		}
		_, _ = file.Write(payload)
		_ = file.Close()
		return
	}
	records := readHistoryRecords()
	records = append(records, record)
	if len(records) > OperationHistoryCap {
		records = records[len(records)-OperationHistoryCap:]
	}
	_ = writeHistoryRecords(records)
}

// parseHistoryRecord validates one JSONL line: blank lines, corrupt JSON,
// and records without an id never consume cap budget and never render.
func parseHistoryRecord(line string) (OpRecord, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return OpRecord{}, false
	}
	var record OpRecord
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return OpRecord{}, false
	}
	if strings.TrimSpace(record.ID) == "" {
		return OpRecord{}, false
	}
	return record, true
}

// countValidHistoryRecords counts valid records without rendering them, so
// the non-overflow append path stays cheap while blanks and corrupt lines
// stay outside the cap budget.
func countValidHistoryRecords() int {
	file, err := os.Open(operationHistoryPath())
	if err != nil {
		return 0
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if _, ok := parseHistoryRecord(scanner.Text()); ok {
			count++
		}
	}
	return count
}

// readHistoryRecords loads every well-formed record, oldest first,
// skipping corrupt lines so one bad write never hides the whole history.
func readHistoryRecords() []OpRecord {
	records := []OpRecord{}
	file, err := os.Open(operationHistoryPath())
	if err != nil {
		return records
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if record, ok := parseHistoryRecord(scanner.Text()); ok {
			records = append(records, record)
		}
	}
	return records
}

// writeHistoryRecords rewrites the whole file atomically (temp + rename)
// so a crash mid-write never leaves a half-truncated history.
func writeHistoryRecords(records []OpRecord) error {
	if err := os.MkdirAll(operationHistoryDir(), conventions.SecretDirPerm); err != nil {
		return err
	}
	var builder strings.Builder
	for _, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			continue
		}
		builder.Write(payload)
		builder.WriteByte('\n')
	}
	tmp, err := os.CreateTemp(operationHistoryDir(), "operation-history-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(builder.String()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, conventions.SecretFilePerm); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, operationHistoryPath()); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// persistInflightLocked mirrors the running map to the crash-recovery
// sidecar. Callers hold the registry mutex. Errors are swallowed for the
// same reason as appendHistoryRecord.
func persistInflightLocked() {
	entries := make(map[string]inflightEntry, len(operationRegistry.running))
	for id, entry := range operationRegistry.running {
		entries[id] = inflightEntry{
			ID:        entry.info.ID,
			Project:   entry.info.Project,
			Kind:      entry.info.Kind,
			Args:      append([]string{}, entry.info.Args...),
			StartedAt: entry.info.StartedAt,
		}
	}
	_ = writeInflightFile(entries)
}

// readInflightFile loads the crash-recovery sidecar; a missing or empty
// file reads as no orphans. Unreadable or corrupt content is an error for
// the caller to quarantine (see quarantineInflightFile): inventing
// interruptions from unreadable bytes would lie, and silently treating it
// as zero would lose the evidence.
func readInflightFile() (map[string]inflightEntry, error) {
	entries := map[string]inflightEntry{}
	data, err := os.ReadFile(operationInflightPath())
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, fmt.Errorf("read operation inflight sidecar: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return entries, nil
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse operation inflight sidecar: %w", err)
	}
	return entries, nil
}

// writeInflightFile stores the sidecar atomically; an empty set removes
// the file so no stale orphans survive a clean shutdown.
func writeInflightFile(entries map[string]inflightEntry) error {
	if len(entries) == 0 {
		if err := os.Remove(operationInflightPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(operationHistoryDir(), conventions.SecretDirPerm); err != nil {
		return err
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(operationHistoryDir(), "operation-inflight-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, conventions.SecretFilePerm); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, operationInflightPath()); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
