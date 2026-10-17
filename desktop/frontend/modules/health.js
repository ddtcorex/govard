/**
 * Health UX (doctor + audit) derivations and controller. The GlobalHealthHeader
 * banners and the HealthPanels island render from the store slices this
 * controller publishes; every banner derives from the latest results, never
 * from cached state, so a clean re-run clears stale banners.
 *
 * Per-project keying: the doctor/audit slices are keyed by normalized scope
 * (the selected project), and banners always derive from the current scope's
 * entry only. Switching projects re-derives that project's banners and shows
 * loading/empty meanwhile, never another project's banner.
 *
 * Preview tokens live in module-local ephemeral controller state, never in
 * the shared store: they are minted per preview, consumed on apply, and
 * dropped on close or project switch.
 *
 * Wire shape (lowerCamel over the Wails bridge; upperCamel also accepted):
 * DoctorReportView {checks[], passed, warnings, failures, opId},
 * DoctorCheckView {id, title, status, message, hint, suggestedCommand,
 * fixKind: safe|risky|none, fixLabel, needsPreview},
 * AuditResultView {projectId, sessionId, runId, status, findingCount,
 * checks[], findings[], opId},
 * AuditCheckView {id, title, enabled, disabledReason}.
 */

import { sanitizeUIText } from "../utils/sanitize.js";

// UI text caps mirror the Go side (doctorUITitleMax etc.): backend strings
// arrive sanitized already; this second pass keeps a hostile or over-long
// value from breaking layout even if it bypassed the service.
const HEALTH_ID_MAX = 120;
const HEALTH_TITLE_MAX = 200;
const HEALTH_PATH_MAX = 280;
const HEALTH_MESSAGE_MAX = 500;
const HEALTH_STATUS_MAX = 40;

const pick = (obj, lower, upper, fallback = "") => {
  const value = obj?.[lower] ?? obj?.[upper] ?? fallback;
  return value;
};

/**
 * The fix affordance for one doctor check: safe fixes are 1-click, risky
 * fixes are preview-gated, fixKind none (or unknown) shows no fix control
 * at all.
 * @param {any} check
 */
export function doctorFixControl(check = {}) {
  switch (String(pick(check, "fixKind", "FixKind", "none"))) {
    case "safe":
      return { kind: "fix", label: "Fix" };
    case "risky":
      return { kind: "preview", label: "Preview fix" };
    default:
      return { kind: "none", label: "" };
  }
}

/**
 * One sanitized doctor check for render.
 * @param {any} check
 */
export function normalizeDoctorCheck(check = {}) {
  return {
    id: sanitizeUIText(pick(check, "id", "ID"), HEALTH_ID_MAX),
    title: sanitizeUIText(pick(check, "title", "Title"), HEALTH_TITLE_MAX),
    status: sanitizeUIText(pick(check, "status", "Status"), HEALTH_STATUS_MAX),
    message: sanitizeUIText(pick(check, "message", "Message"), HEALTH_MESSAGE_MAX),
    hint: sanitizeUIText(pick(check, "hint", "Hint"), HEALTH_MESSAGE_MAX),
    suggestedCommand: sanitizeUIText(
      pick(check, "suggestedCommand", "SuggestedCommand"),
      HEALTH_MESSAGE_MAX,
    ),
    fixKind: String(pick(check, "fixKind", "FixKind", "none")),
    fixLabel: sanitizeUIText(pick(check, "fixLabel", "FixLabel"), HEALTH_TITLE_MAX),
    needsPreview: Boolean(pick(check, "needsPreview", "NeedsPreview", false)),
  };
}

/**
 * One sanitized doctor report for render.
 * @param {any} report
 */
export function normalizeDoctorReport(report = {}) {
  const raw = report?.checks ?? report?.Checks ?? [];
  const checks = Array.isArray(raw) ? raw.map(normalizeDoctorCheck) : [];
  return {
    checks,
    passed: Number(pick(report, "passed", "Passed", 0)) || 0,
    warnings: Number(pick(report, "warnings", "Warnings", 0)) || 0,
    failures: Number(pick(report, "failures", "Failures", 0)) || 0,
    opId: sanitizeUIText(pick(report, "opId", "OpID"), HEALTH_ID_MAX),
  };
}

/**
 * The doctor banner derives from the latest report only: a report with no
 * failing or warning check yields null, which is what clears a stale banner
 * on the next clean run. Failures tone error, warnings-only tone warning.
 * @param {any} report
 */
