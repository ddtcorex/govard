// @ts-check
/**
 * The data tab's data shape and its pure helpers.
 *
 * The tab manages the project database (masked info, guarded ad-hoc query,
 * dump/import with streaming progress) and snapshots (list/create/export,
 * preview-gated restore). Everything that touches the document lives in the
 * DataTab island; what stays here is the part with no element in it:
 *
 * - the query-mode hint for the mode badge. Display only: the backend
 *   classifier stays the gate, so a hint that errs calls it "write" (the
 *   opt-in path), never "read";
 * - the DB info display allowlist. The backend info struct has no password
 *   field by construction; the allowlist drops anything unexpected anyway,
 *   so a secret can never reach the DOM even if the payload shape drifts;
 * - the restore-confirm gate (confirm stays disabled until the preview
 *   renders) and the snapshot scope to db/media flag mapping;
 * - the (op, generation) attribution helpers. Transfer op ids are unique per
 *   start, so a terminal event applies only to its own op and generation,
 *   and a finished transfer never flips back to running.
 *
 * There is deliberately no query history here: the brief needs none, and no
 * history means no secret can linger in one (binding decision 5).
 */

/** Copy shown when no project is selected: every action stays disabled. */
export const NO_PROJECT_GUIDANCE = "Select a project to manage its data.";

/**
 * @param {unknown} project
 * @returns {boolean}
 */
export function isDataActionEnabled(project) {
  return String(project ?? "").trim() !== "";
}

/**
 * First keywords the backend read-only gate allows as openers. The island
 * badge mirrors this list for display; anything else (or anything uncertain)
 * reads as "write" and takes the opt-in plus confirm path.
 */
const READ_ONLY_OPENERS = new Set([
  "SELECT",
  "WITH",
  "SHOW",
  "DESCRIBE",
  "DESC",
  "EXPLAIN",
]);

/**
 * Display hint for the query mode badge: "read" (runs under the read-only
 * default), "write" (needs the explicit opt-in plus confirm) or "empty".
 * A semicolon outside quotes conservatively reads as "write": multi-
 * statement input is refused by default, and the backend decides anyway.
 * @param {unknown} sql
 * @returns {"read" | "write" | "empty"}
 */
export function classifyQueryMode(sql) {
  const text = String(sql ?? "");
  const keyword = firstQueryKeyword(text);
  if (keyword === "") {
    return "empty";
  }
  if (!READ_ONLY_OPENERS.has(keyword)) {
    return "write";
  }
  if (hasSecondStatement(text)) {
    return "write";
  }
  return "read";
}

/**
 * Reads the first letter-run after leading whitespace and SQL comments,
 * uppercased. Anything without a plain leading keyword yields "".
 * @param {string} text
 * @returns {string}
 */
function firstQueryKeyword(text) {
  let i = 0;
  while (i < text.length) {
    while (i < text.length && /\s/.test(text[i])) {
      i++;
    }
    if (text.startsWith("--", i)) {
      const end = text.indexOf("\n", i);
      i = end < 0 ? text.length : end + 1;
      continue;
    }
    if (text[i] === "#") {
      const end = text.indexOf("\n", i);
      i = end < 0 ? text.length : end + 1;
      continue;
    }
    if (text.startsWith("/*", i)) {
      const end = text.indexOf("*/", i + 2);
      i = end < 0 ? text.length : end + 2;
      continue;
    }
    break;
  }
  const rest = text.slice(i);
  const match = /^[A-Za-z]+/.exec(rest);
  return match ? match[0].toUpperCase() : "";
}

/**
 * Reports a statement separator outside string literals and quoted
 * identifiers. Runs of skipped text cannot fuse tokens; a separator inside
 * a comment conservatively counts too (display hint only, and "write" is
 * the safe direction to err in).
 * @param {string} text
 * @returns {boolean}
 */
function hasSecondStatement(text) {
  let single = false;
  let double = false;
  let backtick = false;
  let escaped = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (single || double || backtick) {
      if (escaped) {
        escaped = false;
        continue;
      }
      if (ch === "\\" && !backtick) {
        escaped = true;
        continue;
      }
      if (single && ch === "'") single = false;
      else if (double && ch === '"') double = false;
      else if (backtick && ch === "`") backtick = false;
      continue;
    }
    if (ch === "'") single = true;
    else if (ch === '"') double = true;
    else if (ch === "`") backtick = true;
    else if (ch === ";") return true;
  }
  return false;
}

/**
 * The only DB info fields the island may render. Every value is coerced to a
 * string (hasPassword to a boolean); any other key the payload carries,
 * including a password under any name, is dropped before it can reach the
 * DOM.
 */
const DB_INFO_FIELDS = [
  "project",
  "engine",
  "host",
  "port",
  "database",
  "user",
  "hasPassword",
  "displayUrl",
];

