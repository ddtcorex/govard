import { useCallback, useEffect, useRef, useState } from "react";
import { hasEventRuntime, onEvent } from "../services/events.js";
import {
  NO_PROJECT_GUIDANCE,
  applyTransferTerminalEvent,
  classifyQueryMode,
  createTransferRun,
  describeRunningOp,
  isDataActionEnabled,
  isSnapshotRestoreConfirmEnabled,
  isStartBlocked,
  pickDisplayDBInfo,
  snapshotScopeFlags,
} from "../modules/data-tab.js";
import { getState } from "../state/store.js";
import { useStore } from "../state/useStore.js";

type DisplayDBInfo = {
  project: string;
  engine: string;
  host: string;
  port: string;
  database: string;
  user: string;
  hasPassword: boolean;
  displayUrl: string;
};

type SnapshotRow = {
  name: string;
  createdAt: string;
  sizeHuman: string;
  hasDb: boolean;
  hasMedia: boolean;
};

type RestorePreview = {
  previewToken: string;
  project: string;
  snapshot: string;
  willReplaceDb: boolean;
  willReplaceMedia: boolean;
  summary: string;
};

type TransferKind =
  | "dump"
  | "import"
  | "snapshot-create"
  | "snapshot-restore"
  | "snapshot-export";

type TransferState = {
  opId: string;
  gen: number;
  kind: TransferKind;
  label: string;
  detail: string;
  startedAt: number;
  percent: number | null;
  message: string;
  finished: boolean;
  outcome: string;
};

type RestoreScope = "full" | "db" | "media";

type RestorePanel = {
  name: string;
  scope: RestoreScope;
  preview: RestorePreview | null;
  loading: boolean;
};

type OverwritePanel = {
  kind: "dump" | "export";
  destPath: string;
  snapshotName: string;
};

type WriteConfirm = {
  sql: string;
  token: string;
};

type DataBridge = {
  getDBInfo(project: string): Promise<unknown>;
  runDBQuery(
    project: string,
    sql: string,
    allowWrite?: boolean,
    confirmToken?: string,
  ): Promise<unknown>;
  dumpDB(project: string, destPath: string): Promise<string>;
  importDB(project: string, srcPath: string): Promise<string>;
  cancelDBOp(opId: string): Promise<unknown>;
  pickDumpDestination(
    project: string,
  ): Promise<{ path: string; exists: boolean } | null>;
  pickImportSource(): Promise<string>;
  getDBWriteConfirmToken(project: string): Promise<string>;
  listSnapshots(project: string): Promise<unknown>;
  createSnapshot(project: string, name: string): Promise<string>;
  previewSnapshotRestore(
    project: string,
    name: string,
    options?: { dbOnly?: boolean; mediaOnly?: boolean },
  ): Promise<unknown>;
  applySnapshotRestore(
    project: string,
    name: string,
    previewToken: string,
    confirm: boolean,
    options?: { dbOnly?: boolean; mediaOnly?: boolean },
  ): Promise<string>;
  exportSnapshot(
    project: string,
    name: string,
    destPath: string,
  ): Promise<string>;
  cancelSnapshotOp(opId: string): Promise<unknown>;
  pickExportDestination(
    name: string,
  ): Promise<{ path: string; exists: boolean } | null>;
};

type ToastKind = "info" | "success" | "error" | "warning";
/** What main.js keeps calling after the tab moved into React. */
export type DataIslandApi = {
  refresh(): Promise<void>;
};
type Props = {
  bridge: DataBridge;
  onStatus(message: string): void;
  onToast(message: string, kind?: ToastKind): void;
  registerApi(api: DataIslandApi | null): void;
};

/** The project selection, read from the store rather than captured. */
const currentProject = (): string =>
  String(getState().selectedProject ?? "").trim();

const asRecord = (value: unknown): Record<string, any> =>
  value && typeof value === "object" ? (value as Record<string, any>) : {};

const asArray = (value: unknown): Array<Record<string, any>> =>
  Array.isArray(value) ? value.map(asRecord) : [];

const normalizeInfo = (value: unknown): DisplayDBInfo =>
  pickDisplayDBInfo(value) as DisplayDBInfo;

const normalizeSnapshots = (value: unknown): SnapshotRow[] =>
  asArray(value).map((row) => ({
    name: String(row.name ?? ""),
    createdAt: String(row.createdAt ?? "-"),
    sizeHuman: String(row.sizeHuman ?? row.sizeBytes ?? ""),
    hasDb: Boolean(row.hasDb ?? row.hasDB ?? false),
    hasMedia: Boolean(row.hasMedia ?? false),
  }));

const normalizePreview = (value: unknown): RestorePreview | null => {
  const row = asRecord(value);
  if (typeof row.previewToken !== "string" || row.previewToken.trim() === "") {
    return null;
  }
  return {
    previewToken: row.previewToken,
    project: String(row.project ?? ""),
    snapshot: String(row.snapshot ?? ""),
    willReplaceDb: Boolean(row.willReplaceDb ?? false),
    willReplaceMedia: Boolean(row.willReplaceMedia ?? false),
    summary: String(row.summary ?? ""),
  };
};

