import { doctorFixControl, selectAuditEntry, selectDoctorEntry, selectPreviewEntry } from "../modules/health.js";
import { useStore } from "../state/useStore.js";

export type DoctorCheck = {
  id: string;
  title: string;
  status: string;
  message: string;
  hint: string;
  suggestedCommand: string;
  fixKind: string;
  fixLabel: string;
  needsPreview: boolean;
};

export type AuditCheckRow = {
  id: string;
  title: string;
  enabled: boolean;
  disabledReason: string;
};

export type AuditFinding = {
  tool: string;
  rule: string;
  path: string;
  line: number;
  message: string;
};

export type HealthPanelCallbacks = {
  project: string;
  onRunDoctor(): void;
  onCancelDoctor(): void;
  onApplySafeFix(check: DoctorCheck): void;
  onPreviewFix(check: DoctorCheck): void;
  onConfirmPreview(): void;
  onClosePreview(): void;
  onRunAudit(): void;
  onCancelAudit(): void;
};

const doctorStatusTone = (status: string) => {
  switch (String(status || "")) {
    case "pass":
      return { icon: "check_circle", chip: "border-primary/25 bg-primary/10 text-primary" };
    case "fail":
      return { icon: "error", chip: "border-red-500/25 bg-red-500/10 text-red-600 dark:text-red-400" };
    default:
      return { icon: "warning", chip: "border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400" };
  }
};

