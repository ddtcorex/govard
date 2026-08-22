package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"govard/internal/audit"
	"govard/internal/engine"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// AuditRunOutcomeError reports an audit invocation that completed and persisted
// its result, but whose analysis did not pass. The command still renders its
// full human-readable or JSON summary first; this error only turns the outcome
// into a non-zero exit code so scripts and CI observe failed lint findings and
// cancelled runs instead of a silent success.
type AuditRunOutcomeError struct {
	SessionID string
	RunID     string
	Status    audit.Status
}

func (e *AuditRunOutcomeError) Error() string {
	subject := fmt.Sprintf("audit run %s/%s", e.SessionID, e.RunID)
	if e.Status == audit.StatusCancelled {
		return subject + " was cancelled before completion"
	}
	return subject + " reported failed checks"
}

// auditRunOutcome converts a finished run into the command-level error that
// fails the process when the outcome is anything but a clean pass.
func auditRunOutcome(result audit.RunResult) error {
	switch result.Status {
	case audit.StatusPassed, audit.StatusPending, audit.StatusRunning, audit.StatusNotComparable:
		return nil
	default:
		return &AuditRunOutcomeError{SessionID: result.SessionID, RunID: result.RunID, Status: result.Status}
	}
}

// auditTextRenderer writes the operator-facing text view of audit values. Every
// line goes straight to the command writer. Color is applied only when the
// destination is an interactive terminal and NO_COLOR is unset, so piped or
// redirected output stays free of escape codes.
type auditTextRenderer struct {
	writer io.Writer
	color  bool
}

const auditFieldWidth = 12

// auditColorEnabled reports whether the writer accepts terminal styling.
func auditColorEnabled(writer io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func (r auditTextRenderer) printf(format string, a ...any) {
	fmt.Fprintf(r.writer, format+"\n", a...)
}

func (r auditTextRenderer) blank() {
	fmt.Fprintln(r.writer)
}

func (r auditTextRenderer) header(text string) {
	r.blank()
	if r.color {
		bar := pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold)
		fmt.Fprintln(r.writer, bar.Sprint(" "+text+" "))
		return
	}
	fmt.Fprintf(r.writer, "== %s ==\n", text)
}

func (r auditTextRenderer) section(title string) {
	r.blank()
	if r.color {
		fmt.Fprintln(r.writer, pterm.NewStyle(pterm.Bold).Sprint("  "+title))
		return
	}
	r.printf("  %s", title)
}

func (r auditTextRenderer) field(label, value string) {
	if value == "" {
		return
	}
	r.printf("  %-*s %s", auditFieldWidth, label+":", value)
}

func (r auditTextRenderer) bullet(text string) {
	r.printf("    - %s", text)
}

func writeAuditText(writer io.Writer, value any) error {
	renderer := auditTextRenderer{writer: writer, color: auditColorEnabled(writer)}
	switch typed := value.(type) {
	case audit.RunResult:
		renderer.runResult(typed)
	case audit.SessionManifest:
		renderer.sessionManifest(typed)
	case auditToolchainStatusReport:
		renderer.toolchainStatus(typed)
	case auditToolchainIdentity:
		renderer.toolchainIdentity(typed)
	case map[string]any:
		renderer.removedSessions(typed)
	default:
		fmt.Fprintf(writer, "%+v\n", value)
	}
	return nil
}

func (r auditTextRenderer) runResult(result audit.RunResult) {
	r.header(fmt.Sprintf("Audit run %s / session %s", result.RunID, result.SessionID))
	r.field("Status", auditStatusText(result.Status, r.color))
	r.field("Scope", string(result.Scope))
	if duration := auditRunDurationMS(result); duration > 0 {
		r.field("Duration", auditHumanDuration(duration))
	}
	if env := auditEnvironmentSummary(result.Environment); env != "" {
		r.field("Environment", env)
	}
	if source := auditSourceSummary(result.Source); source != "" {
		r.field("Source", source)
	}
	r.checks(result.Jobs)
	r.errors(result.Errors)
	r.nextSteps(result)
}

