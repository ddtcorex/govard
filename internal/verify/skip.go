package verify

// Skip reports an item that exists but does not apply in this run. A skipped
// item is not a failure: it keeps its row in the artifact so an operator can
// see that the checklist accounted for it and why it did not run.
func Skip(reason string) Evidence {
	return Evidence{Skipped: true, SkipReason: reason}
}