function DoctorPanel({ callbacks }: { callbacks: HealthPanelCallbacks }) {
  const state = useStore();
  // The entry is always the current project scope's: a scope that never ran
  // renders loading/empty, never another scope's report. The op id of a
  // run in flight (bound at run start or recovered on mount, same pattern
  // as the audit panel) drives the Cancel affordance.
  const doctor = selectDoctorEntry(state) as {
    report: { checks: DoctorCheck[]; passed: number; warnings: number; failures: number } | null;
    loading: boolean;
    error: string;
    opId: string;
  };
  // No previewId here by design: the token lives in module-local controller
  // state, never in the shared store, so it cannot reach the DOM or a store
  // snapshot. The entry is the current scope's, so another scope's dialog
  // never renders here.
  const preview = selectPreviewEntry(state) as {
    open: boolean;
    checkId: string;
    summary: string;
    actions: string[];
  };
  const report = doctor.report;
  const previewCheck = (report?.checks || []).find((check) => check.id === preview.checkId);

  return (
    <section
      id="doctorPanel"
      data-testid="doctor-panel"
      className="rounded-xl border border-border-primary bg-surface-primary/80 p-3 shadow-[0_8px_30px_rgba(0,0,0,0.25)]"
    >
      <div className="flex items-center justify-between gap-2">
        <p className="text-[11px] uppercase tracking-[0.16em] text-slate-600 dark:text-white/80 font-bold flex items-center gap-1.5">
          <span className="material-symbols-outlined text-[13px]">monitor_heart</span>
          Doctor
        </p>
        <div className="flex gap-2">
          {doctor.loading && doctor.opId ? (
            <button
              type="button"
              data-testid="cancel-doctor"
              onClick={() => callbacks.onCancelDoctor()}
              className="px-3 py-1.5 rounded-lg text-[11px] font-bold border border-red-500/40 text-red-600 dark:text-red-400 hover:bg-red-500/10 transition-all"
            >
              Cancel
            </button>
          ) : null}
          <button
            type="button"
            data-testid="run-doctor"
            disabled={doctor.loading}
            onClick={() => callbacks.onRunDoctor()}
            className="px-3 py-1.5 rounded-lg text-[11px] font-bold bg-primary text-slate-900 hover:bg-primary/90 disabled:opacity-50 transition-all inline-flex items-center gap-1"
          >
            <span className="material-symbols-outlined text-[14px]">stethoscope</span>
            {doctor.loading ? "Running..." : "Run doctor"}
          </button>
        </div>
      </div>

      {doctor.error ? (
        <p data-testid="doctor-error" className="mt-2 text-[11px] text-red-600 dark:text-red-400">
          {doctor.error}
        </p>
      ) : null}

      {!report ? (
        <p className="mt-2 text-[11px] text-text-tertiary">
          Run doctor to surface fixable health issues. Safe fixes apply in one click;
          riskier ones always open a preview first.
        </p>
      ) : (
        <>
          <p data-testid="doctor-summary" className="mt-2 text-[11px] text-text-tertiary">
            {report.failures} failing, {report.warnings} warning, {report.passed} passing.
          </p>
          <ul className="mt-2 space-y-1.5">
            {report.checks.map((check) => {
              const tone = doctorStatusTone(check.status);
              const control = doctorFixControl(check);
              return (
                <li
                  key={check.id}
                  data-testid={`doctor-check-${check.id}`}
                  className="flex items-start justify-between gap-2 rounded-lg border border-border-primary bg-background-secondary/40 px-2 py-1.5"
                >
                  <div className="min-w-0 flex items-start gap-1.5">
                    <span className="material-symbols-outlined text-[14px] mt-px shrink-0" aria-hidden="true">
                      {tone.icon}
                    </span>
                    <div className="min-w-0">
                      <p className="text-[11px] font-semibold text-text-primary truncate">{check.title}</p>
                      {check.message ? (
                        <p className="text-[10px] text-text-tertiary whitespace-pre-line">{check.message}</p>
                      ) : null}
                      {check.hint ? (
                        <p className="text-[10px] text-text-tertiary/80 whitespace-pre-line">{check.hint}</p>
                      ) : null}
                    </div>
                  </div>
                  {control.kind === "fix" ? (
                    <button
                      type="button"
                      data-testid={`fix-${check.id}`}
                      onClick={() => callbacks.onApplySafeFix(check)}
                      className="shrink-0 px-2.5 py-1 rounded-lg text-[10px] font-bold bg-primary text-slate-900 hover:bg-primary/90 transition-all"
                    >
                      {control.label}
                    </button>
                  ) : control.kind === "preview" ? (
                    <button
                      type="button"
                      data-testid={`preview-fix-${check.id}`}
                      onClick={() => callbacks.onPreviewFix(check)}
                      className="shrink-0 px-2.5 py-1 rounded-lg text-[10px] font-bold border border-primary/40 text-primary hover:bg-primary/10 transition-all"
                    >
                      {control.label}
                    </button>
                  ) : null}
                </li>
              );
            })}
          </ul>
        </>
      )}

      {preview.open ? (
        <div
          data-testid="doctor-preview-dialog"
          role="dialog"
          aria-label={`Preview fix for ${previewCheck?.title || preview.checkId}`}
          className="mt-2 rounded-lg border border-primary/30 bg-background-secondary/60 p-2.5"
        >
          <p className="text-[11px] font-bold text-text-primary flex items-center gap-1.5">
            <span className="material-symbols-outlined text-[14px] text-primary">preview</span>
            Preview fix{previewCheck ? ` for ${previewCheck.title}` : ""}
          </p>
          {preview.summary ? (
            <p data-testid="doctor-preview-summary" className="mt-1 text-[11px] text-text-secondary whitespace-pre-line">
              {preview.summary}
            </p>
          ) : null}
          {preview.actions.length > 0 ? (
            <ul className="mt-1 space-y-1">
              {preview.actions.map((action, index) => (
                <li key={index} className="text-[10px] font-mono text-text-tertiary whitespace-pre-line">
                  {action}
                </li>
              ))}
            </ul>
          ) : null}
          <p className="mt-1 text-[10px] text-text-tertiary">
            Review the plan above. Nothing has changed yet.
          </p>
          <div className="mt-2 flex gap-2 justify-end">
            <button
              type="button"
              data-testid="cancel-preview-fix"
              onClick={() => callbacks.onClosePreview()}
              className="px-3 py-1.5 rounded-lg text-[11px] font-medium text-text-secondary hover:bg-primary/10 transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              data-testid="confirm-preview-fix"
              onClick={() => callbacks.onConfirmPreview()}
              className="px-3 py-1.5 rounded-lg text-[11px] font-bold bg-primary text-slate-900 hover:bg-primary/90 transition-all inline-flex items-center gap-1"
            >
              <span className="material-symbols-outlined text-[14px]">check</span>
              Confirm and apply
            </button>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function AuditPanel({ callbacks }: { callbacks: HealthPanelCallbacks }) {
  const state = useStore();
  // Same per-scope rule as the doctor panel, plus the recovered in-flight
  // op id: Cancel shows when a run started before this panel mounted.
  const audit = selectAuditEntry(state) as {
    result: {
      status: string;
      findingCount: number;
      sessionId: string;
      runId: string;
      checks: AuditCheckRow[];
      findings: AuditFinding[];
    } | null;
    loading: boolean;
    error: string;
    opId: string;
    phase: string;
  };
  const result = audit.result;
  const running = audit.loading || audit.phase === "running";

  return (
    <section
      id="auditPanel"
      data-testid="audit-panel"
      className="rounded-xl border border-border-primary bg-surface-primary/80 p-3 shadow-[0_8px_30px_rgba(0,0,0,0.25)]"
    >
      <div className="flex items-center justify-between gap-2">
        <p className="text-[11px] uppercase tracking-[0.16em] text-slate-600 dark:text-white/80 font-bold flex items-center gap-1.5">
          <span className="material-symbols-outlined text-[13px]">fact_check</span>
          Audit
        </p>
        <div className="flex gap-2">
          {running && audit.opId ? (
            <button
              type="button"
              data-testid="cancel-audit"
              onClick={() => callbacks.onCancelAudit()}
              className="px-3 py-1.5 rounded-lg text-[11px] font-bold border border-red-500/40 text-red-600 dark:text-red-400 hover:bg-red-500/10 transition-all"
            >
              Cancel
            </button>
          ) : null}
          <button
            type="button"
            data-testid="run-audit"
            disabled={audit.loading}
            onClick={() => callbacks.onRunAudit()}
            className="px-3 py-1.5 rounded-lg text-[11px] font-bold bg-primary text-slate-900 hover:bg-primary/90 disabled:opacity-50 transition-all inline-flex items-center gap-1"
          >
            <span className="material-symbols-outlined text-[14px]">play_arrow</span>
            {audit.loading ? "Auditing..." : "Run audit"}
          </button>
        </div>
      </div>

      {audit.error ? (
        <p data-testid="audit-error" className="mt-2 text-[11px] text-red-600 dark:text-red-400">
          {audit.error}
        </p>
      ) : null}

      {!result ? (
        <p className="mt-2 text-[11px] text-text-tertiary">
          No integrity audit yet. The desktop runs the container-free integrity checks;
          Docker-backed checks stay listed below with their reason once a run exists.
        </p>
      ) : (
        <>
          <p data-testid="audit-summary" className="mt-2 text-[11px] text-text-tertiary">
            {result.status}
            {result.findingCount > 0 ? `, ${result.findingCount} finding(s)` : ", clean"}.
          </p>
          {result.findings.length > 0 ? (
            <ul className="mt-2 space-y-1.5">
              {result.findings.map((finding, index) => (
                <li
                  key={index}
                  data-testid="audit-finding"
                  className="rounded-lg border border-amber-500/25 bg-amber-500/10 px-2 py-1.5"
                >
                  <p className="text-[11px] font-semibold text-text-primary">
                    {finding.tool}
                    {finding.rule ? ` (${finding.rule})` : ""}
                  </p>
                  {finding.path ? (
                    <p className="text-[10px] font-mono text-text-tertiary">
                      {finding.path}
                      {finding.line ? `:${finding.line}` : ""}
                    </p>
                  ) : null}
                  <p className="text-[10px] text-text-secondary whitespace-pre-line">{finding.message}</p>
                </li>
              ))}
            </ul>
          ) : null}
          <ul className="mt-2 space-y-1.5">
            {result.checks.map((check) => (
              <li
                key={check.id}
                data-testid={check.enabled ? `audit-check-${check.id}` : "audit-disabled-row"}
                className={`flex items-start gap-1.5 rounded-lg border px-2 py-1.5 ${
                  check.enabled
                    ? "border-border-primary bg-background-secondary/40"
                    : "border-border-primary bg-background-secondary/20 opacity-80"
                }`}
              >
                <span className="material-symbols-outlined text-[14px] mt-px shrink-0" aria-hidden="true">
                  {check.enabled ? "check_circle" : "block"}
                </span>
                <div className="min-w-0">
                  <p className="text-[11px] font-semibold text-text-primary">{check.title}</p>
                  {!check.enabled && check.disabledReason ? (
                    <p className="text-[10px] text-text-tertiary whitespace-pre-line">{check.disabledReason}</p>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

/**
 * The health results views: doctor panel (run, per-check Fix/Preview-fix,
 * preview-confirm dialog) and audit panel (integrity run, findings,
 * disabled-with-reason docker rows). Rendered inside the ops deck island, so
 * no new container or nav markup is needed.
 */
export function HealthPanels({ callbacks }: { callbacks: HealthPanelCallbacks }) {
  return (
    <div className="grid gap-3 xl:grid-cols-2">
      <DoctorPanel callbacks={callbacks} />
      <AuditPanel callbacks={callbacks} />
    </div>
  );
}