func (r auditTextRenderer) checks(jobs []audit.JobResult) {
	if len(jobs) == 0 {
		return
	}
	r.section("Checks")
	for _, job := range jobs {
		line := job.ID + " - " + auditPlainStatus(string(job.Status))
		if job.DurationMS > 0 {
			line += " - " + auditHumanDuration(job.DurationMS)
		}
		if provider := auditEvidenceString(job.Evidence, "provider"); provider != "" {
			line += " - provider " + provider
		}
		if lintStatus := auditEvidenceString(job.Evidence, "lint_status"); lintStatus != "" && lintStatus != string(job.Status) {
			line += " - lint " + lintStatus
		}
		r.bullet(line)
		for _, php := range auditLintPHPViews(job.Evidence) {
			r.phpResult(php)
		}
	}
}

func (r auditTextRenderer) phpResult(php auditLintPHPView) {
	parts := []string{"PHP " + php.Version, auditPlainStatus(php.Outcome)}
	if php.DurationMS > 0 {
		parts = append(parts, auditHumanDuration(php.DurationMS))
	}
	cache := php.CacheState
	if cache == "" {
		cache = "unknown"
	}
	parts = append(parts, "cache "+cache, fmt.Sprintf("%d findings", len(php.Findings)))
	r.printf("      %s", strings.Join(parts, " | "))
	for index, finding := range php.Findings {
		if index == auditMaxDisplayedFindings {
			r.printf("      ... and %d more findings (see the full report under What next)", len(php.Findings)-index)
			break
		}
		r.printf("      - %s", finding.String())
	}
}

func (r auditTextRenderer) errors(errors []audit.AuditError) {
	if len(errors) == 0 {
		return
	}
	r.section("Errors")
	for _, issue := range errors {
		code := issue.Code
		if code == "" {
			code = "error"
		}
		r.bullet("[" + code + "] " + issue.Message)
	}
}

func (r auditTextRenderer) nextSteps(result audit.RunResult) {
	var steps []string
	report := auditReportPath(result.ProjectID, result.SessionID, result.RunID)
	switch result.Status {
	case audit.StatusFailed:
		steps = append(steps,
			"Full findings: "+report,
			"Re-run:        govard audit rerun --session "+result.SessionID)
	case audit.StatusCancelled:
		steps = append(steps,
			"The run stopped before completing.",
			"Re-run:        govard audit rerun --session "+result.SessionID,
			"Evidence:      "+report)
	}
	if len(steps) == 0 {
		return
	}
	r.section("What next")
	for _, step := range steps {
		r.printf("  %s", step)
	}
}

func (r auditTextRenderer) sessionManifest(manifest audit.SessionManifest) {
	r.header("Audit session " + manifest.SessionID)
	r.field("Project", manifest.ProjectID)
	if manifest.ProjectRoot != "" {
		r.field("Root", manifest.ProjectRoot)
	}
	scope := string(manifest.Scope)
	if manifest.BaseRef != "" {
		scope += " (base " + manifest.BaseRef + ")"
	}
	r.field("Scope", scope)
	if !manifest.CreatedAt.IsZero() {
		r.field("Created", auditTimestamp(manifest.CreatedAt))
	}
	if env := auditEnvironmentSummary(manifest.Environment); env != "" {
		r.field("Environment", env)
	}
	if source := auditSourceSummary(manifest.Source); source != "" {
		r.field("Source", source)
	}
	if len(manifest.Runs) == 0 {
		return
	}
	r.section("Runs")
	for _, run := range manifest.Runs {
		line := run.RunID
		if !run.CreatedAt.IsZero() {
			line += " - " + auditTimestamp(run.CreatedAt)
		}
		r.bullet(line)
	}
	r.printf("")
	r.printf("  Inspect a run:")
	r.printf("    govard audit result --session %s --run <run-id>", manifest.SessionID)
}

func (r auditTextRenderer) toolchainStatus(report auditToolchainStatusReport) {
	r.header("Lint toolchain")
	r.field("Provider", report.Provider)
	r.field("Present", auditYesNo(report.Present))
	if report.ContextDigest != "" {
		r.field("Context", report.ContextDigest)
	}
	if report.OfficialImage != "" {
		r.field("Official image", fmt.Sprintf("%s (usable: %s)", report.OfficialImage, auditYesNo(report.OfficialUsable)))
	}
	if report.LocalBuildImage != "" {
		r.field("Local image", fmt.Sprintf("%s (present: %s)", report.LocalBuildImage, auditYesNo(report.LocalBuildPresent)))
	}
	if report.Toolchain.Image != "" {
		r.toolchainIdentityLines(report.Toolchain)
	}
	if report.Repair != "" {
		r.blank()
		r.printf("  Fix: %s", report.Repair)
	}
}