const normalizePick = (
  value: unknown,
): { path: string; exists: boolean } | null => {
  const row = asRecord(value);
  const path = String(row.path ?? "").trim();
  if (path === "") {
    return null;
  }
  return { path, exists: Boolean(row.exists ?? false) };
};

const modeBadgeClass = (mode: "read" | "write" | "empty"): string =>
  mode === "read"
    ? "text-[10px] uppercase tracking-wider text-primary bg-primary/10 border border-primary/20 px-2.5 py-0.5 rounded-full font-bold leading-none"
    : mode === "write"
      ? "text-[10px] uppercase tracking-wider text-amber-600 dark:text-amber-400 bg-amber-500/10 border border-amber-500/30 px-2.5 py-0.5 rounded-full font-bold leading-none"
      : "text-[10px] uppercase tracking-wider text-text-tertiary bg-surface-secondary border border-border-primary px-2.5 py-0.5 rounded-full font-bold leading-none";

const modeBadgeLabel = (mode: "read" | "write" | "empty"): string =>
  mode === "read"
    ? "Read-only"
    : mode === "write"
      ? "Write opt-in needed"
      : "Empty";

/**
 * The data tab. The island owns the whole subtree main.js mounts at
 * #dataTabIsland: masked DB info cards, the guarded query box with its mode
 * badge, dump/import with streaming progress and cancel, and the snapshot
 * list with the preview-gated restore flow.
 *
 * The island sends ids and dialog-picked paths only: the project id, the
 * snapshot id, op ids, explicit booleans, the user's own query text, and
 * file paths that came back from the native pick dialogs. Destructive steps
 * (write query, dump/export overwrite, snapshot restore) each pass an
 * inline confirm first; the restore confirm stays disabled until the
 * backend preview renders. One transfer runs at a time: while it is active
 * every start button stays disabled behind a notice naming the running op,
 * so progress and cancel always belong to the tracked run. Cancel renders
 * the backend's own message, so the UI never claims a stop the backend did
 * not deliver.
 */