export function deriveDoctorBanner(report) {
  if (!report) {
    return null;
  }
  const normalized =
    Array.isArray(report.checks) && report.checks.length > 0
      ? report
      : normalizeDoctorReport(report);
  const actionable = (normalized.checks || []).filter(
    (check) => String(check.status || "") !== "pass",
  );
  if (actionable.length === 0) {
    return null;
  }
  const failures = actionable.filter((check) => String(check.status || "") === "fail");
  return {
    tone: failures.length > 0 ? "error" : "warning",
    total: actionable.length,
    failures: failures.length,
    actionable,
    fixableCount: actionable.filter((check) => doctorFixControl(check).kind !== "none").length,
  };
}

/**
 * One sanitized audit check row for render. Docker-backed rows stay in the
 * list with enabled false and their reason; they are never hidden.
 * @param {any} check
 */
export function normalizeAuditCheck(check = {}) {
  return {
    id: sanitizeUIText(pick(check, "id", "ID"), HEALTH_ID_MAX),
    title: sanitizeUIText(pick(check, "title", "Title"), HEALTH_TITLE_MAX),
    enabled: Boolean(pick(check, "enabled", "Enabled", false)),
    disabledReason: sanitizeUIText(
      pick(check, "disabledReason", "DisabledReason"),
      HEALTH_MESSAGE_MAX,
    ),
  };
}

/**
 * One sanitized audit finding for render.
 * @param {any} finding
 */
export function normalizeAuditFinding(finding = {}) {
  return {
    tool: sanitizeUIText(pick(finding, "tool", "Tool"), HEALTH_ID_MAX),
    rule: sanitizeUIText(pick(finding, "rule", "Rule"), HEALTH_ID_MAX),
    path: sanitizeUIText(pick(finding, "path", "Path"), HEALTH_PATH_MAX),
    line: Number(pick(finding, "line", "Line", 0)) || 0,
    message: sanitizeUIText(pick(finding, "message", "Message"), HEALTH_MESSAGE_MAX),
  };
}

/**
 * One sanitized audit result for render.
 * @param {any} result
 */
export function normalizeAuditResult(result = {}) {
  const rawChecks = result?.checks ?? result?.Checks ?? [];
  const rawFindings = result?.findings ?? result?.Findings ?? [];
  return {
    projectId: sanitizeUIText(pick(result, "projectId", "ProjectID"), HEALTH_ID_MAX),
    sessionId: sanitizeUIText(pick(result, "sessionId", "SessionID"), HEALTH_ID_MAX),
    runId: sanitizeUIText(pick(result, "runId", "RunID"), HEALTH_ID_MAX),
    status: sanitizeUIText(pick(result, "status", "Status"), HEALTH_STATUS_MAX),
    findingCount: Number(pick(result, "findingCount", "FindingCount", 0)) || 0,
    checks: Array.isArray(rawChecks) ? rawChecks.map(normalizeAuditCheck) : [],
    findings: Array.isArray(rawFindings) ? rawFindings.map(normalizeAuditFinding) : [],
    opId: sanitizeUIText(pick(result, "opId", "OpID"), HEALTH_ID_MAX),
  };
}

/**
 * The docker-backed rows of an audit result: present but disabled with a
 * reason, never hidden.
 * @param {any} result
 */
export function auditDisabledRows(result = {}) {
  const normalized =
    Array.isArray(result?.checks) && result.checks.length > 0
      ? result
      : normalizeAuditResult(result);
  return (normalized.checks || []).filter((check) => check.enabled === false);
}

/**
 * The audit banner derives from the latest result only: no result (never
 * ran) and a clean passed result both yield null, so fixed issues clear on
 * the next run instead of lingering.
 * @param {any} result
 */
export function deriveAuditBanner(result) {
  if (!result) {
    return null;
  }
  const normalized =
    typeof result.runId === "string" && result.findings !== undefined
      ? result
      : normalizeAuditResult(result);
  if (!normalized.runId) {
    return null;
  }
  const count = Number(normalized.findingCount || 0);
  if (String(normalized.status || "") === "passed" && count === 0) {
    return null;
  }
  return {
    tone: "warning",
    status: normalized.status,
    findingCount: count,
    sessionId: normalized.sessionId,
    runId: normalized.runId,
    disabled: (normalized.checks || []).filter((check) => check.enabled === false),
  };
}

/**
 * The store key for one project's health bucket: trimmed with trailing
 * slashes stripped, so trivial spelling differences share one bucket.
 * @param {any} project
 */
export function normalizeHealthScope(project) {
  return String(project || "").trim().replace(/\/+$/, "");
}

