import { useCallback, useEffect, useState } from "react";
import {
  deployFetchBlocked,
  emptyReleasesGuidance,
  normalizeDeployReleases,
  probeDisclosureNote,
  sanitizeDeployText,
} from "../modules/deploy.js";
import { normalizeRemotesPayload } from "../modules/remotes.js";
import { getState } from "../state/store.js";
import { useStore } from "../state/useStore.js";

type DeployBridge = {
  getRemotes(project: string): Promise<Record<string, unknown>>;
  deployPlan(project: string, remote: string): Promise<Record<string, unknown>>;
  deployCheck(project: string, remote: string): Promise<Record<string, unknown>>;
  deployStatus(project: string, remote: string): Promise<Record<string, unknown>>;
  listReleases(project: string, remote: string): Promise<unknown>;
  previewRollback(
    project: string,
    remote: string,
    to: string,
  ): Promise<Record<string, unknown>>;
  applyRollback(
    project: string,
    token: string,
    confirm: boolean,
  ): Promise<Record<string, unknown>>;
  previewSandboxUp(
    project: string,
    profile: string,
    recreate: boolean,
  ): Promise<Record<string, unknown>>;
  sandboxUp(
    project: string,
    token: string,
    confirm: boolean,
  ): Promise<Record<string, unknown>>;
  previewSandboxDown(
    project: string,
    purge: boolean,
    volumes: boolean,
  ): Promise<Record<string, unknown>>;
  sandboxDown(
    project: string,
    token: string,
    confirm: boolean,
  ): Promise<Record<string, unknown>>;
  sandboxSSHinfo(project: string): Promise<Record<string, unknown>>;
  openSandboxSSHInTerminal(project: string): Promise<unknown>;
};

type ToastKind = "info" | "success" | "error" | "warning";

/** What main.js keeps calling after the tab moved into React. */
export type DeployIslandApi = {
  refresh(options?: { silent?: boolean }): Promise<void>;
};

type Props = {
  bridge: DeployBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  registerApi(api: DeployIslandApi | null): void;
};

type ReleaseRow = {
  release: string;
  revision: string;
  branch: string;
  status: string;
  createdAt: string;
  createdBy: string;
  live: boolean;
  foreign: boolean;
};

/** Shown before a project is picked. */
const IDLE_NOTICE = "Select an environment and a remote to read deploy state.";

const outputPaneClass =
  "m-0 font-mono whitespace-pre-wrap wrap-break-word text-[11px] text-text-secondary";

const fieldOf = (record: Record<string, unknown>, ...keys: string[]): string => {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "string" && value.trim() !== "") {
      return value;
    }
  }
  return "";
};

const stepsOf = (plan: Record<string, unknown>): Array<Record<string, unknown>> => {
  const steps = plan.Steps ?? plan.steps;
  return Array.isArray(steps) ? steps : [];
};

const warningsOf = (check: Record<string, unknown>): string[] => {
  const warnings = check.Warnings ?? check.warnings;
  return Array.isArray(warnings) ? warnings.map((item) => String(item ?? "")) : [];
};

const boolOf = (
  record: Record<string, unknown>,
  ...keys: string[]
): boolean | null => {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "boolean") {
      return value;
    }
  }
  return null;
};

const yesNo = (value: boolean | null): string => {
  if (value === null) {
    return "unknown";
  }
  return value ? "yes" : "no";
};

/**
 * Deploy views plus guarded mutating actions (M2). The read-only island
 * renders the releases table, the live status view and the plan/check output
 * panes for the remote picked in the selector. Fetching stays gated behind
 * that picker: with no remote picked no call leaves the panel.
 *
 * Every mutation below stays behind its own two-step gate: a preview call
 * that shows exactly what the apply would do and mints a single-use token,
 * then an apply that spends that token plus an explicit checkbox confirm.
 * The apply button stays disabled until the preview is shown AND the box is
 * ticked, so no single click can change target state. A cancelled run
 * renders as cancelled with the backend's cleanup notes, never as a
 * failure.
 */
