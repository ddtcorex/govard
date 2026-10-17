package desktop

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
)

// ToolHistoryEntry is one recorded tool run: which op generation ran what,
// when, and how it ended. Consumers (palette UI, operation center) attribute
// terminal events idempotently keyed on (OpID, Gen).
type ToolHistoryEntry struct {
	OpID      string            `json:"opId"`
	Gen       uint64            `json:"gen"`
	Preset    string            `json:"preset"`
	Args      map[string]string `json:"args,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Outcome   string            `json:"outcome"`
}

// Tool history outcomes. Unknown is the admission state: only a terminal
// event promotes it, so a run whose terminal never arrives (crash) reads
// unknown, never success.
const (
	ToolHistoryOutcomeUnknown   = "unknown"
	ToolHistoryOutcomeSuccess   = "success"
	ToolHistoryOutcomeFailed    = "failed"
	ToolHistoryOutcomeCancelled = "cancelled"
)

// maxToolHistoryEntries caps the per-project history; the next admission
// evicts the oldest run.
const maxToolHistoryEntries = 50

// toolHistorySecretKeys is the tight normalized secret-key list. Lookup
// normalizes the key by lowercasing and stripping hyphens/underscores, then
// matches exactly or by suffix (so db-password, DATABASE_PASSWORD, and apiKey
// all hit). A suffix match that is not really a secret only hides a value
// behind ***, which is the safe direction.
var toolHistorySecretKeys = []string{
	"password", "passwd", "passphrase",
	"secret", "token", "apikey", "accesskey",
	"privatekey", "pwd",
}

// normalizeToolHistoryKeyName lowercases a key and strips hyphens and
// underscores so csrf-token, CSRF_TOKEN, and csrfToken compare equally.
func normalizeToolHistoryKeyName(name string) string {
	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "-", "")
	lower = strings.ReplaceAll(lower, "_", "")
	return lower
}

// isToolHistorySecretKey reports whether an argument (or KEY=value key) name
// carries a secret by name, regardless of the value shape.
func isToolHistorySecretKey(name string) bool {
	normalized := normalizeToolHistoryKeyName(name)
	for _, key := range toolHistorySecretKeys {
		if normalized == key || strings.HasSuffix(normalized, key) {
			return true
		}
	}
	return false
}

// toolHistorySecretKVPattern matches KEY=value pairs whose key names a
// secret and captures the value for replacement. Case-insensitive; hyphens
// and underscores inside the key are accepted (db-password, api_key); the
// value runs to the next whitespace, comma, semicolon, or quote.
var toolHistorySecretKVPattern = regexp.MustCompile(`(?i)(^|[\s,;"'])([A-Za-z0-9_-]*(?:PASSWORD|PASSWD|PASSPHRASE|PRIVATE[_-]?KEY|SECRET|TOKEN|API[_-]?KEY|ACCESS[_-]?KEY|PWD)\s*=\s*)([^\s,;"']+)`)

// toolHistoryCredentialedURLPattern matches any scheme://userinfo@ URL whose
// userinfo carries a password (user:pass@). The scheme list is deliberately
// open (any scheme:// shape, e.g. amqp://user:pass@host): a closed list
// would leak the next scheme. Bare user@host has no credential and survives.
var toolHistoryCredentialedURLPattern = regexp.MustCompile(`(?i)([A-Za-z][A-Za-z0-9+.-]*://)([^/\s@]+:[^/\s@]*)@`)

// toolHistoryPrivateKeyBlockPattern matches a PEM private-key block;
// toolHistoryPrivateKeyHeaderPattern catches a lone header with no END.
var toolHistoryPrivateKeyBlockPattern = regexp.MustCompile(`-----BEGIN [A-Za-z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Za-z0-9 ]*PRIVATE KEY-----`)
var toolHistoryPrivateKeyHeaderPattern = regexp.MustCompile(`-----BEGIN [A-Za-z0-9 ]*PRIVATE KEY-----`)

// scrubToolHistoryText replaces secret values with ***: secret KEY=value
// pairs, credentialed-URL userinfo, and PEM private-key blocks.
func scrubToolHistoryText(s string) string {
	if s == "" {
		return ""
	}
	s = toolHistorySecretKVPattern.ReplaceAllString(s, `${1}${2}***`)
	s = toolHistoryCredentialedURLPattern.ReplaceAllString(s, `${1}***@`)
	s = toolHistoryPrivateKeyBlockPattern.ReplaceAllString(s, `***`)
	s = toolHistoryPrivateKeyHeaderPattern.ReplaceAllString(s, `***`)
	return s
}

// scrubToolHistoryArgs copies args with every secret scrubbed, so the
// history file never stores raw secrets. A value is scrubbed when its KEY
// name matches (regardless of value shape: {"password": "s3cret"} becomes
// {"password": "***"}) or when the value text itself matches a
// credentialed-URL/PEM pattern. Boundary: a bare secret value under a
// non-secret key ({"database": "s3cret"}) is indistinguishable from an
// innocent value and is NOT detectable; only the key name and the value
// shape can drive scrubbing.
func scrubToolHistoryArgs(args map[string]string) map[string]string {
	if len(args) == 0 {
		return nil
	}
	scrubbed := make(map[string]string, len(args))
	for name, value := range args {
		if isToolHistorySecretKey(name) {
			scrubbed[name] = "***"
			continue
		}
		scrubbed[name] = scrubToolHistoryText(value)
	}
	return scrubbed
}

var toolHistoryFileSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// toolHistoryMu serializes history read-modify-write cycles. Tool runs for
// one project are already single-flight, but runs for different projects (and
// terminal updates racing admissions) share this lock.
var toolHistoryMu sync.Mutex

// toolHistoryDir is the per-project history directory under GOVARD_HOME.
func toolHistoryDir() string {
	return filepath.Join(engine.GovardHomeDir(), "tool-history")
}

// toolHistoryPathForRoot maps a project root to its history file. The sha1
// of the cleaned root disambiguates same-named projects; the sanitized base
// keeps the filename debuggable. The hash also confines the filename to safe
// characters regardless of the project path.
func toolHistoryPathForRoot(root string) string {
	clean := filepath.Clean(root)
	sum := sha1.Sum([]byte(clean))
	base := toolHistoryFileSanitizer.ReplaceAllString(strings.ToLower(filepath.Base(clean)), "-")
	base = strings.Trim(base, "-._")
	if base == "" {
		base = "project"
	}
	return filepath.Join(toolHistoryDir(), fmt.Sprintf("history-%s-%x.jsonl", base, sum[:6]))
}

// readToolHistoryFile parses the JSONL history file, oldest first. Missing
// files mean no history yet (empty, no error); corrupt lines are skipped so
// one torn write never hides the rest of the history.
func readToolHistoryFile(path string) ([]ToolHistoryEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tool history: %w", err)
	}
	var entries []ToolHistoryEntry
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry ToolHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// writeToolHistoryFile replaces the history file atomically (temp + rename)
// so a crash mid-write leaves the previous version intact. The file holds
// scrubbed values only, but still gets strict permissions.
func writeToolHistoryFile(path string, entries []ToolHistoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), conventions.SecretDirPerm); err != nil {
		return fmt.Errorf("create tool history dir: %w", err)
	}
	var sb strings.Builder
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("encode tool history entry: %w", err)
		}
		sb.Write(line)
		sb.WriteByte('\n')
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "history-*.tmp")
	if err != nil {
		return fmt.Errorf("create tool history temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(sb.String()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write tool history temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close tool history temp file: %w", err)
	}
	if err := os.Chmod(tmpName, conventions.SecretFilePerm); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("protect tool history temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("publish tool history: %w", err)
	}
	return nil
}

