package engine

// AuditRunJobs returns the default jobs for audit run --jobs when the flag is
// not explicitly set. It delegates to AuditJobs() (min(nproc,4) clamped 2-4)
// so phpstan 116s -> 40-60s auto-tuning stays centralized in lint.go.
func AuditRunJobs() int {
	return AuditJobs()
}

// AuditRunScope resolves --scope diff --base handling for audit run. It
// delegates to AuditScope(diff, base) which validates the base ref via
// git rev-parse --verify and falls back to "project" on empty/invalid base.
// This keeps scope validation centralized while giving audit run a dedicated
// entry point for wiring flags.
func AuditRunScope(diff bool, base string) string {
	return AuditScope(diff, base)
}