/**
 * @param {unknown} info
 * @returns {{ project: string, engine: string, host: string, port: string, database: string, user: string, hasPassword: boolean, displayUrl: string }}
 */
export function pickDisplayDBInfo(info) {
  const src =
    info && typeof info === "object"
      ? /** @type {Record<string, unknown>} */ (info)
      : {};
  /** @type {Record<string, unknown>} */
  const out = {};
  for (const key of DB_INFO_FIELDS) {
    out[key] =
      key === "hasPassword" ? Boolean(src[key]) : String(src[key] ?? "");
  }
  return /** @type {any} */ (out);
}

/**
 * The restore confirm control stays disabled until the preview renders: only
 * a preview object carrying a token enables it (binding decision 3).
 * @param {unknown} preview
 * @returns {boolean}
 */
export function isSnapshotRestoreConfirmEnabled(preview) {
  if (!preview || typeof preview !== "object") {
    return false;
  }
  const token = /** @type {Record<string, unknown>} */ (preview).previewToken;
  return typeof token === "string" && token.trim() !== "";
}

/**
 * Maps the island's restore scope radio to the explicit db/media flags the
 * preview and apply calls take. Unknown scopes fall back to full.
 * @param {unknown} scope "full" | "db" | "media"
 * @returns {{ dbOnly: boolean, mediaOnly: boolean }}
 */
export function snapshotScopeFlags(scope) {
  if (scope === "db") {
    return { dbOnly: true, mediaOnly: false };
  }
  if (scope === "media") {
    return { dbOnly: false, mediaOnly: true };
  }
  return { dbOnly: false, mediaOnly: false };
}

/**
 * @param {unknown} opId
 * @param {unknown} [generation=0]
 * @returns {{ opId: string, gen: number, finished: boolean, outcome: string }}
 */
export function createTransferRun(opId, generation = 0) {
  const gen = Number(generation);
  return {
    opId: String(opId ?? ""),
    gen: Number.isFinite(gen) ? gen : 0,
    finished: false,
    outcome: "",
  };
}

/**
 * True while a transfer is still running. The island gates every start
 * button on this: no second start while one runs, so tracking can never be
 * silently superseded and progress/cancel always belong to the tracked run.
 * @param {unknown} run
 * @returns {boolean}
 */
export function isStartBlocked(run) {
  if (!run || typeof run !== "object") {
    return false;
  }
  return /** @type {Record<string, unknown>} */ (run).finished === false;
}

/**
 * Names the running op for the active-transfer notice and the start
 * refusal: what runs, its detail (destination, source or snapshot name) and
 * when it started. Empty when nothing runs.
 * @param {unknown} run
 * @returns {string}
 */
export function describeRunningOp(run) {
  if (!isStartBlocked(run)) {
    return "";
  }
  const record = /** @type {Record<string, unknown>} */ (run);
  const label = String(record.label ?? "Operation").trim() || "Operation";
  const detail = String(record.detail ?? "").trim();
  let when = "";
  const at = Number(record.startedAt ?? 0);
  if (Number.isFinite(at) && at > 0) {
    when = ` at ${new Date(at).toLocaleTimeString()}`;
  }
  const what = detail !== "" ? ` ${detail}` : "";
  return `${label}${what} started${when}. Wait for it to finish before starting another operation.`;
}

/**
 * @param {string} event
 * @returns {string} "success" for completed events, "failed" otherwise.
 */
export function transferTerminalOutcome(event) {
  const name = String(event ?? "");
  return name === "db:completed" || name === "snapshot:completed"
    ? "success"
    : "failed";
}

/**
 * Applies one terminal event to a tracked transfer, idempotently keyed on
 * (op, generation): a terminal for another op or a stale generation is
 * ignored, and a finished transfer never flips back to running. A transfer
 * whose generation is still unknown (gen 0, the start call only returned an
 * op id) adopts the first terminal's generation. Payloads without a
 * generation (snapshot completed/failed) match an unknown generation and
 * never disturb a known one.
 * @param {{ opId: string, gen: number, finished: boolean, outcome: string }} run
 * @param {{ opId?: unknown, generation?: unknown, gen?: unknown, event?: unknown }} payload
 */
export function applyTransferTerminalEvent(run, payload) {
  if (!run || run.finished) {
    return run;
  }
  if (String(payload?.opId ?? "") !== run.opId) {
    return run;
  }
  const raw = payload?.generation ?? payload?.gen ?? 0;
  const seen = Number(raw);
  const gen = Number.isFinite(seen) ? seen : 0;
  if (run.gen !== 0 && gen !== 0 && gen !== run.gen) {
    return run;
  }
  return {
    ...run,
    gen: run.gen === 0 ? gen : run.gen,
    finished: true,
    outcome: transferTerminalOutcome(String(payload?.event ?? "")),
  };
}
