package generator

import "sort"

// PriorityOverrides declares framework packages that must register before
// another framework whose detection signature could otherwise match
// first. The only known case: a project satisfying both Emdash's and
// Next.js's Detect signature must resolve to Emdash (the legacy
// pre-registry detector's tie-break) - see
// internal/frameworks/emdash/emdash.go's doc comment for why. Lower
// values register earlier; packages absent from this map default to
// priority 0 and are ordered alphabetically among themselves. Add a new
// entry here (not to internal/frameworks/all_generated.go, which is
// generated) if a future framework introduces another ambiguous-match
// case.
var PriorityOverrides = map[string]int{
	"emdash": -1,
}

// OrderByPriority returns a new slice with names sorted by overrides
// (ascending), then alphabetically among equal priorities - the order
// all_generated.go's init() calls Register() in. Does not mutate names.
func OrderByPriority(names []string, overrides map[string]int) []string {
	ordered := make([]string, len(names))
	copy(ordered, names)
	sort.SliceStable(ordered, func(i, j int) bool {
		pi, pj := overrides[ordered[i]], overrides[ordered[j]]
		if pi != pj {
			return pi < pj
		}
		return ordered[i] < ordered[j]
	})
	return ordered
}
