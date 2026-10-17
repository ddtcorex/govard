import { useCallback, useEffect, useRef, useState } from "react";
import { hasEventRuntime, onEvent } from "../services/events.js";
import {
  OPERATIONS_NOTIFICATION_EVENT,
  SYNC_PROGRESS_EVENT,
  isRetryable,
  normalizeHistoryList,
  normalizeOpList,
  rerunHintFor,
} from "../modules/operation-center.js";

type RunningOp = {
  id: string;
  project: string;
  kind: string;
  startedAt: string;
};

type HistoryRecord = {
  id: string;
  project: string;
  kind: string;
  status: string;
  message: string;
  startedAt: string;
  durationMs: number;
};

type OperationsBridge = {
  listOps(): Promise<unknown>;
  listHistory(limit?: number): Promise<unknown>;
  cancelOp(id: string): Promise<unknown>;
  retryOp(id: string): Promise<unknown>;
};

type ToastKind = "info" | "success" | "error" | "warning";

/** What main.js keeps calling after the tab moved into React. */
export type OperationCenterApi = {
  refresh(options?: { silent?: boolean }): Promise<void>;
};

type Props = {
  bridge: OperationsBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  registerApi(api: OperationCenterApi | null): void;
};

const errorMessage = (err: unknown) =>
  err instanceof Error ? err.message : String(err);

/**
 * The operations tab: the visible home for long ops. The running list shows
 * every tracked op with its per-project scope label and a real cancel; the
 * history shows terminal records with Retry on failed entries only and a
 * re-run hint (a fresh run, never a replay) on restart-interrupted ones.
 * Server text renders as text, and every control calls back through the
 * bridge: no data-action attributes (D5), so the global click delegate never
 * sees a click inside this subtree.
 */