const emptyDoctorEntry = () => ({ report: null, loading: false, error: "" });
const emptyAuditEntry = () => ({ result: null, loading: false, error: "", opId: "", phase: "" });

/**
 * The doctor entry for the store's current scope, or an empty entry when
 * this scope never ran. Islands and banners read through here so a scope
 * with no data renders loading/empty, never another scope's report.
 * @param {any} state
 */
export function selectDoctorEntry(state) {
  const slice = state?.healthDoctor || {};
  const entry = slice.byProject?.[slice.scope];
  return { ...emptyDoctorEntry(), ...entry };
}

/**
 * The audit entry for the store's current scope. Same rule as the doctor
 * selector: no data renders empty, never another scope's result.
 * @param {any} state
 */
export function selectAuditEntry(state) {
  const slice = state?.healthAudit || {};
  const entry = slice.byProject?.[slice.scope];
  return { ...emptyAuditEntry(), ...entry };
}

/**
 * Points both health slices at one scope, creating its empty entries when
 * this scope never ran. The standalone seam the islands' project-switch path
 * and the controller share. This is the ONLY place that moves the scope
 * pointer: every other write stores data under its captured key and leaves
 * scope alone.
 */
export function selectHealthProjectForTest(setState, getState, project) {
  const key = normalizeHealthScope(project);
  const doctorSlice = getState().healthDoctor || {};
  const doctorByProject = { ...(doctorSlice.byProject || {}) };
  if (!doctorByProject[key]) {
    doctorByProject[key] = emptyDoctorEntry();
  }
  setState({ healthDoctor: { ...doctorSlice, byProject: doctorByProject, scope: key } });
  const auditSlice = getState().healthAudit || {};
  const auditByProject = { ...(auditSlice.byProject || {}) };
  if (!auditByProject[key]) {
    auditByProject[key] = emptyAuditEntry();
  }
  setState({ healthAudit: { ...auditSlice, byProject: auditByProject, scope: key } });
  return key;
}

export const initialHealthState = () => ({
  healthDoctor: { byProject: {}, scope: "" },
  healthAudit: { byProject: {}, scope: "" },
  healthPreview: { open: false, checkId: "", summary: "", actions: [] },
});

const shortError = (err) =>
  sanitizeUIText(String(err instanceof Error ? err.message : err), HEALTH_MESSAGE_MAX);

/**
 * The pending preview token for the confirm path, read off a controller.
 * Test seam: production reads it through the controller's pendingPreview
 * method (main.js confirm path); the token itself never enters the store.
 */
export function pendingPreviewForTest(controller) {
  return controller.pendingPreview();
}

/**
 * @param {{bridge: any, getState: () => any, setState: (patch: any) => void, onStatus: (msg: string) => void, onToast: (msg: string, tone: string) => void}} deps
 */
