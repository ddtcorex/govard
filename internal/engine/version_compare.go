package engine

import "strings"

// CompareNumericDotVersions compares two dot-separated numeric versions.
// It returns (comparison, true) when both values are comparable:
// comparison is 1 when left > right, -1 when left < right, and 0 when equal.
//
// The parse is deliberately lenient: each dot-separated segment contributes its
// leading digits and everything after them is discarded. "2.4.8-p3" compares as
// 2.4.8, and "8.4\n<8" compares as 8.4 (the "<8" after the newline is part of
// the last segment's tail). A segment that does not start with a digit, such as
// "x8.4", or an empty segment, makes the value not comparable. A missing
// segment counts as 0, so "8.4" equals "8.4.0".
func CompareNumericDotVersions(left, right string) (int, bool) {
	leftParts, ok := parseNumericDotVersion(left)
	if !ok {
		return 0, false
	}
	rightParts, ok := parseNumericDotVersion(right)
	if !ok {
		return 0, false
	}

	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}
	for i := 0; i < maxLen; i++ {
		lv := 0
		if i < len(leftParts) {
			lv = leftParts[i]
		}
		rv := 0
		if i < len(rightParts) {
			rv = rightParts[i]
		}
		if lv > rv {
			return 1, true
		}
		if lv < rv {
			return -1, true
		}
	}
	return 0, true
}

func IsNumericDotVersionAtLeast(raw string, minimum string) bool {
	comparison, comparable := CompareNumericDotVersions(raw, minimum)
	return comparable && comparison >= 0
}

// parseNumericDotVersion splits raw on dots and reads each segment with
// parseLeadingDigits, so the tail of every segment is discarded and only a
// segment with no leading digit, or an empty one, rejects the whole value.
func parseNumericDotVersion(raw string) ([]int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	segments := strings.Split(raw, ".")
	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, false
		}
		value, ok := parseLeadingDigits(segment)
		if !ok {
			return nil, false
		}
		parts = append(parts, value)
	}
	return parts, true
}

// parseLeadingDigits returns the integer formed by the leading digits of
// segment and ignores the first non-digit character and everything after it.
// It reports false when segment does not start with a digit.
func parseLeadingDigits(segment string) (int, bool) {
	value := 0
	seenDigit := false
	for _, r := range segment {
		if r >= '0' && r <= '9' {
			seenDigit = true
			value = value*10 + int(r-'0')
			continue
		}
		if !seenDigit {
			return 0, false
		}
		return value, true
	}
	if !seenDigit {
		return 0, false
	}
	return value, true
}
