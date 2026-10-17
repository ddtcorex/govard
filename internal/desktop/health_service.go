package desktop

import (
	"path/filepath"
	"strings"
)

// HealthService methods live here; the struct itself is declared in
// services.go next to the other bound services. Every method delegates to
// the binding-free package functions in doctor_service.go and audit_service.go,
// so the logic stays testable without Wails; this file only resolves the
// project scope and owns the bindings surface the frontend routes reach.

// resolveHealthProjectRoot maps the frontend's project argument (a name,
// domain, or path, as every other desktop service accepts) to the directory
// doctor fixes and audit runs scope to. An empty argument keeps the legacy
// process-working-directory behavior. A resolution that is not a directory
// (an orphan compose name, a weak fuzzy match) falls back the same way:
// there is no project directory to scope to, and the package functions
// already fail honestly on missing config.
func resolveHealthProjectRoot(project string) (string, error) {
	trimmed := strings.TrimSpace(project)
	if trimmed == "" {
		return "", nil
	}
	root, _, err := resolveProjectRootForRemotes(trimmed)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) {
		return "", nil
	}
	return root, nil
}

func (s *HealthService) RunDoctor(project string) (res DoctorReportView, err error) {
	defer RecoverPanic(&err, "RunDoctor")
	root, err := resolveHealthProjectRoot(project)
	if err != nil {
		return DoctorReportView{}, err
	}
	// A non-empty project that resolves to no directory is refused with
	// select guidance (same rule as the fix path): a working-directory run
	// bucketed under a project key would misattribute host state to the
	// project. An empty project keeps the legacy host-level run.
	if root == "" && strings.TrimSpace(project) != "" {
		return DoctorReportView{}, doctorErrorf("doctor run needs a project directory: %q did not resolve to one (select a project in the desktop app first)", strings.TrimSpace(project))
	}
	return RunDoctorForProject(root)
}

// resolveHealthFixRoot resolves the project scope for a doctor fix. A
// project-touching fix with no usable scope is a hard error, never a silent
// working-directory fallback: an empty project means nothing is selected
// (guided to select one), while a non-empty project that resolves to no
// directory names the argument that failed to resolve. Host-global fixes
// skip the scope entirely and may run explicitly unscoped.
func resolveHealthFixRoot(project string, checkID string) (string, error) {
	root, err := resolveHealthProjectRoot(project)
	if err != nil {
		return "", err
	}
	if root == "" && doctorFixNeedsProjectScope(checkID) && strings.TrimSpace(project) != "" {
		return "", doctorErrorf("doctor fix for %s needs a project directory: %q did not resolve to one (host-global fixes may run without a project)", strings.TrimSpace(checkID), strings.TrimSpace(project))
	}
	return root, nil
}

func (s *HealthService) PreviewDoctorFix(project string, checkID string) (res DoctorFixPreview, err error) {
	defer RecoverPanic(&err, "PreviewDoctorFix")
	root, err := resolveHealthFixRoot(project, checkID)
	if err != nil {
		return DoctorFixPreview{}, err
	}
	return PreviewDoctorFixForProject(checkID, root)
}

func (s *HealthService) ApplyDoctorFix(project string, checkID string, previewID string, confirm bool) (res DoctorFixOutcome, err error) {
	defer RecoverPanic(&err, "ApplyDoctorFix")
	root, err := resolveHealthFixRoot(project, checkID)
	if err != nil {
		return DoctorFixOutcome{}, err
	}
	return ApplyDoctorFixForProject(checkID, previewID, confirm, root)
}

func (s *HealthService) CancelDoctorRun(opID string) bool {
	return CancelDoctorRun(opID)
}

func (s *HealthService) RunIntegrityAudit(project string) (res AuditResultView, err error) {
	defer RecoverPanic(&err, "RunIntegrityAudit")
	root, err := resolveHealthProjectRoot(project)
	if err != nil {
		return AuditResultView{}, err
	}
	// A non-empty project that resolves to no directory is refused with
	// select guidance instead of silently auditing the process working
	// directory (same hard-error rule as the fix path). An empty project
	// keeps the legacy working-directory run.
	if root == "" && strings.TrimSpace(project) != "" {
		return AuditResultView{}, auditErrorf("integrity audit needs a project directory: %q did not resolve to one (select a project in the desktop app first)", strings.TrimSpace(project))
	}
	return RunIntegrityAudit(s.platform, root)
}

func (s *HealthService) GetAuditResult(projectID string, sessionID string, runID string) (res AuditResultView, err error) {
	defer RecoverPanic(&err, "GetAuditResult")
	return GetAuditResult(projectID, sessionID, runID)
}

func (s *HealthService) GetAuditDiff(projectID string, sessionID string, fromRunID string, toRunID string) (res AuditDiffView, err error) {
	defer RecoverPanic(&err, "GetAuditDiff")
	return GetAuditDiff(projectID, sessionID, fromRunID, toRunID)
}

func (s *HealthService) LastAuditResult(project string) (res AuditResultView, err error) {
	defer RecoverPanic(&err, "LastAuditResult")
	root, err := resolveHealthProjectRoot(project)
	if err != nil {
		return AuditResultView{}, err
	}
	view, ok := LastAuditResult(root)
	if !ok {
		return AuditResultView{}, auditErrorf("no completed integrity audit yet for this project")
	}
	return view, nil
}

func (s *HealthService) CancelAudit(opID string) bool {
	return CancelAudit(opID)
}

// RunningAuditOp reports the id of the in-flight integrity audit for a
// project scope, or "" when none is running. The health panel queries it on
// mount so Cancel is available even when the run started before the panel
// did (the audit:progress events only feed ops that start after subscribe).
func (s *HealthService) RunningAuditOp(project string) (res string, err error) {
	defer RecoverPanic(&err, "RunningAuditOp")
	root, err := resolveHealthProjectRoot(project)
	if err != nil {
		return "", err
	}
	return RunningAuditOpIDForProject(root), nil
}
