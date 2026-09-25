import { useCallback, useEffect, useRef, useState } from "react";
import { hasEventRuntime, onEvent } from "../services/events.js";
import {
  buildLogFilename,
  downloadTextAsFile,
  filterLogsText,
  normalizeLogSeverity,
  severityChipClass,
} from "../modules/logs.js";
import { buildEmptyGlobalLogMessage } from "../modules/global-services.js";
import { getState, setState } from "../state/store.js";
import { useStore } from "../state/useStore.js";
import type { GlobalServiceRow } from "./GlobalServicesList.tsx";

type GlobalLogsBridge = {
  getGlobalServiceLogs(serviceId: string, tail: number): Promise<string>;
  startGlobalServiceLogStream?(serviceId: string): Promise<unknown>;
  stopGlobalServiceLogStream(): Promise<unknown>;
  saveLogsToFile?(contents: string, name: string): Promise<string>;
};
type ToastKind = "info" | "success" | "error" | "warning";
/** What main.js keeps calling after the panel moved into React. */
export type GlobalLogsIslandApi = {
  refreshLogs(): Promise<void>;
  stopLive(options?: { skipBridge?: boolean }): Promise<void>;
};
type Props = {
  bridge: GlobalLogsBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  onFeedback(message: string, tone?: string): void;
  registerApi(api: GlobalLogsIslandApi | null): void;
  pollMs?: number;
};

const SEVERITIES = [
  { value: "all", label: "All" },
  { value: "error", label: "Error" },
  { value: "warn", label: "Warn" },
];

/**
 * The global-services log pane. The markup mirrors the static panel in
 * index.html element for element and class for class (no look change), minus
 * the data-action attributes the global delegate resolved (D5).
 *
 * It owns the three things the vanilla controller kept in closure variables and
 * never gave back: the raw buffer, the 2s poll and the three `global-logs:*`
 * subscriptions. All three are React state and effects now, so unmounting the
 * panel is what stops them - the vanilla's `pollTimer` and its subscriptions
 * outlived every re-render and had no owner to stop them (Review Focus 2).
 *
 * The empty-pane copy depends on which service the LAST load asked for, not on
 * the current selection: the vanilla showed the static "Select a global service
 * to view logs." until something called renderLogOutput, which is what `loaded`
 * reproduces. The selection itself is watched in the store, so the card list
 * beside this panel only has to write `selectedGlobalService`.
 */
