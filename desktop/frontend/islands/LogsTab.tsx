import { useCallback, useEffect, useRef, useState } from "react";
import { hasEventRuntime, onEvent } from "../services/events.js";
import {
  buildLogFilename,
  downloadTextAsFile,
  filterLogsText,
  resolveLogTarget,
  resolveServiceTargets,
} from "../modules/logs.js";
import { getState, setState } from "../state/store.js";
import { useStore } from "../state/useStore.js";

type LogTarget = { project: string; service: string; severity: string; query: string };
type LogsBridge = {
  getLogsForService(project: string, service: string, lines?: number): Promise<unknown>;
  startLogStreamForService(project: string, service: string): Promise<unknown>;
  stopLogStream(): Promise<unknown>;
  saveLogsToFile(content: string, suggestedName: string): Promise<unknown>;
};
type ToastKind = "info" | "success" | "error" | "warning";
/** What main.js keeps calling after the tab moved into React. */
export type LogsIslandApi = {
  refresh(options?: { silent?: boolean }): Promise<void>;
  selectionChanged(): Promise<void>;
};
type Props = {
  bridge: LogsBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  registerController(api: LogsIslandApi): void;
  livePollMs?: number;
};

/** "off" until Live is switched on, then the mode Live actually reached. */
type LiveMode = "off" | "stream" | "poll";

const SERVICE_STRIP_ID = "logServiceSelector";
/** A live selection, read from the store rather than captured in a closure. */
const currentTarget = (): LogTarget => {
  const state = getState();
  return resolveLogTarget({
    project: state.selectedProject,
    service: state.selectedService,
    severity: state.selectedSeverity,
    query: state.logQuery,
  });
};

const serviceChipClass = (active: boolean) =>
  `h-7 px-3 rounded-md text-xs font-semibold whitespace-nowrap border transition-colors ${
    active
      ? "bg-primary/20 text-primary dark:text-white border-primary/30 shadow-[0_0_12px_rgba(13,242,89,0.1)]"
      : "text-text-tertiary dark:text-slate-400 border-transparent hover:text-text-primary dark:hover:text-white hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all"
  }`;

const severityChipClass = (active: boolean) =>
  `h-7 px-3 text-[10px] font-bold uppercase tracking-wide rounded-md border transition-colors ${
    active
      ? "bg-primary/20 text-primary dark:text-white border-primary/30"
      : "bg-surface-secondary dark:bg-surface-secondary text-text-tertiary dark:text-slate-400 border-transparent hover:bg-slate-100 dark:hover:bg-surface-primary hover:text-text-primary dark:hover:text-white transition-all"
  }`;

const SEVERITIES: Array<{ value: string; label: string }> = [
  { value: "all", label: "All" },
  { value: "error", label: "Error" },
  { value: "warn", label: "Warn" },
];

/**
 * The logs tab. The island owns the whole subtree the vanilla tab used to inject
 * into #tab-logs: the markup mirrors it element for element (no look change),
 * minus the data-action attributes the global delegate used to route (D5), and
 * the two things the vanilla controller could not clean up after - the live
 * interval and the logs:* subscriptions - are now React cleanup.
 *
 * Selection stays in the shared store (D6) because main.js still drives it:
 * switchTab, openServiceContext and refreshServiceSelector all write
 * selectedService, and a mounted island that captured its selection in a
 * closure would miss them. Reads render through useStore(); imperative paths
 * read the store directly, because main.js calls refresh() in the same
 * synchronous block as the setState that precedes it, before React re-renders.
 */