export function DataTab({ bridge, onStatus, onToast, registerApi }: Props) {
  const state = useStore();
  const project = String(state.selectedProject ?? "").trim();
  const [info, setInfo] = useState<DisplayDBInfo | null>(null);
  const [infoNotice, setInfoNotice] = useState(NO_PROJECT_GUIDANCE);
  const [snapshots, setSnapshots] = useState<SnapshotRow[]>([]);
  const [sql, setSql] = useState("");
  const [allowWrite, setAllowWrite] = useState(false);
  const [queryBusy, setQueryBusy] = useState(false);
  const [queryOutput, setQueryOutput] = useState("");
  const [writeConfirm, setWriteConfirm] = useState<WriteConfirm | null>(null);
  const [createName, setCreateName] = useState("");
  const [transfer, setTransfer] = useState<TransferState | null>(null);
  const [restore, setRestore] = useState<RestorePanel | null>(null);
  const [overwrite, setOverwrite] = useState<OverwritePanel | null>(null);

  /** Bumped by every load: only the newest request may write the pane. */
  const loadRequest = useRef(0);

  const mode = classifyQueryMode(sql);
  const actionsEnabled = isDataActionEnabled(project);

  const trackOp = useCallback(
    (opId: string, kind: TransferKind, label: string, detail: string) => {
      const id = String(opId ?? "").trim();
      if (id === "") {
        return;
      }
      setTransfer({
        ...createTransferRun(id, 0),
        kind,
        label,
        detail,
        startedAt: Date.now(),
        percent: null,
        message: `${label} started...`,
      });
      onStatus(`${label} started...`);
    },
    [onStatus],
  );

  // Backstop behind the disabled start buttons: a second start while a
  // transfer runs is refused with the running op named, never silently
  // superseded. Returns true when the caller must stop.
  const refuseIfBusy = useCallback(
    (action: string): boolean => {
      if (!isStartBlocked(transfer)) {
        return false;
      }
      const message = `Cannot start ${action}: ${describeRunningOp(transfer)}`;
      onStatus(message);
      onToast(message, "warning");
      return true;
    },
    [onStatus, onToast, transfer],
  );

  const refresh = useCallback(async () => {
    const request = loadRequest.current + 1;
    loadRequest.current = request;
    const name = currentProject();
    if (!isDataActionEnabled(name)) {
      setInfo(null);
      setSnapshots([]);
      setInfoNotice(NO_PROJECT_GUIDANCE);
      return;
    }
    setInfoNotice("Loading database info...");
    try {
      const [rawInfo, rawSnapshots] = await Promise.all([
        bridge.getDBInfo(name),
        bridge.listSnapshots(name),
      ]);
      if (loadRequest.current !== request) {
        return;
      }
      setInfo(normalizeInfo(rawInfo));
      setSnapshots(normalizeSnapshots(rawSnapshots));
      setInfoNotice("");
    } catch (err) {
      if (loadRequest.current !== request) {
        return;
      }
      setInfo(null);
      setSnapshots([]);
      setInfoNotice(`Failed to load data: ${err}`);
    }
  }, [bridge]);

  useEffect(() => {
    registerApi({ refresh });
    // Handing the API back on unmount stops main.js from reaching a dead
    // island (the same contract the logs/remotes/sync islands follow).
    return () => registerApi(null);
  }, [registerApi, refresh]);

  // The first value never waits for main.js's first refresh, which can run
  // before React has mounted at all.
  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!hasEventRuntime()) return undefined;
    const offs = [
      onEvent("db:progress", (data: unknown) => {
        const payload = asRecord(data);
        setTransfer((prev) => {
          if (!prev || prev.finished) {
            return prev;
          }
          if (String(payload.opId ?? "") !== prev.opId) {
            return prev;
          }
          const gen = Number(payload.generation ?? 0);
          if (
            prev.gen !== 0 &&
            Number.isFinite(gen) &&
            gen !== 0 &&
            gen !== prev.gen
          ) {
            return prev;
          }
          const percent = Number(payload.percent ?? null);
          return {
            ...prev,
            gen:
              prev.gen === 0 && Number.isFinite(gen) ? gen : prev.gen,
            percent: Number.isFinite(percent) ? percent : prev.percent,
            message: String(payload.message ?? prev.message),
          };
        });
      }),
      onEvent("db:completed", (data: unknown) => {
        const payload = asRecord(data);
        setTransfer((prev) => {
          if (!prev) {
            return prev;
          }
          const next = applyTransferTerminalEvent(
            { ...prev, gen: prev.gen, finished: false, outcome: "" },
            {
              opId: payload.opId,
              generation: payload.generation,
              event: "db:completed",
            },
          );
          if (next.finished && !prev.finished) {
            const message = String(
              payload.message ?? "Transfer completed.",
            );
            onStatus(message);
            onToast(message, "success");
            return { ...prev, ...next, message };
          }
          return prev;
        });
      }),
      onEvent("db:failed", (data: unknown) => {
        const payload = asRecord(data);
        setTransfer((prev) => {
          if (!prev) {
            return prev;
          }
          const next = applyTransferTerminalEvent(
            { ...prev, gen: prev.gen, finished: false, outcome: "" },
            {
              opId: payload.opId,
              generation: payload.generation,
              event: "db:failed",
            },
          );
          if (next.finished && !prev.finished) {
            const message = String(payload.message ?? "Transfer failed.");
            onStatus(message);
            onToast(message, "error");
            return { ...prev, ...next, message };
          }
          return prev;
        });
      }),
      onEvent("snapshot:completed", (data: unknown) => {
        const payload = asRecord(data);
        setTransfer((prev) => {
          if (!prev) {
            return prev;
          }
          const next = applyTransferTerminalEvent(
            { ...prev, gen: prev.gen, finished: false, outcome: "" },
            { opId: payload.opId, event: "snapshot:completed" },
          );
          if (next.finished && !prev.finished) {
            const message = String(
              payload.message ?? "Snapshot operation completed.",
            );
            onStatus(message);
            onToast(message, "success");
            void refresh();
            return { ...prev, ...next, message };
          }
          return prev;
        });
      }),
      onEvent("snapshot:failed", (data: unknown) => {
        const payload = asRecord(data);
        setTransfer((prev) => {
          if (!prev) {
            return prev;
          }
          const next = applyTransferTerminalEvent(
            { ...prev, gen: prev.gen, finished: false, outcome: "" },
            { opId: payload.opId, event: "snapshot:failed" },
          );
          if (next.finished && !prev.finished) {
            // The message is the backend's own wording: a user stop reads
            // as tracking stopped (the engine call may run on), never as a
            // delivered stop.
            const message = String(
              payload.message ?? "Snapshot operation failed.",
            );
            onStatus(message);
            onToast(message, "error");
            return { ...prev, ...next, message };
          }
          return prev;
        });
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [onStatus, onToast, refresh]);

  const runQuery = useCallback(
    async (statement: string, write: boolean, token: string) => {
      const name = currentProject();
      setQueryBusy(true);
      try {
        const raw = await bridge.runDBQuery(name, statement, write, token);
        const output = String(asRecord(raw).output ?? raw ?? "");
        setQueryOutput(output || "(no output)");
        onStatus(write ? "Write query completed." : "Query completed.");
      } catch (err) {
        setQueryOutput("");
        const message = `Query failed: ${err}`;
        onStatus(message);
        onToast(message, "error");
      } finally {
        setQueryBusy(false);
      }
    },
    [bridge, onStatus, onToast],
  );

  const submitQuery = useCallback(async () => {
    const name = currentProject();
    if (!isDataActionEnabled(name)) {
      onStatus(NO_PROJECT_GUIDANCE);
      return;
    }
    const statement = sql.trim();
    if (statement === "" || classifyQueryMode(statement) === "empty") {
      onStatus("Enter a query first.");
      return;
    }
    if (classifyQueryMode(statement) !== "read" && !allowWrite) {
      const message =
        "Write statements need the explicit write opt-in for this project.";
      onStatus(message);
      onToast(message, "warning");
      return;
    }
    if (classifyQueryMode(statement) !== "read") {
      try {
        const token = await bridge.getDBWriteConfirmToken(name);
        setWriteConfirm({ sql: statement, token: String(token ?? "") });
      } catch (err) {
        const message = `Write opt-in failed: ${err}`;
        onStatus(message);
        onToast(message, "error");
      }
      return;
    }
    await runQuery(statement, false, "");
  }, [allowWrite, bridge, onStatus, onToast, runQuery, sql]);

  const confirmWriteQuery = useCallback(async () => {
    if (!writeConfirm) {
      return;
    }
    setWriteConfirm(null);
    await runQuery(writeConfirm.sql, true, writeConfirm.token);
  }, [runQuery, writeConfirm]);

  const startDump = useCallback(
    async (destPath: string) => {
      if (refuseIfBusy("dump")) {
        return;
      }
      const name = currentProject();
      try {
        const opId = await bridge.dumpDB(name, destPath);
        trackOp(opId, "dump", "Dump", destPath);
      } catch (err) {
        const message = `Dump failed to start: ${err}`;
        onStatus(message);
        onToast(message, "error");
      }
    },
    [bridge, onStatus, onToast, refuseIfBusy, trackOp],
  );

  const dump = useCallback(async () => {
    const name = currentProject();
    if (!isDataActionEnabled(name)) {
      onStatus(NO_PROJECT_GUIDANCE);
      return;
    }
    if (refuseIfBusy("dump")) {
      return;
    }
    try {
      const pick = normalizePick(await bridge.pickDumpDestination(name));
      if (!pick) {
        onStatus("Dump destination picker cancelled.");
        return;
      }
      if (pick.exists) {
        setOverwrite({ kind: "dump", destPath: pick.path, snapshotName: "" });
        return;
      }
      await startDump(pick.path);
    } catch (err) {
      const message = `Dump destination picker failed: ${err}`;
      onStatus(message);
      onToast(message, "error");
    }
  }, [bridge, onStatus, onToast, refuseIfBusy, startDump]);

  const importDump = useCallback(async () => {
    const name = currentProject();
    if (!isDataActionEnabled(name)) {
      onStatus(NO_PROJECT_GUIDANCE);
      return;
    }
    if (refuseIfBusy("import")) {
      return;
    }
    try {
      const src = String((await bridge.pickImportSource()) ?? "").trim();
      if (src === "") {
        onStatus("Import source picker cancelled.");
        return;
      }
      const opId = await bridge.importDB(name, src);
      trackOp(opId, "import", "Import", src);
    } catch (err) {
      const message = `Import failed to start: ${err}`;
      onStatus(message);
      onToast(message, "error");
    }
  }, [bridge, onStatus, onToast, refuseIfBusy, trackOp]);

  const cancelTransfer = useCallback(async () => {
    if (!transfer || transfer.finished) {
      return;
    }
    const { opId, kind } = transfer;
    onStatus(`Stopping ${transfer.label.toLowerCase()}...`);
    try {
      if (kind.startsWith("snapshot-")) {
        await bridge.cancelSnapshotOp(opId);
      } else {
        await bridge.cancelDBOp(opId);
      }
    } catch (err) {
      const message = `Stop request failed: ${err}`;
      onStatus(message);
      onToast(message, "error");
    }
  }, [bridge, onStatus, onToast, transfer]);

  const create = useCallback(async () => {
    const name = currentProject();
    if (!isDataActionEnabled(name)) {
      onStatus(NO_PROJECT_GUIDANCE);
      return;
    }
    if (refuseIfBusy("snapshot create")) {
      return;
    }
    try {
      const snapshotName = createName.trim();
      const opId = await bridge.createSnapshot(name, snapshotName);
      trackOp(
        opId,
        "snapshot-create",
        "Snapshot create",
        snapshotName === "" ? "timestamped snapshot" : snapshotName,
      );
    } catch (err) {
      const message = `Snapshot create failed to start: ${err}`;
      onStatus(message);
      onToast(message, "error");
    }
  }, [bridge, createName, onStatus, onToast, refuseIfBusy, trackOp]);

  const openRestore = useCallback(
    async (snapshotName: string, scope: RestoreScope) => {
      const name = currentProject();
      if (!isDataActionEnabled(name)) {
        onStatus(NO_PROJECT_GUIDANCE);
        return;
      }
      setRestore({ name: snapshotName, scope, preview: null, loading: true });
      try {
        const flags = snapshotScopeFlags(scope);
        const preview = normalizePreview(
          await bridge.previewSnapshotRestore(name, snapshotName, flags),
        );
        setRestore({ name: snapshotName, scope, preview, loading: false });
        if (!preview) {
          onStatus("Restore preview came back empty; try again.");
        }
      } catch (err) {
        setRestore({ name: snapshotName, scope, preview: null, loading: false });
        const message = `Restore preview failed: ${err}`;
        onStatus(message);
        onToast(message, "error");
      }
    },
    [bridge, onStatus, onToast],
  );

  const changeRestoreScope = useCallback(
    async (scope: RestoreScope) => {
      if (!restore) {
        return;
      }
      await openRestore(restore.name, scope);
    },
    [openRestore, restore],
  );

  const confirmRestore = useCallback(async () => {
    if (!restore?.preview || !isSnapshotRestoreConfirmEnabled(restore.preview)) {
      return;
    }
    if (refuseIfBusy("snapshot restore")) {
      return;
    }
    const name = currentProject();
    const flags = snapshotScopeFlags(restore.scope);
    const { name: snapshotName, preview } = restore;
    setRestore(null);
    try {
      const opId = await bridge.applySnapshotRestore(
        name,
        snapshotName,
        preview.previewToken,
        true,
        flags,
      );
      trackOp(opId, "snapshot-restore", "Snapshot restore", snapshotName);
    } catch (err) {
      const message = `Snapshot restore failed to start: ${err}`;
      onStatus(message);
      onToast(message, "error");
    }
  }, [bridge, onStatus, onToast, refuseIfBusy, restore, trackOp]);

  const startExport = useCallback(
    async (snapshotName: string, destPath: string) => {
      if (refuseIfBusy("snapshot export")) {
        return;
      }
      const name = currentProject();
      try {
        const opId = await bridge.exportSnapshot(name, snapshotName, destPath);
        trackOp(opId, "snapshot-export", "Snapshot export", snapshotName);
      } catch (err) {
        const message = `Snapshot export failed to start: ${err}`;
        onStatus(message);
        onToast(message, "error");
      }
    },
    [bridge, onStatus, onToast, refuseIfBusy, trackOp],
  );

  const exportSnapshot = useCallback(
    async (snapshotName: string) => {
      const name = currentProject();
      if (!isDataActionEnabled(name)) {
        onStatus(NO_PROJECT_GUIDANCE);
        return;
      }
      if (refuseIfBusy("snapshot export")) {
        return;
      }
      try {
        const pick = normalizePick(
          await bridge.pickExportDestination(snapshotName),
        );
        if (!pick) {
          onStatus("Export destination picker cancelled.");
          return;
        }
        if (pick.exists) {
          setOverwrite({
            kind: "export",
            destPath: pick.path,
            snapshotName,
          });
          return;
        }
        await startExport(snapshotName, pick.path);
      } catch (err) {
        const message = `Export destination picker failed: ${err}`;
        onStatus(message);
        onToast(message, "error");
      }
    },
    [bridge, onStatus, onToast, refuseIfBusy, startExport],
  );

  const confirmOverwrite = useCallback(async () => {
    if (!overwrite) {
      return;
    }
    const pending = overwrite;
    setOverwrite(null);
    if (pending.kind === "dump") {
      await startDump(pending.destPath);
    } else {
      await startExport(pending.snapshotName, pending.destPath);
    }
  }, [overwrite, startDump, startExport]);

  const transferRunning = transfer !== null && !transfer.finished;

  return (
    <div
      data-testid="data-tab"
      className="px-6 lg:px-10 py-6 max-w-[1248px] w-full mx-auto flex-1 flex flex-col gap-6 overflow-y-auto h-full"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="flex items-center justify-center size-8 bg-primary/10 rounded-lg text-primary">
            <span className="material-symbols-outlined text-xl" aria-hidden="true">
              database
            </span>
          </div>
          <h3 className="text-sm font-bold text-slate-800 dark:text-white leading-none">
            Data
          </h3>
          <span className="text-[10px] uppercase tracking-wider text-primary bg-primary/10 border border-primary/20 px-2.5 py-0.5 rounded-full font-bold leading-none">
            Read-only default
          </span>
        </div>
        <button
          type="button"
          data-testid="data-refresh"
          className="h-8 px-3 rounded-md text-xs font-semibold text-text-tertiary dark:text-slate-400 hover:text-text-primary dark:hover:text-white hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all"
          onClick={() => void refresh()}
        >
          Refresh
        </button>
      </div>

      {!actionsEnabled && (
        <p className="text-xs text-slate-500 italic">{NO_PROJECT_GUIDANCE}</p>
      )}

      {transferRunning && transfer && (
        <p
          data-testid="transfer-active-notice"
          role="status"
          className="rounded-lg border border-primary/30 bg-primary/5 px-3 py-2 text-xs text-slate-700 dark:text-slate-200"
        >
          {describeRunningOp(transfer)} Start buttons stay disabled until it
          finishes.
        </p>
      )}

      <section
        data-testid="db-info"
        aria-label="Database connection"
        className="rounded-xl border border-slate-200 dark:border-border-primary bg-white dark:bg-background-primary p-4 shadow-lg"
      >
        <h4 className="text-xs font-bold uppercase tracking-wide text-slate-500 dark:text-slate-400 mb-3">
          Connection
        </h4>
        {info ? (
          <dl className="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
            <div>
              <dt className="text-text-tertiary">Engine</dt>
              <dd className="font-semibold text-slate-800 dark:text-white">
                {info.engine || "-"}
              </dd>
            </div>
            <div>
              <dt className="text-text-tertiary">Host</dt>
              <dd className="font-semibold text-slate-800 dark:text-white">
                {info.host || "-"}
              </dd>
            </div>
            <div>
              <dt className="text-text-tertiary">Port</dt>
              <dd className="font-semibold text-slate-800 dark:text-white">
                {info.port || "-"}
              </dd>
            </div>
            <div>
              <dt className="text-text-tertiary">Database</dt>
              <dd className="font-semibold text-slate-800 dark:text-white">
                {info.database || "-"}
              </dd>
            </div>
            <div>
              <dt className="text-text-tertiary">User</dt>
              <dd className="font-semibold text-slate-800 dark:text-white">
                {info.user || "-"}
              </dd>
            </div>
            <div>
              <dt className="text-text-tertiary">Credential</dt>
              <dd className="font-semibold text-slate-800 dark:text-white">
                {info.hasPassword ? "Set" : "Not set"}
              </dd>
            </div>
            <div className="col-span-2">
              <dt className="text-text-tertiary">Display URL</dt>
              <dd
                data-testid="db-info-display-url"
                className="font-mono break-all text-slate-800 dark:text-white"
              >
                {info.displayUrl || "-"}
              </dd>
            </div>
          </dl>
        ) : (
          <p className="text-xs text-slate-500 italic">{infoNotice}</p>
        )}
      </section>

      <section
        aria-label="Ad-hoc query"
        className="rounded-xl border border-slate-200 dark:border-border-primary bg-white dark:bg-background-primary p-4 shadow-lg flex flex-col gap-3"
      >
        <div className="flex items-center justify-between gap-3">
          <h4 className="text-xs font-bold uppercase tracking-wide text-slate-500 dark:text-slate-400">
            Query
          </h4>
          <span data-testid="db-query-mode" className={modeBadgeClass(mode)}>
            {modeBadgeLabel(mode)}
          </span>
        </div>
        <textarea
          data-testid="db-query-input"
          className="w-full min-h-24 rounded-md border border-border-primary bg-surface-secondary p-3 font-mono text-xs text-text-primary dark:text-white focus:border-primary/50 focus:outline-hidden"
          placeholder="SELECT * FROM users LIMIT 10"
          value={sql}
          onChange={(event) => setSql(event.target.value)}
          disabled={!actionsEnabled}
        />
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2 text-xs text-text-secondary dark:text-slate-300 cursor-pointer">
            <input
              type="checkbox"
              data-testid="db-write-optin"
              checked={allowWrite}
              onChange={(event) => setAllowWrite(event.target.checked)}
              disabled={!actionsEnabled}
            />
            Allow write for this project (confirm each time)
          </label>
          <button
            type="button"
            data-testid="db-run-query"
            className="h-8 px-4 rounded-md text-xs font-bold bg-primary text-slate-900 hover:bg-primary/90 disabled:opacity-50 transition-all"
            disabled={!actionsEnabled || queryBusy}
            onClick={() => void submitQuery()}
          >
            {queryBusy ? "Running..." : "Run query"}
          </button>
        </div>
        {writeConfirm && (
          <div className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 flex flex-col gap-2">
            <p className="text-xs text-slate-700 dark:text-slate-200">
              This write runs against <strong>{project}</strong>. Review it,
              then confirm:
            </p>
            <pre className="font-mono text-xs whitespace-pre-wrap text-slate-700 dark:text-slate-200 max-h-32 overflow-y-auto">
              {writeConfirm.sql}
            </pre>
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                className="px-3 py-1.5 rounded-md text-xs text-text-secondary dark:text-slate-300 hover:bg-primary/10 transition-colors"
                onClick={() => setWriteConfirm(null)}
              >
                Back
              </button>
              <button
                type="button"
                data-testid="confirm-write-query"
                className="px-4 py-1.5 rounded-md text-xs font-bold bg-primary text-slate-900 hover:bg-primary/90 transition-all"
                onClick={() => void confirmWriteQuery()}
              >
                Confirm write
              </button>
            </div>
          </div>
        )}
        {queryOutput !== "" && (
          <pre
            data-testid="db-query-output"
            className="m-0 rounded-md bg-slate-900 text-slate-300 p-3 font-mono text-xs whitespace-pre-wrap max-h-64 overflow-y-auto"
          >
            {queryOutput}
          </pre>
        )}
      </section>

      <section
        aria-label="Dump and import"
        className="rounded-xl border border-slate-200 dark:border-border-primary bg-white dark:bg-background-primary p-4 shadow-lg flex flex-col gap-3"
      >
        <h4 className="text-xs font-bold uppercase tracking-wide text-slate-500 dark:text-slate-400">
          Dump and import
        </h4>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            data-testid="db-dump-button"
            className="h-8 px-4 rounded-md text-xs font-semibold border border-border-primary hover:bg-slate-100 dark:hover:bg-surface-secondary disabled:opacity-50 transition-all"
            disabled={!actionsEnabled || transferRunning}
            onClick={() => void dump()}
          >
            Dump database...
          </button>
          <button
            type="button"
            data-testid="db-import-button"
            className="h-8 px-4 rounded-md text-xs font-semibold border border-border-primary hover:bg-slate-100 dark:hover:bg-surface-secondary disabled:opacity-50 transition-all"
            disabled={!actionsEnabled || transferRunning}
            onClick={() => void importDump()}
          >
            Import dump...
          </button>
        </div>
        {overwrite && (
          <div className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 flex flex-col gap-2">
            <p className="text-xs text-slate-700 dark:text-slate-200">
              <span className="font-mono break-all">{overwrite.destPath}</span>{" "}
              already exists. Overwrite it with the new{" "}
              {overwrite.kind === "dump" ? "dump" : "export"}?
            </p>
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                className="px-3 py-1.5 rounded-md text-xs text-text-secondary dark:text-slate-300 hover:bg-primary/10 transition-colors"
                onClick={() => setOverwrite(null)}
              >
                Back
              </button>
              <button
                type="button"
                data-testid="confirm-overwrite"
                className="px-4 py-1.5 rounded-md text-xs font-bold bg-primary text-slate-900 hover:bg-primary/90 transition-all"
                onClick={() => void confirmOverwrite()}
              >
                Overwrite
              </button>
            </div>
          </div>
        )}
        {transferRunning &&
          (transfer.kind === "dump" || transfer.kind === "import") && (
            <div className="flex flex-col gap-2">
              <div
                data-testid="db-transfer-progress"
                role="progressbar"
                aria-valuenow={transfer.percent ?? undefined}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label={`${transfer.label} progress`}
                className="h-2 rounded-full bg-surface-secondary overflow-hidden"
              >
                <div
                  className="h-full bg-primary transition-all"
                  style={{ width: `${transfer.percent ?? 0}%` }}
                />
              </div>
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p
                    data-testid="db-transfer-owner"
                    className="text-xs font-semibold text-slate-700 dark:text-slate-200 truncate"
                  >
                    {transfer.label}: {transfer.detail}
                  </p>
                  <p className="text-xs text-slate-500 truncate">
                    {transfer.message}
                  </p>
                </div>
                <button
                  type="button"
                  data-testid="db-transfer-cancel"
                  className="h-7 px-3 rounded-md text-xs font-semibold border border-border-primary hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all shrink-0"
                  onClick={() => void cancelTransfer()}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
      </section>

      <section
        aria-label="Snapshots"
        className="rounded-xl border border-slate-200 dark:border-border-primary bg-white dark:bg-background-primary p-4 shadow-lg flex flex-col gap-3"
      >
        <h4 className="text-xs font-bold uppercase tracking-wide text-slate-500 dark:text-slate-400">
          Snapshots
        </h4>
        <div className="flex flex-wrap gap-2">
          <input
            data-testid="snapshot-create-name"
            className="flex-1 min-w-44 rounded-md border border-border-primary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary dark:text-white focus:border-primary/50 focus:outline-hidden placeholder-text-tertiary"
            placeholder="Snapshot name (empty for timestamped)"
            value={createName}
            onChange={(event) => setCreateName(event.target.value)}
            disabled={!actionsEnabled}
          />
          <button
            type="button"
            data-testid="snapshot-create-button"
            className="h-8 px-4 rounded-md text-xs font-semibold border border-border-primary hover:bg-slate-100 dark:hover:bg-surface-secondary disabled:opacity-50 transition-all"
            disabled={!actionsEnabled || transferRunning}
            onClick={() => void create()}
          >
            Create snapshot
          </button>
        </div>
        <ul data-testid="snapshot-list" className="flex flex-col gap-2">
          {snapshots.map((snapshot) => (
            <li
              key={snapshot.name}
              className="flex flex-wrap items-center gap-2 rounded-lg border border-border-primary/60 p-2.5 text-xs"
            >
              <span className="font-bold text-slate-800 dark:text-white">
                {snapshot.name}
              </span>
              <span className="text-text-tertiary">{snapshot.createdAt}</span>
              <span className="text-text-tertiary">{snapshot.sizeHuman}</span>
              {snapshot.hasDb && (
                <span className="text-[10px] uppercase tracking-wide text-primary bg-primary/10 border border-primary/20 px-2 py-0.5 rounded-full font-bold">
                  DB
                </span>
              )}
              {snapshot.hasMedia && (
                <span className="text-[10px] uppercase tracking-wide text-primary bg-primary/10 border border-primary/20 px-2 py-0.5 rounded-full font-bold">
                  Media
                </span>
              )}
              <span className="flex-1" />
              <button
                type="button"
                data-testid="snapshot-restore-button"
                className="h-7 px-3 rounded-md text-xs font-semibold border border-border-primary hover:bg-slate-100 dark:hover:bg-surface-secondary disabled:opacity-50 transition-all"
                disabled={!actionsEnabled || transferRunning}
                onClick={() => void openRestore(snapshot.name, "full")}
              >
                Restore...
              </button>
              <button
                type="button"
                data-testid="snapshot-export-button"
                className="h-7 px-3 rounded-md text-xs font-semibold border border-border-primary hover:bg-slate-100 dark:hover:bg-surface-secondary disabled:opacity-50 transition-all"
                disabled={!actionsEnabled || transferRunning}
                onClick={() => void exportSnapshot(snapshot.name)}
              >
                Export...
              </button>
            </li>
          ))}
          {snapshots.length === 0 && actionsEnabled && (
            <li className="text-xs text-slate-500 italic">No snapshots yet.</li>
          )}
        </ul>
        {restore && (
          <div
            data-testid="snapshot-restore-preview"
            className="rounded-lg border border-border-primary p-3 flex flex-col gap-2"
          >
            <p className="text-xs font-bold text-slate-800 dark:text-white">
              Restore {restore.name}
            </p>
            <div className="flex gap-2" role="radiogroup" aria-label="Restore scope">
              {(["full", "db", "media"] as RestoreScope[]).map((scope) => (
                <label
                  key={scope}
                  className="flex items-center gap-1.5 text-xs text-text-secondary dark:text-slate-300 cursor-pointer"
                >
                  <input
                    type="radio"
                    name="restore-scope"
                    value={scope}
                    checked={restore.scope === scope}
                    onChange={() => void changeRestoreScope(scope)}
                  />
                  {scope === "full" ? "Full" : scope === "db" ? "DB only" : "Media only"}
                </label>
              ))}
            </div>
            {restore.loading ? (
              <p className="text-xs text-slate-500 italic">
                Loading preview...
              </p>
            ) : restore.preview ? (
              <p className="text-xs text-slate-700 dark:text-slate-200">
                {restore.preview.summary}{" "}
                <span className="text-text-tertiary">
                  (DB: {restore.preview.willReplaceDb ? "replaced" : "untouched"};
                  media:{" "}
                  {restore.preview.willReplaceMedia ? "replaced" : "untouched"})
                </span>
              </p>
            ) : (
              <p className="text-xs text-slate-500 italic">
                Preview unavailable.
              </p>
            )}
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                className="px-3 py-1.5 rounded-md text-xs text-text-secondary dark:text-slate-300 hover:bg-primary/10 transition-colors"
                onClick={() => setRestore(null)}
              >
                Back
              </button>
              <button
                type="button"
                data-testid="confirm-snapshot-restore"
                className="px-4 py-1.5 rounded-md text-xs font-bold bg-primary text-slate-900 hover:bg-primary/90 disabled:opacity-50 disabled:pointer-events-none transition-all"
                disabled={
                  !restore.preview ||
                  !isSnapshotRestoreConfirmEnabled(restore.preview) || transferRunning
                }
                onClick={() => void confirmRestore()}
              >
                Confirm restore
              </button>
            </div>
          </div>
        )}
        {transferRunning && transfer.kind.startsWith("snapshot-") && (
          <div className="flex items-center justify-between gap-3 rounded-lg border border-border-primary/60 p-2.5">
            <div className="min-w-0">
              <p
                data-testid="snapshot-transfer-owner"
                className="text-xs font-semibold text-slate-700 dark:text-slate-200 truncate"
              >
                {transfer.label}: {transfer.detail}
              </p>
              <p className="text-xs text-slate-500 truncate">
                {transfer.message}
              </p>
            </div>
            <button
              type="button"
              data-testid="snapshot-cancel"
              className="h-7 px-3 rounded-md text-xs font-semibold border border-border-primary hover:bg-slate-100 dark:hover:bg-surface-secondary transition-all shrink-0"
              onClick={() => void cancelTransfer()}
            >
              Cancel
            </button>
          </div>
        )}
      </section>
    </div>
  );
}
