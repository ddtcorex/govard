package engine

import "testing"

func TestCompareNumericDotVersionsDiscardsTail(t *testing.T) {
	cases := []struct {
		name        string
		left, right string
	}{
		{"newline and operator tail", "8.4\n<8", "8.4"},
		{"patch suffix", "2.4.8-p3", "2.4.8"},
		{"prerelease suffix", "8.3.0RC1", "8.3.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comparison, ok := CompareNumericDotVersions(tc.left, tc.right)
			if !ok || comparison != 0 {
				t.Fatalf("CompareNumericDotVersions(%q, %q) = (%d, %v), want (0, true)", tc.left, tc.right, comparison, ok)
			}
		})
	}
}

func TestCompareNumericDotVersionsRejectsLeadingGarbage(t *testing.T) {
	for _, raw := range []string{"x8.4", "8.x", "8..4", "", "v2.4.8"} {
		if comparison, ok := CompareNumericDotVersions(raw, "8.4"); ok {
			t.Errorf("CompareNumericDotVersions(%q, \"8.4\") = (%d, true), want not comparable", raw, comparison)
		}
	}
}