export function OperationCenter({ bridge, onStatus, onToast, registerApi }: Props) {
  const [ops, setOps] = useState<RunningOp[]>([]);
  const [records, setRecords] = useState<HistoryRecord[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState<Record<string, boolean>>({});

  /** Bumped by every load: only the newest request may write the lists. */
  const loadRequest = useRef(0);

  const refresh = useCallback(async () => {
    const request = loadRequest.current + 1;
    loadRequest.current = request;
    try {
      const [rawOps, rawHistory] = await Promise.all([
        bridge.listOps(),
        bridge.listHistory(),
      ]);
      if (loadRequest.current !== request) {
        return;
      }
      setOps(normalizeOpList(rawOps).ops);
      setRecords(normalizeHistoryList(rawHistory).records);
    } catch (err) {
      if (loadRequest.current !== request) {
        return;
      }
      onToast(`Could not load operations: ${errorMessage(err)}`, "error");
    } finally {
      if (loadRequest.current === request) {
        setLoaded(true);
      }
    }
  }, [bridge, onToast]);

  const cancelOpAction = useCallback(
    async (id: string) => {
      setBusy((prev) => ({ ...prev, [id]: true }));
      try {
        // False means nothing was running (unknown or already finished):
        // the backend treats that as a harmless no-op, so the center says
        // so instead of erroring.
        const cancelled = await bridge.cancelOp(id);
        if (cancelled) {
          onStatus(`Cancelling ${id}...`);
        } else {
          onToast(`${id} already finished.`, "info");
        }
      } catch (err) {
        onToast(`Could not cancel ${id}: ${errorMessage(err)}`, "error");
      } finally {
        setBusy((prev) => ({ ...prev, [id]: false }));
        void refresh();
      }
    },
    [bridge, onStatus, onToast, refresh],
  );

  const retryAction = useCallback(
    async (record: HistoryRecord) => {
      setBusy((prev) => ({ ...prev, [record.id]: true }));
      try {
        await bridge.retryOp(record.id);
        onStatus(`Retrying ${record.id}...`);
      } catch (err) {
        onToast(`Could not retry ${record.id}: ${errorMessage(err)}`, "error");
      } finally {
        setBusy((prev) => ({ ...prev, [record.id]: false }));
        void refresh();
      }
    },
    [bridge, onStatus, onToast, refresh],
  );

  // The imperative API main.js keeps calling (tab switches refresh through
  // it). Handed back on unmount so a stale reference can never drive a dead
  // island.
  useEffect(() => {
    registerApi({ refresh });
    return () => registerApi(null);
  }, [registerApi, refresh]);

  // The center stays live: progress and terminal notifications refresh both
  // lists. The guard keeps node and preview contexts without a backend from
  // subscribing.
  useEffect(() => {
    if (!hasEventRuntime()) {
      return;
    }
    const offProgress = onEvent(SYNC_PROGRESS_EVENT, () => {
      void refresh();
    });
    const offNotification = onEvent(OPERATIONS_NOTIFICATION_EVENT, () => {
      void refresh();
    });
    return () => {
      offProgress();
      offNotification();
    };
  }, [refresh]);

  // First paint loads both lists; later refreshes arrive through the API and
  // the event subscriptions above.
  useEffect(() => {
    void refresh();
  }, [refresh]);

  return (
    <div className="px-6 lg:px-10 py-6 max-w-[1248px] w-full mx-auto">
      <div className="space-y-8">
        <section aria-label="Running operations">
          <h3 className="text-slate-900 dark:text-white text-lg font-semibold flex items-center gap-2 pt-4">
            <span className="material-symbols-outlined text-primary" aria-hidden="true">sync</span>
            Running
          </h3>
          {!loaded ? (
            <p className="text-sm text-slate-500">Loading operations...</p>
          ) : ops.length === 0 ? (
            <p className="text-sm text-slate-500">No running operations.</p>
          ) : (
            <ul className="space-y-3">
              {ops.map((op) => (
                <li
                  key={op.id}
                  className="glass-panel p-4 rounded-xl border border-border-primary flex items-center gap-4"
                >
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold text-slate-900 dark:text-slate-200 truncate">
                      {op.kind === "" ? "Operation" : op.kind}
                    </p>
                    <p className="text-xs text-text-secondary dark:text-slate-500 truncate">
                      {op.project === "" ? op.id : `${op.project} · ${op.id}`}
                    </p>
                  </div>
                  <button
                    type="button"
                    data-testid="operation-cancel"
                    aria-label={`Cancel ${op.id}`}
                    disabled={Boolean(busy[op.id])}
                    onClick={() => void cancelOpAction(op.id)}
                    className="px-4 py-2 text-xs font-bold rounded-lg border border-slate-200 dark:border-white/10 text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-white/5 transition-all active:scale-95 disabled:opacity-50"
                  >
                    Cancel
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section aria-label="Operation history">
          <h3 className="text-slate-900 dark:text-white text-lg font-semibold flex items-center gap-2 pt-4">
            <span className="material-symbols-outlined text-primary" aria-hidden="true">history</span>
            History
          </h3>
          {!loaded ? (
            <p className="text-sm text-slate-500">Loading history...</p>
          ) : records.length === 0 ? (
            <p className="text-sm text-slate-500">No operations yet.</p>
          ) : (
            <ul className="space-y-3">
              {records.map((record) => {
                const hint = rerunHintFor(record);
                return (
                  <li
                    key={`${record.id}-${record.startedAt}`}
                    className="glass-panel p-4 rounded-xl border border-border-primary flex items-center gap-4"
                  >
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-semibold text-slate-900 dark:text-slate-200 truncate">
                        {record.kind === "" ? "Operation" : record.kind}
                        <span className="ml-2 text-xs font-mono text-text-tertiary">
                          {record.status === "" ? "unknown" : record.status}
                        </span>
                      </p>
                      <p className="text-xs text-text-secondary dark:text-slate-500 truncate">
                        {record.project === "" ? record.id : `${record.project} · ${record.id}`}
                      </p>
                      {record.message !== "" ? (
                        <p className="text-xs text-slate-500 dark:text-slate-400 truncate">
                          {record.message}
                        </p>
                      ) : null}
                      {hint !== "" ? (
                        <p className="text-xs text-slate-500 dark:text-slate-400">{hint}</p>
                      ) : null}
                    </div>
                    {isRetryable(record) ? (
                      <button
                        type="button"
                        data-testid="operation-retry"
                        aria-label={`Retry ${record.id}`}
                        disabled={Boolean(busy[record.id])}
                        onClick={() => void retryAction(record)}
                        className="px-4 py-2 text-xs font-bold rounded-lg bg-primary text-slate-900 hover:scale-[1.02] active:scale-[0.98] transition-all disabled:opacity-50"
                      >
                        Retry
                      </button>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}