export function GlobalLogsPanel({
  bridge,
  onStatus,
  onToast,
  onFeedback,
  registerApi,
  pollMs = 2000,
}: Props) {
  const state = useStore();
  const [raw, setRaw] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  /** True only while the poll fallback is driving this pane. */
  const [polling, setPolling] = useState(false);

  const viewportRef = useRef<HTMLDivElement | null>(null);
  const forceScroll = useRef(false);
  const firstSelection = useRef(true);

  const selectedId = String(state.selectedGlobalService || "")
    .trim()
    .toLowerCase();
  const severity = normalizeLogSeverity(state.globalLogSeverity || "all");
  const query = String(state.globalLogQuery || "");
  const live = Boolean(state.globalLiveLogsEnabled);
  const services = state.globalServices as GlobalServiceRow[];
  const serviceName =
    services.find((service) => service.id === selectedId)?.name || "Select service";

  // The selection is read from the store rather than a closure: main.js calls
  // refreshLogs() in the same synchronous block as the setState that selects a
  // service, before React has re-rendered with it.
  const refreshLogs = useCallback(async () => {
    const serviceID = String(getState().selectedGlobalService || "").trim();
    setLoaded(true);
    if (!serviceID) {
      setRaw("");
      forceScroll.current = true;
      return;
    }
    setLoading(true);
    try {
      const text = String(
        (await bridge.getGlobalServiceLogs(serviceID, 300)) || "",
      );
      setRaw(text);
    } catch (err) {
      setRaw(`Failed to load logs: ${err}`);
    } finally {
      setLoading(false);
      forceScroll.current = true;
    }
  }, [bridge]);

  const stopLive = useCallback(
    async ({ skipBridge = false } = {}) => {
      setPolling(false);
      setState({ globalLiveLogsEnabled: false });
      onFeedback("Live log stream paused.", "info");
      if (skipBridge) {
        return;
      }
      try {
        await bridge.stopGlobalServiceLogStream();
      } catch (_err) {
        // Polling fallback mode may not hold stream state.
      }
    },
    [bridge, onFeedback],
  );

  const startLive = useCallback(async () => {
    const serviceID = String(getState().selectedGlobalService || "").trim();
    if (!serviceID) {
      onStatus("Select a global service to stream logs.");
      onFeedback("Select a service first to stream logs.", "warning");
      return;
    }
    setState({ globalLiveLogsEnabled: true });
    const name =
      (getState().globalServices as GlobalServiceRow[] | undefined)?.find(
        (item) => item.id === serviceID,
      )?.name || serviceID;
    onFeedback(`Streaming live logs for ${name}.`, "info");

    if (bridge.startGlobalServiceLogStream && hasEventRuntime()) {
      try {
        await bridge.startGlobalServiceLogStream(serviceID);
        setPolling(false);
        return;
      } catch (_err) {
        // Fall through to the polling fallback.
      }
    }

    await refreshLogs();
    setPolling(true);
  }, [bridge, onFeedback, onStatus, refreshLogs]);

  const toggleLive = useCallback(async () => {
    if (Boolean(getState().globalLiveLogsEnabled)) {
      await stopLive();
      return;
    }
    await startLive();
  }, [startLive, stopLive]);

  const clearLogs = useCallback(async () => {
    await stopLive();
    setRaw("");
    setLoaded(true);
    forceScroll.current = true;
    onStatus("Global service logs cleared.");
    onFeedback("Global service logs cleared.", "info");
  }, [onFeedback, onStatus, stopLive]);

  const downloadLogs = useCallback(async () => {
    const output = String(raw || "").trim();
    if (!output) {
      onStatus("No global logs available to download.");
      onToast("No global logs available to download.", "warning");
      return;
    }

    const current = getState();
    const selected = String(current.selectedGlobalService || "").trim();
    const service = (
      current.globalServices as GlobalServiceRow[] | undefined
    )?.find((item) => item.id === selected);
    const filename = buildLogFilename({
      scope: "global",
      project: "services",
      service: service?.name || selected || "all",
    });

    let nativeExportError: unknown = null;
    if (bridge.saveLogsToFile) {
      try {
        const response = await bridge.saveLogsToFile(output, filename);
        const message = String(response || "").trim();
        if (message.toLowerCase().includes("cancelled")) {
          onStatus(message || "Global log export cancelled.");
          onFeedback(message || "Global log export cancelled.", "info");
          return;
        }
        onStatus(message || "Global logs downloaded successfully.");
        onToast("Global logs downloaded successfully.", "success");
        onFeedback(message || "Global logs downloaded successfully.", "success");
        return;
      } catch (err) {
        nativeExportError = err;
      }
    }

    const downloaded = downloadTextAsFile(output, filename);
    if (!downloaded) {
      onStatus(
        nativeExportError !== null
          ? `Failed to download global logs: ${nativeExportError}`
          : "Failed to download global logs.",
      );
      onToast("Failed to download global logs.", "error");
      onFeedback("Failed to download global logs.", "error");
      return;
    }
    onStatus("Global logs downloaded successfully.");
    onToast("Global logs downloaded successfully.", "success");
    onFeedback("Global logs downloaded successfully.", "success");
  }, [bridge, onFeedback, onStatus, onToast, raw]);

  useEffect(() => {
    registerApi({ refreshLogs, stopLive });
    // Handing the API back on unmount is what stops main.js's own call
    // sites from reaching an island that no longer exists: the island's timers
    // die with it, but the closures main.js kept would not.
    return () => registerApi(null);
  }, [registerApi, refreshLogs, stopLive]);

  // One effect owns the poll, so unmounting the pane is what stops it.
  useEffect(() => {
    if (!polling) {
      return undefined;
    }
    const timer = setInterval(() => void refreshLogs(), pollMs);
    return () => clearInterval(timer);
  }, [polling, pollMs, refreshLogs]);

  // The three runtime events die with the panel too; the vanilla subscribed once
  // at controller creation and never unsubscribed.
  useEffect(() => {
    if (!hasEventRuntime()) {
      return undefined;
    }
    const offs = [
      onEvent("global-logs:line", (line) => {
        const value = String(line ?? "").trim();
        if (!value) {
          return;
        }
        setRaw((prev) => (prev ? `${prev}\n${value}` : value));
        setLoaded(true);
      }),
      onEvent("global-logs:status", (message) => {
        const value = String(message ?? "").trim();
        if (value) {
          onStatus(value);
        }
      }),
      onEvent("global-logs:error", (message) => {
        const value = String(message ?? "").trim();
        if (!value) {
          return;
        }
        onStatus(value);
        onToast(value, "error");
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [onStatus, onToast]);

  // A changed selection reloads this pane (or restarts the stream). The first
  // run is skipped on purpose: the vanilla left the pane alone until something
  // asked it to load, so mounting is not a load.
  useEffect(() => {
    if (firstSelection.current) {
      firstSelection.current = false;
      return;
    }
    if (Boolean(getState().globalLiveLogsEnabled)) {
      void stopLive({ skipBridge: true }).then(() => startLive());
      return;
    }
    void refreshLogs();
  }, [selectedId, refreshLogs, startLive, stopLive]);

  // Only a forced reload or a live stream pulls the pane to the bottom.
  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) {
      return;
    }
    if (!forceScroll.current && !live) {
      return;
    }
    forceScroll.current = false;
    viewport.scrollTop = viewport.scrollHeight;
  }, [raw, loading, live]);

  const filtered = filterLogsText(raw, severity, query).trim();
  const output = !loaded
    ? "Select a global service to view logs."
    : loading
      ? "Loading logs..."
      : raw.trim()
        ? filtered || "No logs match the current filters."
        : buildEmptyGlobalLogMessage(selectedId, services);

  return (
    <>
      <h3 className="text-sm font-semibold text-primary">Logs</h3>
      <div className="rounded-xl border border-border-primary bg-background-primary overflow-hidden flex-1 min-h-0 flex flex-col">
        <div className="p-3 border-b border-border-primary bg-surface-primary">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-4 min-w-0 font-bold">
              <span className="material-symbols-outlined text-primary text-[18px]">
                receipt_long
              </span>
              <span className="text-xs font-bold text-text-primary dark:text-white">
                Global Logs
              </span>
              <span
                id="globalLogServiceName"
                className="text-[10px] text-text-tertiary truncate"
              >
                {serviceName}
              </span>
            </div>
            <div className="flex items-center gap-2 ml-auto">
              <div className="relative">
                <span className="absolute inset-y-0 left-2 flex items-center">
                  <span className="material-symbols-outlined text-text-tertiary text-base">
                    search
                  </span>
                </span>
                <input
                  id="globalLogSearch"
                  data-testid="global-log-search"
                  className="bg-surface-secondary text-xs text-text-primary dark:text-white pl-8 pr-3 py-2 rounded-md border border-border-primary focus:border-primary/50 focus:outline-hidden placeholder-text-tertiary w-40 md:w-52"
                  placeholder="Filter logs..."
                  type="text"
                  value={query}
                  onChange={(event) =>
                    setState({ globalLogQuery: event.target.value })
                  }
                />
              </div>
              <button
                type="button"
                data-testid="clear-global-logs"
                className="p-2 rounded-md text-primary hover:text-primary hover:bg-primary/10 transition-colors"
                title="Clear Logs"
                onClick={() => void clearLogs()}
              >
                <span className="material-symbols-outlined text-lg">block</span>
              </button>
              <button
                type="button"
                data-testid="download-global-logs"
                className="p-2 rounded-md text-primary hover:text-primary hover:bg-primary/10 transition-colors"
                title="Download Logs"
                onClick={() => void downloadLogs()}
              >
                <span className="material-symbols-outlined text-lg">download</span>
              </button>
            </div>
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2 bg-surface-secondary rounded-lg p-1.5 border border-border-primary shrink-0">
              <span className="text-[10px] uppercase tracking-wide text-text-tertiary px-1 shrink-0">
                Severity
              </span>
              <div id="globalLogSeverity" className="flex gap-1">
                {SEVERITIES.map((entry) => (
                  <button
                    key={entry.value}
                    type="button"
                    data-testid={`global-severity-${entry.value}`}
                    data-severity={entry.value}
                    className={severityChipClass(entry.value === severity)}
                    onClick={() => setState({ globalLogSeverity: entry.value })}
                  >
                    {entry.label}
                  </button>
                ))}
              </div>
            </div>
            <div className="flex items-center gap-1.5">
              <button
                type="button"
                id="globalToggleLive"
                data-testid="global-toggle-live"
                className="h-7 px-2.5 rounded-md text-[10px] font-semibold bg-primary/20 dark:bg-[#2e573a] text-primary dark:text-white hover:bg-primary-hover hover:text-white transition-colors"
                onClick={() => void toggleLive()}
              >
                {live ? "Live: On" : "Live: Off"}
              </button>
              <button
                type="button"
                data-testid="refresh-global-logs"
                className="h-7 px-2.5 rounded-md text-[10px] font-semibold text-primary dark:text-primary hover:bg-primary/10 dark:hover:bg-[#2e573a] transition-colors"
                onClick={() => void refreshLogs()}
              >
                Refresh
              </button>
            </div>
          </div>
        </div>
        <div
          id="globalLogViewport"
          ref={viewportRef}
          className="flex-1 min-h-0 overflow-y-auto p-3 custom-scrollbar log-pane-scroll"
        >
          <pre
            id="globalLogOutput"
            className="m-0 font-mono whitespace-pre-wrap wrap-break-word text-[11px] text-text-secondary"
          >
            {output}
          </pre>
        </div>
      </div>
    </>
  );
}
