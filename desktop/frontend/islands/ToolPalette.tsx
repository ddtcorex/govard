import { useCallback, useEffect, useRef, useState } from "react";
import { hasEventRuntime, onEvent } from "../services/events.js";
import { useStore } from "../state/useStore.js";
import {
  NO_PROJECT_GUIDANCE,
  applyToolOutputBatch,
  applyToolTerminalEvent,
  buildToolRunPayload,
  currentToolGeneration,
  isToolRunEnabled,
} from "../modules/tool-palette.js";

type ToolArgSpec = {
  name: string;
  label?: string;
  required?: boolean;
  flag?: boolean;
};
type ToolPreset = {
  id: string;
  label?: string;
  display?: string;
  argSpec?: ToolArgSpec[];
};
type ToolHistoryEntry = {
  opId?: string;
  gen?: number;
  preset?: string;
  args?: Record<string, string>;
  timestamp?: string;
  outcome?: string;
};
type ToolBridge = {
  listToolPresets(project: string): Promise<ToolPreset[]>;
  runTool(project: string, presetId: string, args: Record<string, string>): Promise<string>;
  cancelToolRun(opId: string): Promise<boolean>;
  getToolHistory(project: string): Promise<ToolHistoryEntry[]>;
};
type ToastKind = "info" | "success" | "error" | "warning";
/** What main.js keeps calling after the tab moved into React. */
export type ToolPaletteApi = {
  refresh(options?: { silent?: boolean }): Promise<void>;
};
type Props = {
  bridge: ToolBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  registerApi(api: ToolPaletteApi | null): void;
};
type RunState = {
  opId: string;
  gen: number;
  running: boolean;
  finished: boolean;
  outcome: string;
};

const IDLE_RUN: RunState = { opId: "", gen: 0, running: false, finished: false, outcome: "" };
const IDLE_NOTICE = "Pick a preset, fill its arguments, then run.";

const presetLabel = (preset: ToolPreset) =>
  String(preset.label || preset.id || "");
const presetCommand = (preset: ToolPreset) =>
  String(preset.display || preset.id || "");
const specLabel = (spec: ToolArgSpec) => String(spec.label || spec.name || "");
const historyLabel = (entry: ToolHistoryEntry) => {
  const when = String(entry.timestamp || "");
  const preset = String(entry.preset || "run");
  const outcome = String(entry.outcome || "unknown");
  return `${preset} — ${outcome}${when ? ` — ${when}` : ""}`;
};

/**
 * The tool runner palette. It reads the selected project from the shared
 * store, lists the backend's preset allowlist for it, and runs one preset
 * at a time with streamed output. Runs send a preset ID plus an args map
 * only: the exec command stays backend-owned, so the palette cannot be
 * turned into a shell. Terminal events apply through the (op, gen)
 * attribution helper, so a stale receipt never marks the current run, and
 * a finished run never flips back to running.
 */