export function DeployPanel({ bridge, onStatus, onToast, registerApi }: Props) {
  // The selection lives in the store so main.js refresh calls land on the
  // current project even when they run before React re-renders with it.
  useStore();
  const [remoteNames, setRemoteNames] = useState<string[]>([]);
  const [remote, setRemote] = useState("");
  const [releases, setReleases] = useState<ReleaseRow[]>([]);
  const [status, setStatus] = useState<Record<string, unknown> | null>(null);
  const [plan, setPlan] = useState<Record<string, unknown> | null>(null);
  const [check, setCheck] = useState<Record<string, unknown> | null>(null);
  const [loading, setLoading] = useState(false);
  const [notice, setNotice] = useState(IDLE_NOTICE);
  const [failure, setFailure] = useState("");

  // M2 rollback gate: preview first (mints the token), then an explicit
  // checkbox confirm, then the apply spends both. The confirm resets on
  // every fresh preview so a stale tick can never authorise a new swap.
  const [rollbackTo, setRollbackTo] = useState("");
  const [rollbackPreview, setRollbackPreview] = useState<Record<
    string,
    unknown
  > | null>(null);
  const [rollbackConfirm, setRollbackConfirm] = useState(false);
  const [rollbackBusy, setRollbackBusy] = useState(false);
  const [rollbackOutcome, setRollbackOutcome] = useState("");
  const [rollbackCancelled, setRollbackCancelled] = useState(false);

  // M2 sandbox rehearse gate: same preview-plus-confirm shape. The up
  // preview describes the container the apply would create; the down
  // preview names the container the apply would remove.
  const [sandboxProfile, setSandboxProfile] = useState("");
  const [sandboxRecreate, setSandboxRecreate] = useState(false);
  const [sandboxUpPreview, setSandboxUpPreview] = useState<Record<
    string,
    unknown
  > | null>(null);
  const [sandboxUpConfirm, setSandboxUpConfirm] = useState(false);
  const [sandboxUpBusy, setSandboxUpBusy] = useState(false);
  const [sandboxUpOutcome, setSandboxUpOutcome] = useState("");
  const [sandboxUpCancelled, setSandboxUpCancelled] = useState(false);
  const [sandboxPurge, setSandboxPurge] = useState(false);
  const [sandboxVolumes, setSandboxVolumes] = useState(false);
  const [sandboxDownPreview, setSandboxDownPreview] = useState<Record<
    string,
    unknown
  > | null>(null);
  const [sandboxDownConfirm, setSandboxDownConfirm] = useState(false);
  const [sandboxDownBusy, setSandboxDownBusy] = useState(false);
  const [sandboxDownOutcome, setSandboxDownOutcome] = useState("");
  const [sandboxDownCancelled, setSandboxDownCancelled] = useState(false);
  const [sandboxSSH, setSandboxSSH] = useState<Record<string, unknown> | null>(
    null,
  );
  const [sandboxSSHBusy, setSandboxSSHBusy] = useState(false);

  const refresh = useCallback(
    async (options: { silent?: boolean } = {}) => {
      const silent = Boolean(options.silent);
      const project = String(getState().selectedProject || "").trim();
      const blocked = deployFetchBlocked(project, remote);
      if (blocked) {
        setRemoteNames([]);
        setReleases([]);
        setStatus(null);
        setPlan(null);
        setCheck(null);
        setFailure("");
        setNotice(project ? blocked : IDLE_NOTICE);
        return;
      }
      if (!silent) {
        onStatus(`Status: loading deploy state for ${remote}...`);
      }
      setLoading(true);
      setFailure("");
      try {
        // Every pane settles on its own: one failing call must not discard
        // the four successes, so each pane renders from its own outcome and
        // only the failed names are reported.
        const [remotesRes, planRes, checkRes, statusRes, releasesRes] =
          await Promise.allSettled([
            bridge.getRemotes(project),
            bridge.deployPlan(project, remote),
            bridge.deployCheck(project, remote),
            bridge.deployStatus(project, remote),
            bridge.listReleases(project, remote),
          ]);
        const failed: string[] = [];
        if (remotesRes.status === "fulfilled") {
          const payload = normalizeRemotesPayload(remotesRes.value);
          setRemoteNames(
            payload.remotes.map((entry: { name: string }) => entry.name),
          );
        } else {
          failed.push("remotes");
        }
        if (planRes.status === "fulfilled") {
          const planRaw = planRes.value;
          setPlan(
            planRaw && typeof planRaw === "object"
              ? (planRaw as Record<string, unknown>)
              : null,
          );
        } else {
          setPlan(null);
          failed.push("plan");
        }
        if (checkRes.status === "fulfilled") {
          const checkRaw = checkRes.value;
          setCheck(
            checkRaw && typeof checkRaw === "object"
              ? (checkRaw as Record<string, unknown>)
              : null,
          );
        } else {
          setCheck(null);
          failed.push("check");
        }
        if (statusRes.status === "fulfilled") {
          const statusRaw = statusRes.value;
          setStatus(
            statusRaw && typeof statusRaw === "object"
              ? (statusRaw as Record<string, unknown>)
              : null,
          );
        } else {
          setStatus(null);
          failed.push("status");
        }
        if (releasesRes.status === "fulfilled") {
          setReleases(normalizeDeployReleases(releasesRes.value));
        } else {
          setReleases([]);
          failed.push("releases");
        }
        if (failed.length === 0) {
          setNotice("");
          if (!silent) {
            onStatus(`Status: deploy state loaded for ${remote}`);
          }
        } else if (failed.length === 5) {
          const message = "Failed to load deploy state.";
          setFailure(message);
          onStatus(`Status: failed to load deploy state for ${remote}`);
          onToast("Failed to load deploy state.", "error");
        } else {
          const message = `Some deploy panes failed to load (${failed.join(", ")}); the rest are shown.`;
          setFailure(sanitizeDeployText(message));
          onStatus(`Status: partially loaded deploy state for ${remote}`);
          onToast("Some deploy panes failed to load.", "warning");
        }
      } catch (err) {
        const message = sanitizeDeployText(
          err instanceof Error ? err.message : String(err),
        );
        setFailure(message || "Failed to load deploy state.");
        onStatus(`Status: failed to load deploy state for ${remote}`);
        onToast("Failed to load deploy state.", "error");
      } finally {
        setLoading(false);
      }
    },
    [bridge, onStatus, onToast, remote],
  );

  // Loading the picker options is read-only too: it lists remote names so the
  // selector has something to offer before any deploy call runs.
  const refreshPicker = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    if (!project) {
      setRemoteNames([]);
      setNotice(IDLE_NOTICE);
      return;
    }
    try {
      const payload = normalizeRemotesPayload(await bridge.getRemotes(project));
      const names = payload.remotes.map((entry: { name: string }) => entry.name);
      setRemoteNames(names);
      setRemote((prev) => (prev && names.includes(prev) ? prev : (names[0] ?? "")));
      if (!remote) {
        setNotice(deployFetchBlocked(project, names[0] ?? ""));
      }
    } catch {
      setRemoteNames([]);
    }
  }, [bridge, remote]);

  useEffect(() => {
    registerApi({ refresh });
    return () => registerApi(null);
  }, [registerApi, refresh]);

  // --- M2 mutating actions -------------------------------------------------
  // Each pair shares one shape: preview shows what the apply would do and
  // hands back a single-use token; the apply spends it plus an explicit
  // confirm. A spent or expired token refuses, so every second run needs a
  // fresh preview. Cancelled runs render as cancelled with the backend's
  // cleanup notes, never as failures.

  const previewRollbackRun = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    if (!project || !remote) {
      return;
    }
    setRollbackBusy(true);
    setRollbackOutcome("");
    setRollbackCancelled(false);
    try {
      const raw = await bridge.previewRollback(project, remote, rollbackTo);
      setRollbackPreview(
        raw && typeof raw === "object"
          ? (raw as Record<string, unknown>)
          : null,
      );
      setRollbackConfirm(false);
      onStatus("Status: rollback preview ready, confirm to apply");
    } catch (err) {
      setRollbackPreview(null);
      const message = sanitizeDeployText(
        err instanceof Error ? err.message : String(err),
      );
      setRollbackOutcome(message || "Rollback preview failed.");
      onToast("Rollback preview failed.", "error");
    } finally {
      setRollbackBusy(false);
    }
  }, [bridge, onStatus, onToast, remote, rollbackTo]);

  const applyRollbackRun = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    const token = String(rollbackPreview?.Token ?? rollbackPreview?.token ?? "");
    if (!project || !token || !rollbackConfirm) {
      return;
    }
    setRollbackBusy(true);
    try {
      const raw = await bridge.applyRollback(project, token, true);
      const result =
        raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {};
      const cancelled = Boolean(result.Cancelled ?? result.cancelled);
      setRollbackCancelled(cancelled);
      setRollbackOutcome(
        sanitizeDeployText(
          fieldOf(result, "Message", "message") ||
            (cancelled ? "Rollback was cancelled." : "Rollback applied."),
        ),
      );
      // The token is spent either way: every further run needs a preview.
      setRollbackPreview(null);
      setRollbackConfirm(false);
      if (cancelled) {
        onStatus("Status: rollback was cancelled");
      } else {
        onStatus("Status: rollback applied");
        await refresh({ silent: true });
      }
    } catch (err) {
      const message = sanitizeDeployText(
        err instanceof Error ? err.message : String(err),
      );
      setRollbackCancelled(false);
      setRollbackOutcome(message || "Rollback failed.");
      onToast("Rollback failed.", "error");
    } finally {
      setRollbackBusy(false);
    }
  }, [bridge, onStatus, onToast, refresh, rollbackConfirm, rollbackPreview]);

  const previewSandboxUpRun = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    if (!project) {
      return;
    }
    setSandboxUpBusy(true);
    setSandboxUpOutcome("");
    setSandboxUpCancelled(false);
    try {
      const raw = await bridge.previewSandboxUp(
        project,
        sandboxProfile,
        sandboxRecreate,
      );
      setSandboxUpPreview(
        raw && typeof raw === "object"
          ? (raw as Record<string, unknown>)
          : null,
      );
      setSandboxUpConfirm(false);
      onStatus("Status: sandbox preview ready, confirm to start");
    } catch (err) {
      setSandboxUpPreview(null);
      const message = sanitizeDeployText(
        err instanceof Error ? err.message : String(err),
      );
      setSandboxUpOutcome(message || "Sandbox preview failed.");
      onToast("Sandbox preview failed.", "error");
    } finally {
      setSandboxUpBusy(false);
    }
  }, [bridge, onStatus, onToast, sandboxProfile, sandboxRecreate]);

  const sandboxUpRun = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    const token = String(
      sandboxUpPreview?.Token ?? sandboxUpPreview?.token ?? "",
    );
    if (!project || !token || !sandboxUpConfirm) {
      return;
    }
    setSandboxUpBusy(true);
    try {
      const raw = await bridge.sandboxUp(project, token, true);
      const result =
        raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {};
      const cancelled = Boolean(result.Cancelled ?? result.cancelled);
      setSandboxUpCancelled(cancelled);
      setSandboxUpOutcome(
        sanitizeDeployText(
          fieldOf(result, "Message", "message") ||
            (cancelled ? "Sandbox start was cancelled." : "Sandbox started."),
        ),
      );
      setSandboxUpPreview(null);
      setSandboxUpConfirm(false);
      onStatus(
        cancelled
          ? "Status: sandbox start was cancelled"
          : "Status: sandbox started",
      );
    } catch (err) {
      const message = sanitizeDeployText(
        err instanceof Error ? err.message : String(err),
      );
      setSandboxUpCancelled(false);
      setSandboxUpOutcome(message || "Sandbox start failed.");
      onToast("Sandbox start failed.", "error");
    } finally {
      setSandboxUpBusy(false);
    }
  }, [bridge, onStatus, onToast, sandboxUpConfirm, sandboxUpPreview]);

  const previewSandboxDownRun = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    if (!project) {
      return;
    }
    setSandboxDownBusy(true);
    setSandboxDownOutcome("");
    setSandboxDownCancelled(false);
    try {
      const raw = await bridge.previewSandboxDown(
        project,
        sandboxPurge,
        sandboxVolumes,
      );
      setSandboxDownPreview(
        raw && typeof raw === "object"
          ? (raw as Record<string, unknown>)
          : null,
      );
      setSandboxDownConfirm(false);
      onStatus("Status: sandbox removal preview ready, confirm to apply");
    } catch (err) {
      setSandboxDownPreview(null);
      const message = sanitizeDeployText(
        err instanceof Error ? err.message : String(err),
      );
      setSandboxDownOutcome(message || "Sandbox removal preview failed.");
      onToast("Sandbox removal preview failed.", "error");
    } finally {
      setSandboxDownBusy(false);
    }
  }, [bridge, onStatus, onToast, sandboxPurge, sandboxVolumes]);

  const sandboxDownRun = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    const token = String(
      sandboxDownPreview?.Token ?? sandboxDownPreview?.token ?? "",
    );
    if (!project || !token || !sandboxDownConfirm) {
      return;
    }
    setSandboxDownBusy(true);
    try {
      const raw = await bridge.sandboxDown(project, token, true);
      const result =
        raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {};
      const cancelled = Boolean(result.Cancelled ?? result.cancelled);
      setSandboxDownCancelled(cancelled);
      setSandboxDownOutcome(
        sanitizeDeployText(
          fieldOf(result, "Message", "message") ||
            (cancelled
              ? "Sandbox removal was cancelled."
              : "Sandbox removed."),
        ),
      );
      setSandboxDownPreview(null);
      setSandboxDownConfirm(false);
      onStatus(
        cancelled
          ? "Status: sandbox removal was cancelled"
          : "Status: sandbox removed",
      );
    } catch (err) {
      const message = sanitizeDeployText(
        err instanceof Error ? err.message : String(err),
      );
      setSandboxDownCancelled(false);
      setSandboxDownOutcome(message || "Sandbox removal failed.");
      onToast("Sandbox removal failed.", "error");
    } finally {
      setSandboxDownBusy(false);
    }
  }, [bridge, onStatus, onToast, sandboxDownConfirm, sandboxDownPreview]);

  const loadSandboxSSH = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    if (!project) {
      return;
    }
    setSandboxSSHBusy(true);
    try {
      const raw = await bridge.sandboxSSHinfo(project);
      setSandboxSSH(
        raw && typeof raw === "object"
          ? (raw as Record<string, unknown>)
          : null,
      );
    } catch (err) {
      setSandboxSSH(null);
      onToast(
        sanitizeDeployText(err instanceof Error ? err.message : String(err)) ||
          "Failed to read sandbox connection info.",
        "error",
      );
    } finally {
      setSandboxSSHBusy(false);
    }
  }, [bridge, onToast]);

  const openSandboxSSH = useCallback(async () => {
    const project = String(getState().selectedProject || "").trim();
    if (!project) {
      return;
    }
    try {
      await bridge.openSandboxSSHInTerminal(project);
      onStatus("Status: sandbox shell opened in the OS terminal");
    } catch (err) {
      onToast(
        sanitizeDeployText(err instanceof Error ? err.message : String(err)) ||
          "Failed to open the sandbox shell.",
        "error",
      );
    }
  }, [bridge, onStatus, onToast]);

  useEffect(() => {
    void refreshPicker();
  }, [refreshPicker]);

  const statusText = status
    ? fieldOf(status, "Status", "status") ||
      (String(status.Reachable ?? status.reachable) === "false"
        ? "Unreachable"
        : "Live")
    : "";
  const statusError = status ? sanitizeDeployText(fieldOf(status, "Error", "error")) : "";
  const probeNote = check ? probeDisclosureNote(check) : "";
  const planSteps = plan ? stepsOf(plan) : [];
  const checkWarnings = check ? warningsOf(check) : [];

  return (
    <div className="px-6 lg:px-10 py-6 max-w-[1248px] w-full mx-auto" id="deploy">
      <div className="space-y-8">
        <div className="flex items-center justify-between">
          <h2 className="text-slate-900 dark:text-white text-xl font-semibold flex items-center gap-2">
            <span className="material-symbols-outlined text-primary">rocket_launch</span>
            Deploy
          </h2>
          <div className="flex items-center gap-3">
            <label
              htmlFor="deployRemotePicker"
              className="text-[11px] uppercase tracking-wide text-slate-500"
            >
              Remote
            </label>
            <select
              id="deployRemotePicker"
              data-testid="deploy-remote-picker"
              className="bg-surface-secondary text-xs text-text-primary dark:text-white px-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden"
              value={remote}
              onChange={(event) => setRemote(event.target.value)}
            >
              <option value="">Select a remote</option>
              {remoteNames.map((name) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))}
            </select>
            <button
              type="button"
              data-testid="refresh-deploy"
              className="px-4 py-2 bg-background-secondary text-text-primary dark:text-white rounded-lg text-sm font-medium hover:bg-primary/20 dark:hover:bg-[#2e573a] transition-colors border border-border-primary dark:border-[#366b47] flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
              disabled={loading || !remote}
              onClick={() => void refresh()}
            >
              <span className="material-symbols-outlined text-lg">refresh</span>
              Refresh
            </button>
          </div>
        </div>

        {notice ? (
          <div className="p-8 text-center text-slate-500 border border-dashed border-(--bg-secondary) rounded-xl">
            {notice}
          </div>
        ) : null}

        {failure ? (
          <div className="p-4 rounded-xl border border-red-500/30 bg-red-500/5 text-sm text-red-600 dark:text-red-300 flex items-center justify-between gap-4">
            <span data-testid="deploy-error">{failure}</span>
            <button
              type="button"
              data-testid="deploy-retry"
              className="px-3 py-1.5 rounded-md text-xs font-semibold border border-red-500/40 hover:bg-red-500/10 transition-colors shrink-0"
              onClick={() => void refresh()}
            >
              Retry
            </button>
          </div>
        ) : null}

        <section aria-label="Deploy status">
          <h3 className="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2 pb-2">
            Status
          </h3>
          {status ? (
            <div className="rounded-xl border border-border-primary bg-background-primary p-4 text-sm space-y-1">
              <div>
                <span className="text-slate-500">Release: </span>
                <span data-testid="deploy-status-release">
                  {sanitizeDeployText(fieldOf(status, "Release", "release")) || "none"}
                </span>
              </div>
              <div>
                <span className="text-slate-500">Revision: </span>
                <span data-testid="deploy-status-revision">
                  {sanitizeDeployText(fieldOf(status, "Revision", "revision")) || "unknown"}
                </span>
              </div>
              <div>
                <span className="text-slate-500">State: </span>
                <span data-testid="deploy-status-state">{sanitizeDeployText(statusText)}</span>
              </div>
              {statusError ? (
                <div className="text-red-500">{statusError}</div>
              ) : null}
            </div>
          ) : (
            <div className="text-sm text-slate-500">
              {loading ? "Loading status..." : "No status loaded yet."}
            </div>
          )}
        </section>

        <section aria-label="Deploy releases">
          <h3 className="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2 pb-2">
            Releases
          </h3>
          {releases.length > 0 ? (
            <div className="rounded-xl border border-border-primary overflow-hidden">
              <table className="w-full text-sm" data-testid="deploy-releases-table">
                <thead>
                  <tr className="bg-surface-primary text-left text-[11px] uppercase tracking-wide text-slate-500">
                    <th className="px-4 py-2">Release</th>
                    <th className="px-4 py-2">Revision</th>
                    <th className="px-4 py-2">Branch</th>
                    <th className="px-4 py-2">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {releases.map((row) => (
                    <tr
                      key={row.release || `${row.revision}-${row.branch}`}
                      className="border-t border-border-primary"
                    >
                      <td className="px-4 py-2 font-mono text-xs">{row.release}</td>
                      <td className="px-4 py-2 font-mono text-xs">{row.revision}</td>
                      <td className="px-4 py-2">{row.branch}</td>
                      <td className="px-4 py-2">
                        {row.status}
                        {row.live ? " (live)" : ""}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="p-8 text-center text-slate-500 border border-dashed border-(--bg-secondary) rounded-xl">
              {emptyReleasesGuidance()}
            </div>
          )}
        </section>

        <section aria-label="Deploy plan">
          <h3 className="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2 pb-2">
            Plan
          </h3>
          <div className="rounded-xl border border-border-primary bg-background-primary overflow-hidden">
            <div className="flex-1 min-h-0 overflow-y-auto p-3 custom-scrollbar">
              <pre className={outputPaneClass} data-testid="deploy-plan-output">
                {plan
                  ? sanitizeDeployText(
                      planSteps
                        .map((step) => {
                          const title = sanitizeDeployText(
                            fieldOf(step, "Title", "title"),
                          );
                          const command = sanitizeDeployText(
                            fieldOf(step, "Command", "command"),
                          );
                          return command ? `${title}\n  ${command}` : title;
                        })
                        .join("\n") ||
                        `Branch: ${sanitizeDeployText(fieldOf(plan, "Branch", "branch"))}`,
                    )
                  : loading
                    ? "Loading plan..."
                    : "No plan loaded yet."}
              </pre>
            </div>
          </div>
        </section>

        <section aria-label="Deploy check">
          <h3 className="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2 pb-2">
            Check
          </h3>
          <div className="rounded-xl border border-border-primary bg-background-primary overflow-hidden">
            <div className="flex-1 min-h-0 overflow-y-auto p-3 custom-scrollbar">
              <pre className={outputPaneClass} data-testid="deploy-check-output">
                {check
                  ? sanitizeDeployText(
                      [
                        `Host: ${fieldOf(check, "Host", "host")}`,
                        `Deploy path: ${fieldOf(check, "DeployPath", "deployPath")}`,
                        `Strategy: ${fieldOf(check, "PublishStrategy", "publishStrategy")}`,
                        `Layout: ${fieldOf(check, "Layout", "layout")}`,
                        ...checkWarnings,
                      ].join("\n"),
                    )
                  : loading
                    ? "Loading check..."
                    : "No check loaded yet."}
              </pre>
              {probeNote ? (
                <p
                  className="px-3 pb-3 text-[11px] text-amber-600 dark:text-amber-300"
                  data-testid="deploy-probe-note"
                >
                  {probeNote}
                </p>
              ) : null}
            </div>
          </div>
        </section>

        <section aria-label="Deploy rollback">
          <h3 className="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2 pb-2">
            Rollback
          </h3>
          <div className="rounded-xl border border-border-primary bg-background-primary p-4 text-sm space-y-3">
            <p className="text-xs text-slate-500">
              Rollback swaps the live release back. Preview first to see the
              exact target, then tick the confirm box to enable the apply. A
              stale preview (a deploy landed in between) is refused and needs
              a fresh preview.
            </p>
            <div className="flex items-center gap-3 flex-wrap">
              <label
                htmlFor="rollbackTarget"
                className="text-[11px] uppercase tracking-wide text-slate-500"
              >
                Target release
              </label>
              <input
                id="rollbackTarget"
                data-testid="rollback-target"
                className="bg-surface-secondary text-xs text-text-primary dark:text-white px-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden"
                placeholder="empty means previous release"
                value={rollbackTo}
                onChange={(event) => setRollbackTo(event.target.value)}
              />
              <button
                type="button"
                data-testid="rollback-preview"
                className="px-3 py-1.5 rounded-md text-xs font-semibold border border-border-primary hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                disabled={rollbackBusy || !remote}
                onClick={() => void previewRollbackRun()}
              >
                Preview rollback
              </button>
            </div>
            {rollbackPreview ? (
              <div
                className="rounded-lg border border-border-primary p-3 text-xs space-y-1"
                data-testid="rollback-preview-output"
              >
                <div>
                  <span className="text-slate-500">Target: </span>
                  {sanitizeDeployText(
                    fieldOf(
                      (rollbackPreview.Target ??
                        rollbackPreview.target ??
                        {}) as Record<string, unknown>,
                      "Release",
                      "release",
                    ),
                  ) || "previous release"}
                </div>
                <div>
                  <span className="text-slate-500">Revision: </span>
                  {sanitizeDeployText(
                    fieldOf(
                      (rollbackPreview.Target ??
                        rollbackPreview.target ??
                        {}) as Record<string, unknown>,
                      "Revision",
                      "revision",
                    ),
                  ) || "unknown"}
                </div>
                <div>
                  <span className="text-slate-500">Previous: </span>
                  {sanitizeDeployText(
                    fieldOf(rollbackPreview, "Previous", "previous"),
                  ) || "none"}
                </div>
                <div>
                  <span className="text-slate-500">Strategy: </span>
                  {sanitizeDeployText(
                    fieldOf(rollbackPreview, "Strategy", "strategy"),
                  ) || "unknown"}
                </div>
                <label
                  htmlFor="rollbackConfirm"
                  className="flex items-center gap-2 pt-1 font-medium"
                >
                  <input
                    id="rollbackConfirm"
                    data-testid="rollback-confirm"
                    type="checkbox"
                    checked={rollbackConfirm}
                    onChange={(event) =>
                      setRollbackConfirm(event.target.checked)
                    }
                  />
                  I have read the preview and want this rollback applied
                </label>
              </div>
            ) : null}
            <div>
              <button
                type="button"
                data-testid="rollback-apply"
                className="px-3 py-1.5 rounded-md text-xs font-semibold border border-red-500/40 text-red-600 dark:text-red-300 hover:bg-red-500/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                disabled={
                  rollbackBusy || !rollbackPreview || !rollbackConfirm
                }
                onClick={() => void applyRollbackRun()}
              >
                Apply rollback
              </button>
            </div>
            {rollbackOutcome ? (
              <div
                data-testid="rollback-result"
                className={
                  rollbackCancelled
                    ? "text-xs text-amber-600 dark:text-amber-300"
                    : "text-xs text-text-secondary"
                }
              >
                {rollbackCancelled ? "Rollback was cancelled. " : ""}
                {rollbackOutcome}
              </div>
            ) : null}
          </div>
        </section>

        <section aria-label="Sandbox rehearse">
          <h3 className="text-text-primary dark:text-white text-lg font-semibold flex items-center gap-2 pb-2">
            Sandbox
          </h3>
          <div className="rounded-xl border border-border-primary bg-background-primary p-4 text-sm space-y-4">
            <p className="text-xs text-slate-500">
              The sandbox rehearses a deploy in a container. Starting and
              removal both need a preview plus a ticked confirm box. The
              rehearse starts empty; seeding stays CLI-only.
            </p>
            <div className="space-y-3">
              <div className="flex items-center gap-3 flex-wrap">
                <label
                  htmlFor="sandboxProfile"
                  className="text-[11px] uppercase tracking-wide text-slate-500"
                >
                  Profile
                </label>
                <input
                  id="sandboxProfile"
                  data-testid="sandbox-profile"
                  className="bg-surface-secondary text-xs text-text-primary dark:text-white px-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden"
                  placeholder="default profile"
                  value={sandboxProfile}
                  onChange={(event) => setSandboxProfile(event.target.value)}
                />
                <label
                  htmlFor="sandboxRecreate"
                  className="flex items-center gap-2 text-xs"
                >
                  <input
                    id="sandboxRecreate"
                    data-testid="sandbox-recreate"
                    type="checkbox"
                    checked={sandboxRecreate}
                    onChange={(event) =>
                      setSandboxRecreate(event.target.checked)
                    }
                  />
                  Recreate
                </label>
                <button
                  type="button"
                  data-testid="sandbox-up-preview"
                  className="px-3 py-1.5 rounded-md text-xs font-semibold border border-border-primary hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={sandboxUpBusy}
                  onClick={() => void previewSandboxUpRun()}
                >
                  Preview start
                </button>
              </div>
              {sandboxUpPreview ? (
                <div
                  className="rounded-lg border border-border-primary p-3 text-xs space-y-1"
                  data-testid="sandbox-up-preview-output"
                >
                  <div>
                    <span className="text-slate-500">Container: </span>
                    {sanitizeDeployText(
                      fieldOf(sandboxUpPreview, "Container", "container"),
                    ) || "new container"}
                  </div>
                  <div>
                    <span className="text-slate-500">Profile: </span>
                    {sanitizeDeployText(
                      fieldOf(sandboxUpPreview, "Profile", "profile"),
                    ) || "default profile"}
                  </div>
                  <div>
                    <span className="text-slate-500">Recreate: </span>
                    {yesNo(boolOf(sandboxUpPreview, "Recreate", "recreate"))}
                  </div>
                  <div>
                    <span className="text-slate-500">Existing: </span>
                    {yesNo(boolOf(sandboxUpPreview, "Exists", "exists"))}
                  </div>
                  <label
                    htmlFor="sandboxUpConfirm"
                    className="flex items-center gap-2 pt-1 font-medium"
                  >
                    <input
                      id="sandboxUpConfirm"
                      data-testid="sandbox-up-confirm"
                      type="checkbox"
                      checked={sandboxUpConfirm}
                      onChange={(event) =>
                        setSandboxUpConfirm(event.target.checked)
                      }
                    />
                    I have read the preview and want this sandbox started
                  </label>
                </div>
              ) : null}
              <div>
                <button
                  type="button"
                  data-testid="sandbox-up-apply"
                  className="px-3 py-1.5 rounded-md text-xs font-semibold border border-border-primary hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={
                    sandboxUpBusy || !sandboxUpPreview || !sandboxUpConfirm
                  }
                  onClick={() => void sandboxUpRun()}
                >
                  Start sandbox
                </button>
              </div>
              {sandboxUpOutcome ? (
                <div
                  data-testid="sandbox-up-result"
                  className={
                    sandboxUpCancelled
                      ? "text-xs text-amber-600 dark:text-amber-300"
                      : "text-xs text-text-secondary"
                  }
                >
                  {sandboxUpCancelled ? "Sandbox start was cancelled. " : ""}
                  {sandboxUpOutcome}
                </div>
              ) : null}
            </div>
            <div className="space-y-3 border-t border-border-primary pt-4">
              <div className="flex items-center gap-3 flex-wrap">
                <label
                  htmlFor="sandboxPurge"
                  className="flex items-center gap-2 text-xs"
                >
                  <input
                    id="sandboxPurge"
                    data-testid="sandbox-purge"
                    type="checkbox"
                    checked={sandboxPurge}
                    onChange={(event) => setSandboxPurge(event.target.checked)}
                  />
                  Purge
                </label>
                <label
                  htmlFor="sandboxVolumes"
                  className="flex items-center gap-2 text-xs"
                >
                  <input
                    id="sandboxVolumes"
                    data-testid="sandbox-volumes"
                    type="checkbox"
                    checked={sandboxVolumes}
                    onChange={(event) =>
                      setSandboxVolumes(event.target.checked)
                    }
                  />
                  Volumes
                </label>
                <button
                  type="button"
                  data-testid="sandbox-down-preview"
                  className="px-3 py-1.5 rounded-md text-xs font-semibold border border-border-primary hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={sandboxDownBusy}
                  onClick={() => void previewSandboxDownRun()}
                >
                  Preview removal
                </button>
              </div>
              {sandboxDownPreview ? (
                <div
                  className="rounded-lg border border-border-primary p-3 text-xs space-y-1"
                  data-testid="sandbox-down-preview-output"
                >
                  <div>
                    <span className="text-slate-500">Container: </span>
                    {sanitizeDeployText(
                      fieldOf(sandboxDownPreview, "Container", "container"),
                    ) || "no container"}
                  </div>
                  <div>
                    <span className="text-slate-500">Profile: </span>
                    {sanitizeDeployText(
                      fieldOf(sandboxDownPreview, "Profile", "profile"),
                    ) || "unknown"}
                  </div>
                  <div>
                    <span className="text-slate-500">Purge: </span>
                    {yesNo(boolOf(sandboxDownPreview, "Purge", "purge"))}
                  </div>
                  <div>
                    <span className="text-slate-500">Volumes: </span>
                    {yesNo(boolOf(sandboxDownPreview, "Volumes", "volumes"))}
                  </div>
                  <div>
                    <span className="text-slate-500">Existing: </span>
                    {yesNo(boolOf(sandboxDownPreview, "Exists", "exists"))}
                  </div>
                  <label
                    htmlFor="sandboxDownConfirm"
                    className="flex items-center gap-2 pt-1 font-medium"
                  >
                    <input
                      id="sandboxDownConfirm"
                      data-testid="sandbox-down-confirm"
                      type="checkbox"
                      checked={sandboxDownConfirm}
                      onChange={(event) =>
                        setSandboxDownConfirm(event.target.checked)
                      }
                    />
                    I have read the preview and want this sandbox removed
                  </label>
                </div>
              ) : null}
              <div>
                <button
                  type="button"
                  data-testid="sandbox-down-apply"
                  className="px-3 py-1.5 rounded-md text-xs font-semibold border border-red-500/40 text-red-600 dark:text-red-300 hover:bg-red-500/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={
                    sandboxDownBusy ||
                    !sandboxDownPreview ||
                    !sandboxDownConfirm
                  }
                  onClick={() => void sandboxDownRun()}
                >
                  Remove sandbox
                </button>
              </div>
              {sandboxDownOutcome ? (
                <div
                  data-testid="sandbox-down-result"
                  className={
                    sandboxDownCancelled
                      ? "text-xs text-amber-600 dark:text-amber-300"
                      : "text-xs text-text-secondary"
                  }
                >
                  {sandboxDownCancelled
                    ? "Sandbox removal was cancelled. "
                    : ""}
                  {sandboxDownOutcome}
                </div>
              ) : null}
            </div>
            <div className="space-y-2 border-t border-border-primary pt-4">
              <div className="flex items-center gap-3 flex-wrap">
                <button
                  type="button"
                  data-testid="sandbox-ssh"
                  className="px-3 py-1.5 rounded-md text-xs font-semibold border border-border-primary hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  disabled={sandboxSSHBusy}
                  onClick={() => void loadSandboxSSH()}
                >
                  Show connection info
                </button>
                <button
                  type="button"
                  data-testid="sandbox-ssh-open"
                  className="px-3 py-1.5 rounded-md text-xs font-semibold border border-border-primary hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  onClick={() => void openSandboxSSH()}
                >
                  Open shell in terminal
                </button>
              </div>
              {sandboxSSH ? (
                <div
                  className="rounded-lg border border-border-primary p-3 text-xs font-mono wrap-break-word"
                  data-testid="sandbox-ssh-output"
                >
                  {sanitizeDeployText(fieldOf(sandboxSSH, "Command", "command")) ||
                    "No sandbox shell available."}
                </div>
              ) : null}
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
