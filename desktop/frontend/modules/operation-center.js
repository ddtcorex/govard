// @ts-check

/**
 * Operation Center helpers: the pure shapes behind the island. The running
 * list reads ListOps, history reads ListHistory; both bindings emit Go
 * field names (ID, Project, ...) while the preview fixtures use camelCase,
 * so every normalizer takes either. Retry is failed-only and interrupted
 * entries carry a re-run hint (a fresh run, never a replay).
 */

/** The backend event the operation watcher emits per terminal op. */
export const OPERATIONS_NOTIFICATION_EVENT = "operations:notification";

/** The structured sync progress event the remotes flow emits mid-op. */
export const SYNC_PROGRESS_EVENT = "sync:progress";

/** A restart-interrupted entry never replays: its hint points at a fresh run. */
export const INTERRUPTED_RERUN_HINT =
  "Interrupted by an app restart. Start a fresh run; the stored args are kept for reference.";

/**
 * The toast action ids the backend notification catalog emits. They live
 * once here so the notification handler switches on constants, never
 * inline strings.
 */
export const NOTIFICATION_ACTION_RETRY = "retry";
export const NOTIFICATION_ACTION_OPEN_LOGS = "open-logs";

/**
 * The backend event payload normalized to camelCase, whatever casing the
 * bindings produced. Actions default to [] (success notifications carry
 * none); retryId defaults to "" (no failed record matched the event).
 * @param {any} payload
 */
export const normalizeNotification = (payload = {}) => {
  const source = payload ?? {};
  const rawActions = source.actions ?? source.Actions;
  const actions = Array.isArray(rawActions) ? rawActions : [];
  return {
    id: asString(source.id ?? source.ID),
    title: asString(source.title ?? source.Title),
    body: asString(source.body ?? source.Body),
    level: asString(source.level ?? source.Level),
    operation: asString(source.operation ?? source.Operation),
    status: asString(source.status ?? source.Status).toLowerCase(),
    project: asString(source.project ?? source.Project),
    actions: actions
      .map((action) => ({
        id: asString(action?.id ?? action?.ID),
        label: asString(action?.label ?? action?.Label),
      }))
      .filter((action) => action.id !== "" && action.label !== ""),
    retryId: asString(source.retryId ?? source.RetryID ?? source.retryID),
  };
};

/**
 * @param {any} value
 */
const asString = (value) => String(value ?? "").trim();

/**
 * @param {any} value
 */
const asStringList = (value) =>
  Array.isArray(value)
    ? value.map((item) => String(item ?? "")).filter((item) => item !== "")
    : [];

/**
 * @param {any} op
 */
const normalizeOp = (op = {}) => ({
  id: asString(op.id ?? op.ID),
  project: asString(op.project ?? op.Project),
  kind: asString(op.kind ?? op.Kind),
  args: asStringList(op.args ?? op.Args),
  startedAt: asString(op.startedAt ?? op.StartedAt),
});

/**
 * @param {any} record
 */
const normalizeRecord = (record = {}) => ({
  id: asString(record.id ?? record.ID),
  project: asString(record.project ?? record.Project),
  kind: asString(record.kind ?? record.Kind),
  args: asStringList(record.args ?? record.Args),
  status: asString(record.status ?? record.Status).toLowerCase(),
  message: asString(record.message ?? record.Message),
  startedAt: asString(record.startedAt ?? record.StartedAt),
  endedAt: asString(record.endedAt ?? record.EndedAt),
  durationMs: Number(record.durationMs ?? record.DurationMS ?? 0) || 0,
});

/**
 * The running ops, oldest first as the backend orders them. Blank ids never
 * render: a record without an id is corrupt history, not an operation.
 * @param {any} payload
 */
export const normalizeOpList = (payload = {}) => {
  const raw = Array.isArray(payload) ? payload : (payload.ops ?? payload.Ops ?? []);
  const list = Array.isArray(raw) ? raw : [];
  return { ops: list.map(normalizeOp).filter((op) => op.id !== "") };
};

/**
 * Terminal records, newest first as the backend orders them.
 * @param {any} payload
 */
export const normalizeHistoryList = (payload = {}) => {
  const raw = Array.isArray(payload)
    ? payload
    : (payload.records ?? payload.Records ?? []);
  const list = Array.isArray(raw) ? raw : [];
  return { records: list.map(normalizeRecord).filter((record) => record.id !== "") };
};

/**
 * Only failed ops show Retry: succeeded and cancelled entries are done, and
 * an interrupted entry needs a fresh run (see rerunHintFor), never a replay.
 * @param {any} record
 */
export const isRetryable = (record = {}) =>
  asString(record.status ?? record.Status).toLowerCase() === "failed";

/**
 * @param {any} record
 */
export const rerunHintFor = (record = {}) =>
  asString(record.status ?? record.Status).toLowerCase() === "interrupted"
    ? INTERRUPTED_RERUN_HINT
    : "";

/**
 * The toast tone trusts the backend level: the watcher already classifies
 * each event (success/error), so body-substring sniffing here would only
 * override it, e.g. escalating an info note that merely mentions "error".
 * A missing level defaults to success.
 * @param {any} note a normalized notification
 */
export const notificationLevel = (note = {}) => {
  const level = asString(note.level ?? note.Level);
  return level === "" ? "success" : level;
};

/**
 * Resolve a toast action click to its dispatch target. Retry dispatches only
 * with the exact record id bound at notify time; without one there is
 * nothing to replay, so the target is unavailable and the caller must fail
 * closed (record unavailable) without dispatching a retry.
 * @param {any} actionId
 * @param {any} note a normalized notification
 */
export const notificationActionTarget = (actionId, note = {}) => {
  if (actionId === NOTIFICATION_ACTION_RETRY) {
    const id = asString(note.retryId);
    if (id === "") {
      return { kind: "unavailable", reason: "Record unavailable for retry." };
    }
    return { kind: "retry", id };
  }
  if (actionId === NOTIFICATION_ACTION_OPEN_LOGS) {
    return { kind: "open-logs", project: asString(note.project) };
  }
  return { kind: "unknown", reason: `Unknown notification action ${asString(actionId) || "(blank)"}.` };
};