export function createHealthController({ bridge, getState, setState, onStatus, onToast }) {
  // The pending preview token lives here, in module-local ephemeral state:
  // previewFix mints it, applyFix consumes it, closePreview and project
  // switches drop it. It is never written to the shared store.
  let pending = { checkId: "", previewId: "" };
  // In-flight audit op ids by scope: progress events carry no project, so
  // the controller remembers which scope each op belongs to.
  const auditOpScopes = {};

  const scopeKey = (project) => normalizeHealthScope(project);

  // Data writes never move the scope pointer: every async completion
  // (runDoctor/runAudit/refreshBanner/recoverAuditOp/handleAuditProgress/
  // cancelAudit) stores its payload under the captured key only. ONLY an
  // explicit user project-switch (selectHealthProjectForTest below) moves
  // scope, so a late completion from an old scope can never yank the
  // pointer back during rapid A-B-A switches.
  const writeDoctor = (key, patch) => {
    const slice = getState().healthDoctor || {};
    const byProject = { ...(slice.byProject || {}) };
    byProject[key] = { ...emptyDoctorEntry(), ...byProject[key], ...patch };
    setState({ healthDoctor: { ...slice, byProject } });
  };

  const writeAudit = (key, patch) => {
    const slice = getState().healthAudit || {};
    const byProject = { ...(slice.byProject || {}) };
    byProject[key] = { ...emptyAuditEntry(), ...byProject[key], ...patch };
    setState({ healthAudit: { ...slice, byProject } });
  };

  const pendingPreview = () => ({ ...pending });

  const runDoctor = async (project, { silent = false } = {}) => {
    const key = scopeKey(project);
    if (!silent) {
      writeDoctor(key, { loading: true, error: "" });
    }
    try {
      const report = normalizeDoctorReport(await bridge.runDoctor());
      writeDoctor(key, { report, loading: false, error: "" });
      const banner = deriveDoctorBanner(report);
      if (!silent) {
        onStatus(
          banner
            ? `Doctor: ${report.failures} failing, ${report.warnings} warning, ${report.passed} passing.`
            : `Doctor: all ${report.passed} checks passing.`,
        );
      }
      return report;
    } catch (err) {
      writeDoctor(key, { loading: false, error: `Doctor run failed: ${shortError(err)}` });
      if (!silent) {
        onStatus(`Doctor run failed: ${err}`);
        onToast(`Doctor run failed: ${err}`, "error");
      }
      return null;
    }
  };

  const applySafeFix = async (project, check) => {
    const id = String(check?.id || "").trim();
    if (!id) {
      return null;
    }
    // Client-side fixKind gate: only safe 1-click fixes go through here. A
    // risky or none check never reaches the bridge (the Go refusal stays as
    // the backstop, this is the client gate).
    if (doctorFixControl(check || {}).kind !== "fix") {
      onToast("This check has no one-click fix. Open a preview first.", "error");
      return null;
    }
    try {
      const outcome = await bridge.applyDoctorFix(project, id, "", false);
      const message = sanitizeUIText(outcome?.message || "Fix applied.", HEALTH_MESSAGE_MAX);
      onToast(message, "success");
      onStatus(message);
      // The banner clears only through a fresh run: re-run so fixed issues
      // disappear instead of lingering as stale red.
      await runDoctor(project, { silent: true });
      return outcome;
    } catch (err) {
      onStatus(`Fix failed: ${err}`);
      onToast(`Fix failed: ${err}`, "error");
      return null;
    }
  };

  const previewFix = async (project, check) => {
    const control = doctorFixControl(check || {});
    const id = String(check?.id || "").trim();
    if (!id || control.kind === "none") {
      onToast("This check has no fix available.", "error");
      return null;
    }
    if (control.kind === "fix") {
      // Safe fixes stay 1-click even when reached through the preview path.
      return applySafeFix(project, check);
    }
    try {
      const preview = await bridge.previewDoctorFix(project, id);
      pending = {
        checkId: sanitizeUIText(preview?.checkId || preview?.CheckID || id, HEALTH_ID_MAX),
        previewId: String(preview?.previewId || preview?.PreviewID || ""),
      };
      setState({
        healthPreview: {
          open: true,
          checkId: pending.checkId,
          // No token here by design: panels never render it (see the DOM
          // test) and no store snapshot may carry it (see the token test);
          // it is only sent back to ApplyDoctorFix on confirm.
          summary: sanitizeUIText(
            preview?.summary || preview?.Summary || "",
            HEALTH_MESSAGE_MAX,
          ),
          actions: Array.isArray(preview?.actions ?? preview?.Actions)
            ? (preview.actions ?? preview.Actions).map((action) =>
                sanitizeUIText(action, HEALTH_MESSAGE_MAX),
              )
            : [],
        },
      });
      return preview;
    } catch (err) {
      onStatus(`Preview failed: ${err}`);
      onToast(`Preview failed: ${err}`, "error");
      return null;
    }
  };

  const applyFix = async (project, check, previewId, confirm) => {
    const id = String(check?.id || "").trim();
    const token = String(previewId || "");
    // Risky fixes always open a preview first: without the matching stored
    // token plus an explicit confirm the bridge is never reached (the Go
    // side refuses too, this is the client-side gate). The token must also
    // match the module-local pending preview, so a token replayed from an
    // old snapshot cannot apply.
    if (
      doctorFixControl(check || {}).kind !== "preview" ||
      !id ||
      !token ||
      !confirm ||
      pending.checkId !== id ||
      pending.previewId !== token
    ) {
      onToast("Preview the fix and confirm explicitly before applying.", "error");
      return null;
    }
    try {
      const outcome = await bridge.applyDoctorFix(project, id, token, true);
      const message = sanitizeUIText(outcome?.message || "Fix applied.", HEALTH_MESSAGE_MAX);
      pending = { checkId: "", previewId: "" };
      closePreview();
      onToast(message, "success");
      onStatus(message);
      await runDoctor(project, { silent: true });
      return outcome;
    } catch (err) {
      onStatus(`Fix failed: ${err}`);
      onToast(`Fix failed: ${err}`, "error");
      return null;
    }
  };

  const closePreview = () => {
    pending = { checkId: "", previewId: "" };
    setState({
      healthPreview: { open: false, checkId: "", summary: "", actions: [] },
    });
  };

  const runAudit = async (project) => {
    const key = scopeKey(project);
    writeAudit(key, { loading: true, error: "", phase: "running" });
    try {
      const result = normalizeAuditResult(await bridge.runIntegrityAudit(project));
      if (result.opId) {
        auditOpScopes[result.opId] = key;
      }
      writeAudit(key, { result, loading: false, error: "", opId: "", phase: "done" });
      onStatus(
        result.findingCount > 0
          ? `Audit: ${result.findingCount} finding(s).`
          : "Audit: integrity clean.",
      );
      return result;
    } catch (err) {
      writeAudit(key, { loading: false, error: `Audit run failed: ${shortError(err)}`, opId: "", phase: "failed" });
      onStatus(`Audit run failed: ${err}`);
      onToast(`Audit run failed: ${err}`, "error");
      return null;
    }
  };

  const cancelAudit = async () => {
    const opId = String(selectAuditEntry(getState()).opId || "");
    if (!opId) {
      onToast("No audit run in flight to cancel.", "error");
      return false;
    }
    try {
      const cancelled = await bridge.cancelAudit(opId);
      if (cancelled) {
        const key = auditOpScopes[opId] ?? normalizeHealthScope(getState().healthAudit?.scope);
        writeAudit(key, { opId: "", phase: "cancelled" });
        onStatus("Audit cancellation requested; the late result will be discarded.");
      }
      return Boolean(cancelled);
    } catch (err) {
      onStatus(`Cancel failed: ${err}`);
      onToast(`Cancel failed: ${err}`, "error");
      return false;
    }
  };

  // The audit:progress events feed the in-flight op id (for Cancel) into the
  // store; main.js subscribes with onEvent and forwards here. Events carry
  // no project, so an op whose scope the controller recorded patches that
  // scope; otherwise the current scope takes it.
  const handleAuditProgress = (payload = {}) => {
    const phase = String(payload?.phase || payload?.Phase || "");
    const opId = sanitizeUIText(payload?.opId || payload?.OpID || "", HEALTH_ID_MAX);
    if (!phase) {
      return;
    }
    const key = (opId && auditOpScopes[opId]) || scopeKey(getState().healthAudit?.scope);
    if (phase === "done" || phase === "failed" || phase === "cancelled") {
      if (opId) {
        delete auditOpScopes[opId];
      }
      writeAudit(key, { opId: "", phase });
      return;
    }
    if (opId) {
      auditOpScopes[opId] = key;
    }
    writeAudit(key, { opId, phase });
  };

  // The banner's persisted shortcut: fills the audit panel on mount without
  // running anything. A miss is neutral (never-ran state), not an error.
  const refreshBanner = async (project) => {
    const key = scopeKey(project);
    try {
      const result = normalizeAuditResult(await bridge.lastAuditResult(project));
      if (!result.runId) {
        writeAudit(key, { result: null, error: "" });
        return null;
      }
      writeAudit(key, { result, error: "" });
      return result;
    } catch (err) {
      writeAudit(key, { result: null, error: "" });
      return null;
    }
  };

  // Pre-mount recovery: a run that started before the panel mounted never
  // produced progress events here, so ask the backend for the scope's
  // in-flight op id and surface Cancel for it. A miss is neutral.
  const recoverAuditOp = async (project) => {
    const key = scopeKey(project);
    try {
      const opId = sanitizeUIText(String((await bridge.runningAuditOp(project)) || ""), HEALTH_ID_MAX);
      if (!opId) {
        return "";
      }
      auditOpScopes[opId] = key;
      writeAudit(key, { opId, phase: "running", loading: true });
      return opId;
    } catch (err) {
      return "";
    }
  };

  // Project switch: point both slices at the new scope (loading/empty when
  // it never ran, never the previous scope's banners), drop any pending
  // preview from the old scope, then re-derive this scope's audit banner
  // and recover its in-flight op.
  const selectHealthProject = async (project) => {
    const key = selectHealthProjectForTest(setState, getState, project);
    pending = { checkId: "", previewId: "" };
    setState({
      healthPreview: { open: false, checkId: "", summary: "", actions: [] },
    });
    await refreshBanner(project);
    await recoverAuditOp(project);
    return key;
  };

  return {
    runDoctor,
    previewFix,
    applySafeFix,
    applyFix,
    closePreview,
    runAudit,
    cancelAudit,
    handleAuditProgress,
    refreshBanner,
    recoverAuditOp,
    selectHealthProject,
    pendingPreview,
  };
}
