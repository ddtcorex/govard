package tests

import (
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

func TestVerifyRegistryCounts(t *testing.T) {
	if got := len(verify.Registry); got != 56 {
		t.Fatalf("Registry length = %d, want 56", got)
	}

	phaseCounts := map[int]int{}
	ids := map[string]bool{}
	allowedGuards := map[string]bool{
		"":                  true,
		"READ-ONLY-REMOTE":  true,
		"DESTRUCTIVE-LOCAL": true,
	}

	for _, item := range verify.Registry {
		phaseCounts[item.Phase]++
		if ids[item.ID] {
			t.Fatalf("duplicate ID %q", item.ID)
		}
		ids[item.ID] = true
		if !allowedGuards[item.Guard] {
			t.Fatalf("item %q has invalid Guard %q, want one of \"\", \"READ-ONLY-REMOTE\", \"DESTRUCTIVE-LOCAL\"", item.ID, item.Guard)
		}
		// When is nil or func — ensure calling does not panic
		if item.When != nil {
			// exercise with empty config and with magento2 config
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("item %q When panicked: %v", item.ID, r)
					}
				}()
				_ = item.When(engine.Config{})
				_ = item.When(engine.Config{Framework: "magento2"})
			}()
		}
	}

	wantCounts := map[int]int{1: 7, 2: 14, 3: 15, 4: 12, 5: 8}
	for phase, want := range wantCounts {
		if got := phaseCounts[phase]; got != want {
			t.Fatalf("phase %d count = %d, want %d", phase, got, want)
		}
	}
	// Ensure no unexpected phases
	for phase := range phaseCounts {
		if _, ok := wantCounts[phase]; !ok {
			t.Fatalf("unexpected phase %d found", phase)
		}
	}
}