export function ToolPalette({ bridge, onStatus, onToast, registerApi }: Props) {
  const state = useStore();
  const project = String(state.selectedProject || "");
  const enabled = isToolRunEnabled(project);

  const [presets, setPresets] = useState<ToolPreset[]>([]);
  const [presetId, setPresetId] = useState("");
  const [argValues, setArgValues] = useState<Record<string, string>>({});
  const [history, setHistory] = useState<ToolHistoryEntry[]>([]);
  const [run, setRun] = useState<RunState>(IDLE_RUN);
  const [lines, setLines] = useState<string[]>([]);
  const [notice, setNotice] = useState(IDLE_NOTICE);
  const runRef = useRef<RunState>(IDLE_RUN);
  runRef.current = run;

  const selectedPreset = presets.find((preset) => preset.id === presetId) || null;
  const argSpecs = selectedPreset?.argSpec || [];

  const loadHistory = useCallback(
    async (targetProject: string) => {
      try {
        const entries = await bridge.getToolHistory(targetProject);
        setHistory(Array.isArray(entries) ? entries : []);
      } catch {
        // History is best-effort: a failed load must never block a run.
      }
    },
    [bridge],
  );

  const refresh = useCallback(
    async (options?: { silent?: boolean }) => {
      const target = String(project || "").trim();
      if (!isToolRunEnabled(target)) {
        setPresets([]);
        setPresetId("");
        setArgValues({});
        setHistory([]);
        setNotice(NO_PROJECT_GUIDANCE);
        return;
      }
      try {
        const listed = await bridge.listToolPresets(target);
        const next = Array.isArray(listed) ? listed : [];
        setPresets(next);
        setPresetId((prev) => {
          if (prev && next.some((preset) => preset.id === prev)) return prev;
          return next.length > 0 ? String(next[0].id || "") : "";
        });
        setNotice((prev) => (prev === NO_PROJECT_GUIDANCE ? IDLE_NOTICE : prev));
        await loadHistory(target);
      } catch (err) {
        const message = String(err instanceof Error ? err.message : err);
        setNotice(message || "Could not load tool presets.");
        if (!options?.silent) {
          onStatus(message || "Could not load tool presets.");
        }
      }
    },
    [bridge, loadHistory, onStatus, project],
  );

  useEffect(() => {
    setRun(IDLE_RUN);
    setLines([]);
    void refresh({ silent: true });
  }, [refresh]);

  useEffect(() => {
    registerApi({ refresh });
    // Handing the API back on unmount is what stops main.js's own call
    // sites from reaching an island that no longer exists.
    return () => registerApi(null);
  }, [registerApi, refresh]);

  useEffect(() => {
    if (!hasEventRuntime()) return undefined;
    const offs = [
      onEvent("tool:output", (payload: unknown) => {
        const data = (payload || {}) as { opId?: unknown; text?: unknown };
        const current = runRef.current;
        if (!current.running || String(data.opId ?? "") !== current.opId) return;
        setLines((prev) => applyToolOutputBatch(prev, data, current.opId));
      }),
      onEvent("tool:completed", (payload: unknown) => {
        settleRun(payload, "tool:completed");
      }),
      onEvent("tool:failed", (payload: unknown) => {
        settleRun(payload, "tool:failed");
      }),
    ];
    return () => offs.forEach((off) => off());
    // settleRun reads runRef, so subscribing once is correct: resubscribing
    // per run would drop events between renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const settleRun = (payload: unknown, event: string) => {
    const current = runRef.current;
    if (!current.running) return;
    const data = (payload || {}) as { opId?: unknown; gen?: unknown };
    const next = applyToolTerminalEvent(
      { opId: current.opId, gen: current.gen, finished: false, outcome: "" },
      { opId: data.opId, gen: data.gen, event },
    );
    if (!next.finished) return;
    const settled: RunState = {
      opId: current.opId,
      gen: next.gen,
      running: false,
      finished: true,
      outcome: next.outcome,
    };
    setRun(settled);
    setLines((prev) => [
      ...prev,
      "",
      next.outcome === "success" ? "[SUCCESS] Tool run completed" : "[FAILED] Tool run failed",
    ]);
    const target = String(project || "").trim();
    if (target) {
      void loadHistory(target);
    }
    if (next.outcome === "success") {
      onToast("Tool run completed", "success");
    } else {
      onToast("Tool run failed", "error");
    }
  };

  const startRun = async () => {
    const target = String(project || "").trim();
    if (!isToolRunEnabled(target) || run.running) return;
    const payload = buildToolRunPayload(presetId, argValues);
    if (!payload.presetId) {
      onToast("Pick a tool preset first.", "warning");
      return;
    }
    setNotice("");
    try {
      const opId = String(await bridge.runTool(target, payload.presetId, payload.args));
      const started: RunState = { opId, gen: 0, running: true, finished: false, outcome: "" };
      setRun(started);
      setLines([`$ ${presetCommand(selectedPreset || { id: payload.presetId })}`, ""]);
      onStatus(`Tool run started: ${payload.presetId}`);
      // Learn the admitted generation before the terminal can arrive: the
      // backend promotes the history entry before it emits the outcome, so
      // the newest entry for this op is the current generation.
      try {
        const entries = await bridge.getToolHistory(target);
        const list = Array.isArray(entries) ? entries : [];
        setHistory(list);
        const gen = currentToolGeneration(list, opId);
        if (gen > 0) {
          setRun({ ...started, gen });
        }
      } catch {
        // The run streams without attribution until the next refresh.
      }
    } catch (err) {
      const message = String(err instanceof Error ? err.message : err);
      setNotice(message || "Tool run failed to start.");
      onToast(message || "Tool run failed to start.", "error");
    }
  };

  const cancelRun = async () => {
    const current = runRef.current;
    if (!current.running || !current.opId) return;
    try {
      await bridge.cancelToolRun(current.opId);
      onStatus("Tool run cancelling...");
    } catch (err) {
      onToast(String(err instanceof Error ? err.message : err), "error");
    }
  };

  const applyHistoryEntry = (index: number) => {
    const entry = history[index];
    if (!entry) return;
    const id = String(entry.preset || "");
    if (id && presets.some((preset) => preset.id === id)) {
      setPresetId(id);
    }
    const args: Record<string, string> = {};
    for (const [name, value] of Object.entries(entry.args || {})) {
      if (String(value ?? "") !== "***") {
        args[name] = String(value ?? "");
      }
    }
    setArgValues(args);
  };

  return (
    <div
      data-testid="tool-palette"
      className="px-6 lg:px-10 py-6 max-w-[1248px] w-full mx-auto flex-1 flex flex-col gap-6 overflow-hidden h-full"
    >
      <div className="flex-1 flex flex-col gap-4 overflow-hidden h-full min-h-0">
        <div className="flex-1 flex flex-col rounded-xl border border-slate-200 dark:border-border-primary bg-white dark:bg-background-primary overflow-hidden shadow-lg relative">
          <div className="p-3 border-b border-slate-200 dark:border-border-primary bg-slate-50 dark:bg-surface-primary">
            <div className="flex flex-wrap items-center gap-3 min-h-[40px]">
              <div className="flex items-center justify-center size-8 bg-primary/10 rounded-lg text-primary">
                <span className="material-symbols-outlined text-xl">terminal</span>
              </div>
              <h3 className="text-sm font-bold text-slate-800 dark:text-white leading-none">
                Tool Runner
              </h3>
              <select
                data-testid="tool-preset-picker"
                aria-label="Tool preset"
                className="bg-surface-secondary text-xs text-text-primary dark:text-white px-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden"
                value={presetId}
                disabled={!enabled || run.running}
                onChange={(event) => {
                  setPresetId(event.target.value);
                  setArgValues({});
                }}
              >
                {presets.length === 0 && <option value="">No presets</option>}
                {presets.map((preset) => (
                  <option key={preset.id} value={preset.id}>
                    {presetLabel(preset)}
                  </option>
                ))}
              </select>
              <select
                data-testid="tool-history"
                aria-label="Recent tool runs"
                className="bg-surface-secondary text-xs text-text-primary dark:text-white px-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden max-w-56"
                value=""
                disabled={!enabled || history.length === 0}
                onChange={(event) => applyHistoryEntry(Number(event.target.value))}
              >
                <option value="">Recent runs</option>
                {history.map((entry, index) => (
                  <option key={`${String(entry.opId)}:${String(entry.gen)}:${index}`} value={index}>
                    {historyLabel(entry)}
                  </option>
                ))}
              </select>
              <div className="flex items-center gap-2 ml-auto">
                <button
                  type="button"
                  data-testid="tool-run"
                  className="h-9 px-5 bg-primary text-slate-900 rounded-lg text-xs font-bold hover:bg-primary/90 transition-all flex items-center gap-2 shadow-lg shadow-primary/10 active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed"
                  title={!enabled ? NO_PROJECT_GUIDANCE : `Run ${presetId}`}
                  disabled={!enabled || run.running || !presetId}
                  onClick={() => void startRun()}
                >
                  <span className="material-symbols-outlined text-base">play_arrow</span>
                  Run
                </button>
                <button
                  type="button"
                  data-testid="tool-cancel"
                  className="h-9 px-5 bg-red-600 text-white rounded-lg text-xs font-bold hover:bg-red-500 transition-all flex items-center gap-2 shadow-lg shadow-red-500/20 active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed"
                  title="Cancel the running tool"
                  disabled={!run.running}
                  onClick={() => void cancelRun()}
                >
                  <span className="material-symbols-outlined text-base">stop</span>
                  Cancel
                </button>
              </div>
            </div>
            {argSpecs.length > 0 && (
              <div className="flex flex-wrap items-center gap-3 mt-3">
                {argSpecs.map((spec) => (
                  <label
                    key={spec.name}
                    className="flex items-center gap-2 text-xs text-text-secondary dark:text-slate-400"
                  >
                    {specLabel(spec)}
                    {spec.required && <span className="text-red-500">*</span>}
                    <input
                      data-testid={`tool-arg-${spec.name}`}
                      aria-label={specLabel(spec)}
                      className="bg-surface-secondary text-xs text-text-primary dark:text-white px-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden w-44"
                      type="text"
                      value={argValues[spec.name] || ""}
                      disabled={!enabled || run.running}
                      onChange={(event) =>
                        setArgValues((prev) => ({ ...prev, [spec.name]: event.target.value }))
                      }
                    />
                  </label>
                ))}
              </div>
            )}
          </div>
          <div
            data-testid="tool-output-pane"
            className="flex-1 overflow-y-auto p-4 bg-slate-950 text-slate-100 min-h-0"
          >
            {!enabled ? (
              <p className="m-0 text-sm text-slate-400">{NO_PROJECT_GUIDANCE}</p>
            ) : (
              <pre className="m-0 font-mono whitespace-pre-wrap text-xs leading-relaxed">
                {lines.length > 0 ? lines.join("\n") : notice}
              </pre>
            )}
          </div>
          <div className="px-4 py-2 border-t border-slate-200 dark:border-border-primary bg-slate-50 dark:bg-surface-primary flex items-center gap-2 text-[11px] text-text-secondary dark:text-slate-400">
            <span
              data-testid="tool-run-status"
              className="font-mono uppercase tracking-wider truncate"
            >
              {run.running
                ? `Running ${presetId}...`
                : run.finished
                  ? `Last run ${run.outcome}`
                  : enabled
                    ? "Idle"
                    : "No project selected"}
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
