package engine

// AuditRunJobs returns the default jobs for audit run --jobs when the flag is
// not explicitly set. It delegates to AuditJobs() (min(nproc,4) clamped 2-4)
// so phpstan 116s -> 40-60s auto-tuning stays centralized in lint.go.
func AuditRunJobs() int {
	return AuditJobs()
}
