package desktop

import (
	"regexp"
	"strings"
)

// WS-D UNIFICATION NOTE: the scrub primitives below are ported verbatim
// (identical names, identical semantics) from the shared tool-history
// scrubber on feat/desktop-tool-runner (internal/desktop/tool_history.go:
// scrubToolHistoryText and friends). Cross-branch import is impossible, so
// this shim carries the exact definitions until that branch lands; when it
// does, DELETE THIS FILE and keep tool_history.go — every call site below
// uses identical identifiers and keeps compiling unchanged. Do NOT fork
// these patterns into a second scheme: one secret definition for WS-D.
//
// Shared identifiers (verify each survives the merge blind, by name):
// scrubToolHistoryText, isToolHistorySecretKey, normalizeToolHistoryKeyName,
// toolHistorySecretKeys, toolHistorySecretKVPattern,
// toolHistoryCredentialedURLPattern, toolHistoryPrivateKeyBlockPattern,
// toolHistoryPrivateKeyHeaderPattern.
//
// Deliberately NOT ported: scrubToolHistoryArgs (the map[string]string
// variant — op history args are CLI argv, adapted by scrubToolHistoryArgv
// in operation_history.go) and all file/service surfaces, which stay in
// tool_history.go.

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