// appendToolHistoryEntryForRoot records a newly admitted run with the
// unknown outcome via one O_APPEND write call: one entry is one line, so
// concurrent writers (threads or processes) can never interleave partial
// lines from this writer; small O_APPEND writes are atomic. The hot path
// deliberately reads nothing. History is best-effort: callers never fail a
// run over it.
func appendToolHistoryEntryForRoot(root string, entry ToolHistoryEntry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode tool history entry: %w", err)
	}
	line = append(line, '\n')
	path := toolHistoryPathForRoot(root)
	if err := os.MkdirAll(filepath.Dir(path), conventions.SecretDirPerm); err != nil {
		return fmt.Errorf("create tool history dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, conventions.SecretFilePerm)
	if err != nil {
		return fmt.Errorf("open tool history: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("append tool history: %w", err)
	}
	return f.Close()
}

// updateToolHistoryOutcomeForRoot promotes the admission entry for (opID,
// gen) under a project root to its terminal outcome and enforces the cap,
// evicting the oldest runs. It is the only path that rewrites the file, so
// cap overflow from crash-orphaned admissions self-heals on the next
// terminal rewrite; GetToolHistory additionally clamps its view.
// Residual cross-process races (two rewrites interleaving, or a rewrite
// racing an append) can only lose an entry or briefly exceed the cap, never
// corrupt a line: admissions are single O_APPEND writes and rewrites are
// atomic temp+rename publishes. No flock.
func updateToolHistoryOutcomeForRoot(root, opID string, gen uint64, outcome string) error {
	toolHistoryMu.Lock()
	defer toolHistoryMu.Unlock()
	path := toolHistoryPathForRoot(root)
	entries, err := readToolHistoryFile(path)
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].OpID == opID && entries[i].Gen == gen {
			entries[i].Outcome = outcome
		}
	}
	if len(entries) > maxToolHistoryEntries {
		entries = entries[len(entries)-maxToolHistoryEntries:]
	}
	return writeToolHistoryFile(path, entries)
}

// GetToolHistory returns the scrubbed per-project tool-run history, oldest
// first, capped at the most recent 50 runs. Unresolvable projects surface an
// error (never a misleading list); a project with no runs yields none.
//
// The ToolService method below is the Wails-bound surface for the palette;
// it delegates to this function.
func GetToolHistory(project string) ([]ToolHistoryEntry, error) {
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}
	toolHistoryMu.Lock()
	defer toolHistoryMu.Unlock()
	entries, err := readToolHistoryFile(toolHistoryPathForRoot(root))
	if err != nil {
		return nil, err
	}
	if len(entries) > maxToolHistoryEntries {
		entries = entries[len(entries)-maxToolHistoryEntries:]
	}
	return entries, nil
}

// GetToolHistory returns the scrubbed per-project tool-run history for the
// palette: each entry carries its (OpID, Gen) so the UI attributes terminal
// events idempotently to the current generation only.
func (s *ToolService) GetToolHistory(project string) (res []ToolHistoryEntry, err error) {
	defer RecoverPanic(&err, "GetToolHistory")
	return GetToolHistory(project)
}

// ToolHistoryPathForTest exposes the history file location for tests.
func ToolHistoryPathForTest(project string) string {
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return ""
	}
	return toolHistoryPathForRoot(root)
}
