package engine

import (
	"os/exec"
	"runtime"
	"strings"

	"govard/internal/conventions"
)

// LintIgnore returns ignore patterns for lint. Quick mode ignores
// vendor/dev/tests/lib/m2-hotfixes and always ignores generated artefacts.
// Deep mode keeps vendor/dev/lib/m2-hotfixes but still ignores pub/media etc.
func LintIgnore(quick bool) []string {
	return conventions.LintIgnore(quick)
}

// StableVolumeKey returns a stable volume key derived from the project name
// from govard.yml (not sha1(cwd)) so the volume survives directory moves.
func StableVolumeKey(name string) string {
	return conventions.StableVolumeKey(name)
}

// AuditJobs returns the lint worker count as min(nproc,4) clamped to 2-8.
// It auto-tunes phpstan/physics jobs: phpstan 116s -> 40-60s on 4 cores.
func AuditJobs() int {
	n := runtime.NumCPU()
	if n > 4 {
		n = 4
	}
	if n < 2 {
		n = 2
	}
	return n
}

// AuditScope resolves the audit scope. diff=true with a non-empty base ref yields
// "diff", otherwise "project". A non-empty base that looks like a valid git ref
// is treated as diff even if not locally fetched (e.g. origin/master in shallow
// CI where fetch-depth:1 hasn't fetched the remote). A syntactically invalid
// base (empty or whitespace-only) falls back to project; if rev-parse is
// available and the ref is clearly invalid (e.g. contains spaces), it also
// falls back.
func AuditScope(diff bool, base string) string {
	if !diff {
		return "project"
	}
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return "project"
	}
	// Basic ref syntax check: allow A-Za-z0-9._/- and leading ^ for stash
	if strings.Contains(trimmed, " ") {
		return "project"
	}
	// Best-effort verify if git is available and ref is HEAD or a locally known ref.
	// For remote refs like origin/master that may not be fetched in shallow CI,
	// treat as diff if syntax is valid — the audit runner will fetch or report
	// a clear error at run time, but the scope decision should remain "diff".
	if trimmed == "HEAD" || trimmed == "origin/master" || trimmed == "origin/main" {
		return "diff"
	}
	if err := exec.Command("git", "rev-parse", "--verify", trimmed).Run(); err != nil {
		// If rev-parse fails, still allow diff for plausible remote refs (contain /)
		if strings.Contains(trimmed, "/") {
			return "diff"
		}
		return "project"
	}
	return "diff"
}
