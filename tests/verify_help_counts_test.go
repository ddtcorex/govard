package tests

import (
	"regexp"
	"strconv"
	"testing"

	"govard/internal/cmd"
	"govard/internal/verify"
)

// The help text is the user-visible summary of the registry. It has drifted
// before — it still advertised 12 phase-4 items after the registry grew to 16 —
// so it is compared against the registry instead of being trusted.
func TestVerifyHelpPhaseCountsMatchRegistry(t *testing.T) {
	long := cmd.VerifyCommandForTest().Long
	matches := regexp.MustCompile(`([1-5]) ([A-Za-z /&]+)\((\d+)\)`).FindAllStringSubmatch(long, -1)
	if len(matches) != 5 {
		t.Fatalf("parsed %d phase entries from the verify help text, want 5:\n%s", len(matches), long)
	}

	want := map[int]int{}
	for _, item := range verify.Registry {
		want[item.Phase]++
	}

	for _, m := range matches {
		phase, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("phase %q: %v", m[1], err)
		}
		got, err := strconv.Atoi(m[3])
		if err != nil {
			t.Fatalf("count %q: %v", m[3], err)
		}
		if got != want[phase] {
			t.Errorf("help text says phase %d has %d items, the registry has %d", phase, got, want[phase])
		}
	}
}