func (r auditTextRenderer) toolchainIdentity(identity auditToolchainIdentity) {
	r.header("Lint toolchain identity")
	r.toolchainIdentityLines(identity)
}

func (r auditTextRenderer) toolchainIdentityLines(identity auditToolchainIdentity) {
	r.field("Image", identity.Image)
	if identity.ImageDigest != "" {
		r.field("Image digest", identity.ImageDigest)
	}
	if identity.ContextDigest != "" {
		r.field("Context digest", identity.ContextDigest)
	}
	r.field("Local build", auditYesNo(identity.LocalBuild))
}

func (r auditTextRenderer) removedSessions(payload map[string]any) {
	raw, _ := payload["removed_sessions"].([]string)
	ids := make([]string, 0, len(raw))
	ids = append(ids, raw...)
	r.header("Audit cleanup")
	if len(ids) == 0 {
		r.printf("  No sessions matched the requested age.")
		return
	}
	r.field("Removed", fmt.Sprintf("%d session(s)", len(ids)))
	for _, id := range ids {
		r.bullet(id)
	}
}

// auditMaxDisplayedFindings caps the per-PHP finding list so a broken module
// cannot flood the terminal; the provider report keeps the complete list.
const auditMaxDisplayedFindings = 10

type auditLintPHPView struct {
	Version     string
	Outcome     string
	DurationMS  int64
	CacheState  string
	CacheReason string
	Findings    []auditLintFindingView
}

type auditLintFindingView struct {
	Tool    string
	Rule    string
	Path    string
	Line    int
	Column  int
	Message string
}

func (f auditLintFindingView) String() string {
	location := f.Path
	if location == "" {
		location = "(unknown path)"
	}
	if f.Line > 0 {
		location += ":" + fmt.Sprint(f.Line)
		if f.Column > 0 {
			location += ":" + fmt.Sprint(f.Column)
		}
	}
	prefix := f.Tool
	if prefix == "" {
		prefix = "lint"
	}
	if f.Rule != "" {
		prefix += " " + f.Rule
	}
	message := f.Message
	if len(message) > 160 {
		message = message[:157] + "..."
	}
	if message == "" {
		return prefix + " " + location
	}
	return prefix + " " + location + ": " + message
}

// auditLintPHPViews normalizes the lint evidence carried by one job. A fresh
// run holds typed []audit.LintPHPResult values, while a result read back from
// the store decodes them into []any/map[string]any; both shapes collapse into
// the same view so run and result render identically.
func auditLintPHPViews(evidence map[string]any) []auditLintPHPView {
	if evidence == nil {
		return nil
	}
	if typed, ok := evidence["php_results"].([]audit.LintPHPResult); ok {
		views := make([]auditLintPHPView, 0, len(typed))
		for _, result := range typed {
			findings := make([]auditLintFindingView, 0, len(result.Findings))
			for _, finding := range result.Findings {
				findings = append(findings, auditLintFindingView{
					Tool: finding.Tool, Rule: finding.Rule, Path: finding.Path,
					Line: finding.Line, Column: finding.Column, Message: finding.Message,
				})
			}
			views = append(views, auditLintPHPView{
				Version:     result.PHPVersion,
				Outcome:     result.Outcome,
				DurationMS:  result.DurationMS,
				CacheState:  result.Cache.State,
				CacheReason: result.Cache.Reason,
				Findings:    findings,
			})
		}
		return views
	}
	raw, ok := evidence["php_results"].([]any)
	if !ok {
		return nil
	}
	views := make([]auditLintPHPView, 0, len(raw))
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		view := auditLintPHPView{
			Version:    auditMapString(item, "php_version"),
			Outcome:    auditMapString(item, "outcome"),
			DurationMS: auditMapInt64(item, "duration_ms"),
		}
		if cache, ok := item["cache"].(map[string]any); ok {
			view.CacheState = auditMapString(cache, "state")
			view.CacheReason = auditMapString(cache, "reason")
		}
		if findings, ok := item["findings"].([]any); ok {
			for _, raw := range findings {
				finding, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				view.Findings = append(view.Findings, auditLintFindingView{
					Tool:    auditMapString(finding, "tool"),
					Rule:    auditMapString(finding, "rule"),
					Path:    auditMapString(finding, "path"),
					Line:    int(auditMapInt64(finding, "line")),
					Column:  int(auditMapInt64(finding, "column")),
					Message: auditMapString(finding, "message"),
				})
			}
		}
		views = append(views, view)
	}
	return views
}

