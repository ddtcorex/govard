package desktop

import (
	"regexp"
	"strings"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var orphanAnsiStylePattern = regexp.MustCompile(`\[(?:\d{1,3}(?:;\d{1,3})*)m`)

// truncatedMarker suffixes UI-bound text cut to its display cap so a
// clipped toast, status line, or progress message never reads as complete.
const truncatedMarker = "…(truncated)"

// sanitizeUIText prepares s for UI-bound surfaces (toasts, status lines,
// progress messages). It strips ANSI escape sequences and orphan style
// fragments, drops control bytes except newline and tab, coerces invalid
// UTF-8, and truncates over-long input with a truncation marker so the
// total stays within max runes. A max <= 0 disables truncation.
func sanitizeUIText(s string, max int) string {
	if s == "" {
		return ""
	}

	clean := strings.ToValidUTF8(s, "")
	clean = ansiEscapePattern.ReplaceAllString(clean, "")
	clean = orphanAnsiStylePattern.ReplaceAllString(clean, "")

	var builder strings.Builder
	builder.Grow(len(clean))
	for _, r := range clean {
		if r == '\n' || r == '\t' {
			builder.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		builder.WriteRune(r)
	}
	clean = builder.String()

	if max > 0 {
		runes := []rune(clean)
		if len(runes) > max {
			marker := []rune(truncatedMarker)
			if max <= len(marker) {
				return string(marker[:max])
			}
			return string(runes[:max-len(marker)]) + truncatedMarker
		}
	}
	return clean
}