export function LogsTab({ bridge, onStatus, onToast, registerController, livePollMs = 2000 }: Props) {
  const state = useStore();
  const [lines, setLines] = useState<string[]>([]);
  const [notice, setNotice] = useState("Select an environment to view logs.");
  const [liveMode, setLiveMode] = useState<LiveMode>("off");
  const viewport = useRef<HTMLDivElement | null>(null);

  const target = resolveLogTarget({
    project: state.selectedProject,
    service: state.selectedService,
    severity: state.selectedSeverity,
    query: state.logQuery,
  });
  const { targets } = resolveServiceTargets(
    state.environments,
    state.selectedProject,
    state.selectedService,
  );

  const load = useCallback(async () => {
    const { project, service } = currentTarget();
    if (!project) {
      setLines([]);
      setNotice("Select an environment to view logs.");
      return;
    }
    setNotice("Loading logs...");
    try {
      // Two arguments, exactly as the vanilla controller called it: the bridge
      // owns the tail default (1000 lines), so the backend sees the same
      // request it saw before the migration.
      const raw = await bridge.getLogsForService(project, service);
      setLines(String(raw ?? "").split("\n"));
      setNotice("");
    } catch (err) {
      setLines([]);
      setNotice(`Failed to load logs: ${err}`);
    }
  }, [bridge]);

  const startLive = useCallback(async () => {
    const { project, service } = currentTarget();
    if (!project) {
      onStatus("Select an environment to stream logs.");
      return;
    }
    // Optimistic: the button flips as soon as the user asks, and the mode
    // settles to "poll" if the backend stream is not available.
    setLiveMode("stream");
    if (bridge.startLogStreamForService && hasEventRuntime()) {
      try {
        await bridge.startLogStreamForService(project, service);
        return;
      } catch (_err) {
        // Fall back to polling.
      }
    }
    setLiveMode("poll");
    await load();
  }, [bridge, load, onStatus]);

  const stopLive = useCallback(() => {
    setLiveMode("off");
  }, []);

  const toggleLive = useCallback(async () => {
    if (liveMode !== "off") {
      stopLive();
      return;
    }
    await startLive();
  }, [liveMode, startLive, stopLive]);

  const selectService = useCallback(
    async (service: string) => {
      // Write the selection before anything reads it: the reload below resolves
      // the target from the store, so the order matters.
      setState({ selectedService: service });
      if (liveMode !== "off") {
        stopLive();
        await startLive();
        return;
      }
      await load();
    },
    [liveMode, load, startLive, stopLive],
  );

  const clearLogs = useCallback(() => {
    stopLive();
    setLines([]);
    setNotice("");
    onStatus("Logs cleared.");
    onToast("Logs cleared successfully.", "success");
  }, [onStatus, onToast, stopLive]);

  const downloadLogs = useCallback(async () => {
    const output = lines.join("\n").trim();
    if (!output) {
      onStatus("No logs available to download.");
      onToast("No logs available to download.", "warning");
      return;
    }
    const { project, service } = currentTarget();
    const filename = buildLogFilename({ scope: "environment", project, service });

    let nativeExportError: unknown = null;
    if (bridge.saveLogsToFile) {
      try {
        const response = await bridge.saveLogsToFile(output, filename);
        const message = String(response || "").trim();
        if (message.toLowerCase().includes("cancelled")) {
          onStatus(message || "Log export cancelled.");
          return;
        }
        onStatus(message || "Logs downloaded successfully.");
        onToast("Logs downloaded successfully.", "success");
        return;
      } catch (err) {
        nativeExportError = err;
      }
    }

    const downloaded = downloadTextAsFile(output, filename);
    if (!downloaded) {
      onStatus(
        nativeExportError !== null
          ? `Failed to download logs: ${nativeExportError}`
          : "Failed to download logs.",
      );
      onToast("Failed to download logs.", "error");
      return;
    }
    onStatus("Logs downloaded successfully.");
    onToast("Logs downloaded successfully.", "success");
  }, [bridge, lines, onStatus, onToast]);

  // main.js keeps its own triggers (tab switch, dashboard refresh, project
  // change) rather than the island watching the store, so no path fetches the
  // same buffer twice.
  const selectionChanged = useCallback(async () => {
    if (liveMode !== "off") {
      stopLive();
      await startLive();
      return;
    }
    await load();
  }, [liveMode, load, startLive, stopLive]);

  useEffect(() => {
    registerController({ refresh: load, selectionChanged });
  }, [registerController, load, selectionChanged]);

  // The first value never waits for main.js's first refreshDashboard, which can
  // run before React has mounted at all.
  useEffect(() => {
    void load();
  }, [load]);

  // One effect owns the whole live lifecycle, so unmounting is what stops the
  // poll and the backend stream (Review Focus 2).
  useEffect(() => {
    if (liveMode === "off") return undefined;
    const timer =
      liveMode === "poll" ? setInterval(() => void load(), livePollMs) : null;
    return () => {
      if (timer !== null) clearInterval(timer);
      void Promise.resolve(bridge.stopLogStream()).catch(() => {});
    };
  }, [liveMode, livePollMs, load, bridge]);

  useEffect(() => {
    if (!hasEventRuntime()) return undefined;
    const offs = [
      onEvent("logs:line", (line: unknown) => {
        setLines((prev) => [...prev, String(line ?? "")]);
      }),
      onEvent("logs:status", (message: unknown) => {
        const text = String(message ?? "").trim();
        if (!text) return;
        onStatus(text);
        onToast(text, "success");
      }),
      onEvent("logs:error", (message: unknown) => {
        const text =
          message && typeof message === "object"
            ? String((message as { message?: unknown }).message ?? "")
            : String(message ?? "").trim();
        if (!text) return;
        onStatus(text);
        onToast(text, "error");
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [onStatus, onToast]);

  // Only the buffer changes scroll the pane: filtering leaves the user where
  // they were, which is what the vanilla controller did.
  useEffect(() => {
    const el = viewport.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines]);

  const filtered = filterLogsText(lines.join("\n"), target.severity, target.query);
  const output = notice || filtered || "No logs match the current filters.";

  return (
    <div className="px-6 lg:px-10 py-6 max-w-[1248px] w-full mx-auto flex-1 flex flex-col gap-6 overflow-hidden h-full">
      <div className="flex-1 flex flex-col gap-4 overflow-hidden h-full min-h-0">
        <div className="flex-1 flex flex-col rounded-xl border border-slate-200 dark:border-border-primary bg-white dark:bg-background-primary overflow-hidden shadow-lg relative">
          <div className="p-3 border-b border-slate-200 dark:border-border-primary bg-slate-50 dark:bg-surface-primary">
            <div className="flex flex-wrap items-center justify-between gap-3 min-h-[40px]">
              <div className="flex items-center gap-3 min-w-0 h-full">
                <div className="flex items-center justify-center size-8 bg-primary/10 rounded-lg text-primary">
                  <span className="material-symbols-outlined text-xl">receipt_long</span>
                </div>
                <div className="flex items-center gap-3">
                  <h3 className="text-sm font-bold text-slate-800 dark:text-white leading-none">Logs</h3>
                  <span className="text-[10px] uppercase tracking-wider text-primary bg-primary/10 border border-primary/20 px-2.5 py-0.5 rounded-full font-bold leading-none">
                    Live Stream
                  </span>
                </div>
              </div>
              <div className="flex items-center gap-2 ml-auto h-full">
                <div className="relative flex items-center">
                  <span className="absolute left-2.5 flex items-center">
                    <span className="material-symbols-outlined text-text-tertiary text-base">search</span>
                  </span>
                  <input
                    id="logSearch"
                    data-testid="log-search"
                    className="bg-surface-secondary text-xs text-text-primary dark:text-white pl-8 pr-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden placeholder-text-tertiary w-44 md:w-56"
                    placeholder="Filter logs..."
                    type="text"
                    value={target.query}
                    onChange={(event) => setState({ logQuery: event.target.value })}
                  />
                </div>
                <button
                  type="button"
                  data-testid="clear-logs"
                  className="size-9 flex items-center justify-center rounded-md text-primary hover:text-primary hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all"
                  title="Clear Logs"
                  onClick={clearLogs}
                >
                  <span className="material-symbols-outlined text-[18px]">block</span>
                </button>
                <button
                  type="button"
                  data-testid="download-logs"
                  className="size-9 flex items-center justify-center rounded-md text-text-tertiary dark:text-primary hover:text-text-primary dark:hover:text-white hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all"
                  title="Download Logs"
                  onClick={() => void downloadLogs()}
                >
                  <span className="material-symbols-outlined text-[18px]">download</span>
                </button>
              </div>
            </div>
            <div className="mt-3 flex flex-col gap-2 xl:flex-row xl:items-center xl:justify-between">
              <div className="flex flex-col gap-2 min-w-0 flex-1 lg:flex-row lg:items-center">
                <div className="flex items-center gap-2 bg-surface-secondary dark:bg-surface-secondary rounded-lg p-1.5 border border-border-primary min-w-0 flex-1">
                  <span className="h-7 inline-flex items-center leading-none text-[10px] uppercase tracking-wide text-text-tertiary px-1 shrink-0">
                    Service
                  </span>
                  <div
                    id={SERVICE_STRIP_ID}
                    className="flex items-center gap-1 min-w-0 flex-1 overflow-x-auto service-strip-scroll"
                  >
                    {targets.map((service) => (
                      <button
                        key={service}
                        type="button"
                        data-testid={`service-${service}`}
                        data-service={service}
                        className={serviceChipClass(service === target.service)}
                        onClick={() => void selectService(service)}
                      >
                        {service}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="flex items-center gap-2 bg-surface-secondary dark:bg-surface-secondary rounded-lg p-1.5 border border-border-primary shrink-0">
                  <span className="text-[10px] uppercase tracking-wide text-text-tertiary px-1 shrink-0">
                    Severity
                  </span>
                  <div id="logSeverity" className="flex gap-1">
                    {SEVERITIES.map((severity) => (
                      <button
                        key={severity.value}
                        type="button"
                        data-testid={`severity-${severity.value}`}
                        data-severity={severity.value}
                        className={severityChipClass(severity.value === target.severity)}
                        onClick={() => setState({ selectedSeverity: severity.value })}
                      >
                        {severity.label}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <button
                  id="toggleLive"
                  type="button"
                  data-testid="toggle-live"
                  className={
                    liveMode === "off"
                      ? "h-8 px-3 rounded-md text-xs font-semibold text-text-tertiary dark:text-slate-400 hover:text-text-primary dark:hover:text-white hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all"
                      : "h-8 px-3 rounded-md text-xs font-semibold bg-primary text-slate-900 hover:bg-primary/90 transition-colors"
                  }
                  onClick={() => void toggleLive()}
                >
                  {liveMode === "off" ? "Live: Off" : "Live: On"}
                </button>
                <button
                  type="button"
                  data-testid="refresh-logs"
                  className="h-8 px-3 rounded-md text-xs font-semibold text-text-tertiary dark:text-slate-400 hover:text-text-primary dark:hover:text-white hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all"
                  onClick={() => void load()}
                >
                  Refresh
                </button>
              </div>
            </div>
          </div>
          <div
            id="logOutputViewport"
            ref={viewport}
            className="flex-1 overflow-y-auto px-4 pb-4 pt-2 terminal-text text-xs bg-slate-900 dark:bg-background-primary custom-scrollbar log-pane-scroll text-slate-300 dark:text-slate-300"
          >
            <pre id="logOutput" className="m-0 font-mono whitespace-pre-wrap">{output}</pre>
          </div>
        </div>
      </div>
    </div>
  );
}