func auditMapString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func auditMapInt64(values map[string]any, key string) int64 {
	switch number := values[key].(type) {
	case int64:
		return number
	case int:
		return int64(number)
	case float64:
		return int64(number)
	default:
		return 0
	}
}

func auditEvidenceString(evidence map[string]any, key string) string {
	if evidence == nil {
		return ""
	}
	value, _ := evidence[key].(string)
	return value
}

func auditRunDurationMS(result audit.RunResult) int64 {
	var longest int64
	for _, job := range result.Jobs {
		if job.DurationMS > longest {
			longest = job.DurationMS
		}
	}
	if longest > 0 {
		return longest
	}
	for _, job := range result.Jobs {
		if duration := auditMapInt64(job.Evidence, "duration_ms"); duration > longest {
			longest = duration
		}
	}
	return longest
}

func auditHumanDuration(milliseconds int64) string {
	if milliseconds < 1000 {
		return fmt.Sprintf("%dms", milliseconds)
	}
	if milliseconds < 60000 {
		return fmt.Sprintf("%.1fs", float64(milliseconds)/1000)
	}
	duration := time.Duration(milliseconds) * time.Millisecond
	return fmt.Sprintf("%dm%ds", int(duration.Minutes()), int(duration.Seconds())%60)
}

func auditTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04 UTC")
}

func auditYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func auditPlainStatus(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

var auditStatusStyles = map[audit.Status]*pterm.Style{}

func init() {
	auditStatusStyles[audit.StatusPassed] = pterm.NewStyle(pterm.FgGreen, pterm.Bold)
	auditStatusStyles[audit.StatusFailed] = pterm.NewStyle(pterm.FgRed, pterm.Bold)
	auditStatusStyles[audit.StatusCancelled] = pterm.NewStyle(pterm.FgYellow, pterm.Bold)
	auditStatusStyles[audit.StatusPending] = pterm.NewStyle(pterm.FgGray)
	auditStatusStyles[audit.StatusRunning] = pterm.NewStyle(pterm.FgGray)
}

func auditStatusText(status audit.Status, color bool) string {
	text := strings.ToUpper(auditPlainStatus(string(status)))
	style, ok := auditStatusStyles[status]
	if !color || !ok || style == nil {
		return text
	}
	return style.Sprint(text)
}

func auditEnvironmentSummary(env audit.EnvironmentFingerprint) string {
	parts := make([]string, 0, 4)
	if env.Framework != "" {
		parts = append(parts, env.Framework)
	}
	if env.FrameworkVersion != "" {
		parts = append(parts, env.FrameworkVersion)
	}
	if env.WebServer != "" {
		parts = append(parts, env.WebServer)
	}
	if env.GovardVersion != "" {
		parts = append(parts, "Govard "+env.GovardVersion)
	}
	return strings.Join(parts, " | ")
}

func auditSourceSummary(source audit.SourceFingerprint) string {
	parts := make([]string, 0, 2)
	if source.GitCommit != "" {
		commit := source.GitCommit
		if source.GitDirty {
			commit += " (uncommitted changes)"
		} else {
			commit += " (clean)"
		}
		parts = append(parts, commit)
	}
	if source.Digest != "" {
		parts = append(parts, source.Digest)
	}
	return strings.Join(parts, " | ")
}

// auditReportPath points operators at the persisted provider report without
// reproducing any of its content in the terminal.
func auditReportPath(projectID, sessionID, runID string) string {
	root := audit.DefaultStoreRoot(engine.GovardHomeDir())
	return fmt.Sprintf("%s/%s/sessions/%s/runs/%s/report.json", root, projectID, sessionID, runID)
}
