package desktop

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

// OpMessage is the human failure message for one desktop operation: an
// actionable title plus a hint, with the buttons the toast renders. WS-D/E
// history entries reuse this shape instead of raw CLI stderr.
type OpMessage struct {
	Title   string            `json:"title"`
	Hint    string            `json:"hint"`
	Actions []OpMessageAction `json:"actions"`
}

// OpMessageAction is one toast button: the id the caller switches on and
// the label the toast renders.
type OpMessageAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Toast action ids shared by the Go catalog and the toast renderer.
const (
	OpActionRetry    = "retry"
	OpActionOpenLogs = "open-logs"
	OpActionCopy     = "copy"
)

// opMessageHintMax caps the fallback hint at the same width as the toast
// body so a verbose CLI error never becomes a wall of text.
const opMessageHintMax = 280

// opDisplayNames gives each catalogued op its human headline name. Unknown
// ops fall back to a capitalized passthrough so a new caller still reads
// sensibly while its failures stay in the sanitized fallback.
var opDisplayNames = map[string]string{
	"sync":         "Sync",
	"onboard":      "Onboarding",
	"up":           "Environment start",
	"pull":         "Pull",
	"restart":      "Environment restart",
	"rollback":     "Rollback",
	"sandbox-up":   "Sandbox start",
	"sandbox-down": "Sandbox removal",
}

// opFailureSeed is one known failure shape: the markers matched
// case-insensitively against the error text, the title suffix, the
// actionable hint, and the buttons the toast renders.
type opFailureSeed struct {
	titleSuffix string
	hint        string
	actions     []OpMessageAction
	markers     []string
}

func retryActions() []OpMessageAction {
	return []OpMessageAction{{ID: OpActionRetry, Label: "Retry"}, {ID: OpActionOpenLogs, Label: "Open logs"}}
}

func logsActions() []OpMessageAction {
	return []OpMessageAction{{ID: OpActionOpenLogs, Label: "Open logs"}}
}

var opFailureSeeds = []opFailureSeed{
	{
		titleSuffix: "timed out",
		hint:        "The remote took too long to answer. Check the connection and retry.",
		markers:     []string{"timed out", "timeout", "deadline exceeded"},
		actions:     retryActions(),
	},
	{
		titleSuffix: "could not reach the remote",
		hint:        "The remote host refused the connection or cannot be resolved. Check the address and network, then retry.",
		markers: []string{
			"connection refused", "no such host", "could not resolve",
			"network is unreachable", "no route to host", "connection reset",
			"dial tcp", "connection broken", "broken pipe",
		},
		actions: retryActions(),
	},
	{
		titleSuffix: "was denied access",
		hint:        "The remote rejected the credentials. Check the user, key, and permissions, then retry.",
		markers: []string{
			"permission denied", "access denied", "unauthorized",
			"authentication failed", "publickey", "password authentication",
		},
		actions: logsActions(),
	},
	{
		titleSuffix: "needs Docker running",
		hint:        "The container runtime is unreachable. Start Docker and retry.",
		markers: []string{
			"cannot connect to the docker daemon", "docker daemon",
			"docker: ", "no such container", "container failed to start",
		},
		actions: logsActions(),
	},
	{
		titleSuffix: "ran out of disk space",
		hint:        "The disk is full. Free space and retry.",
		markers:     []string{"no space left", "disk full", "out of disk"},
		actions:     logsActions(),
	},
}

// humanOpMessage maps one operation failure to its human message. Known
// failure shapes become an actionable title plus Retry/Open-logs; anything
// else falls back to the sanitized, secret-scrubbed raw error as the hint
// with a Copy button. Every string passes through sanitizeUIText: error text can carry
// ANSI escapes and control bytes from CLI stderr, and the payload reaches
// toasts as well as history entries.
func humanOpMessage(op string, err error) OpMessage {
	if err == nil {
		return OpMessage{}
	}
	display := opDisplayName(op)
	// Only the stdlib sentinel counts as a deliberate cancel, so an error
	// that merely mentions "cancelled" in prose still hits the sanitized
	// fallback instead of being misread.
	if errors.Is(err, context.Canceled) {
		return OpMessage{
			Title:   sanitizeUIText(display+" was cancelled", 0),
			Hint:    sanitizeUIText("The operation was cancelled before it finished.", 0),
			Actions: logsActions(),
		}
	}
	raw := err.Error()
	lowered := strings.ToLower(raw)
	for _, seed := range opFailureSeeds {
		if containsOpMarker(lowered, seed.markers) {
			return OpMessage{
				Title:   sanitizeUIText(display+" "+seed.titleSuffix, 0),
				Hint:    sanitizeUIText(seed.hint, 0),
				Actions: seed.actions,
			}
		}
	}
	return OpMessage{
		Title: sanitizeUIText(display+" failed", 0),
		Hint:  sanitizeUIText(scrubToolHistoryText(raw), opMessageHintMax),
		Actions: []OpMessageAction{
			{ID: OpActionCopy, Label: "Copy"},
			{ID: OpActionOpenLogs, Label: "Open logs"},
		},
	}
}

func containsOpMarker(lowered string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// opDisplayName resolves the headline name for op, sanitized so a hostile
// or empty caller value can never inject markup into the title.
func opDisplayName(op string) string {
	if display, ok := opDisplayNames[strings.ToLower(strings.TrimSpace(op))]; ok {
		return display
	}
	cleaned := sanitizeUIText(strings.TrimSpace(op), 0)
	if cleaned == "" {
		return "Operation"
	}
	runes := []rune(cleaned)
	return string(unicode.ToUpper(runes[0])) + string(runes[1:])
}
