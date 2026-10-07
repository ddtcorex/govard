package verify

import "fmt"

// GuardDecision is what the runner does with one item: run it, or leave it out
// of the run with a reason the operator can act on.
type GuardDecision struct {
	Run    bool
	Reason string
}

// DecideGuard is the one place Guard is read. A guarded item the current run may
// not perform is skipped with a reason the operator can act on.
//
// phase is the phase being run; 0 means every phase, so an item is judged by its
// own phase — not by "is this run outside phase 5".
func DecideGuard(it Item, phase int, opts VerifyOpts) GuardDecision {
	switch it.Guard {
	case GuardRemoteWrite:
		if opts.AllowRemoteWrite {
			return GuardDecision{Run: true}
		}
		return GuardDecision{Reason: fmt.Sprintf(
			"writes through a remote: run `%s` manually, or pass --allow-remote-write to attempt it here; verify's children have no tty, so the row carries its own confirmation flag (-y or --yes) and runs unattended",
			it.Title)}
	case GuardDestructiveLocal:
		runIn := phase
		if runIn == 0 {
			runIn = it.Phase
		}
		if runIn != 5 {
			return GuardDecision{Reason: fmt.Sprintf(
				"destructive local item: only a phase-5 run performs it (this run resolved to phase %d)", runIn)}
		}
		return GuardDecision{Run: true}
	default:
		return GuardDecision{Run: true}
	}
}
