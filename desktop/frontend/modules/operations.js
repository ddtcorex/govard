/**
 * One in-flight registry for long desktop operations. Keys are scoped so
 * mutually exclusive work cannot overlap: mutating environment actions share
 * one key per project (`env:sample-project`), so a Start-then-Stop race is refused
 * just like a double-clicked Start. The refused caller reports through the
 * status line instead.
 *
 * Env locks are also published into the store's `busyEnvs` map (project to
 * label) on every begin/end, so the hero and the sidebar can disable visibly
 * with a spinner instead of surprising through the status line. Non-env keys
 * never touch the store.
 */

import { getState, setState } from "../state/store.js";

const inFlight = new Map();
let nextToken = 1;

/** The prefix mutating environment actions share, one key per project. */
export const ENV_BUSY_PREFIX = "env:";

/**
 * @param {string} project
 * @returns {string} the in-flight key mutating actions hold for the project.
 */
export function envBusyKey(project) {
  return `${ENV_BUSY_PREFIX}${String(project || "").trim()}`;
}

function publishBusyEnvs() {
  const busy = {};
  for (const [key, held] of inFlight) {
    if (typeof key === "string" && key.startsWith(ENV_BUSY_PREFIX)) {
      const project = key.slice(ENV_BUSY_PREFIX.length);
      if (project) {
        busy[project] = held.label;
      }
    }
  }
  setState({ busyEnvs: busy });
}

/**
 * @param {string} key
 * @param {string} [label] human-readable name reported while the key is held.
 * @returns {string | null} a token to pass to endOperation, or null when the
 * key already has an operation in flight.
 */
export function beginOperation(key, label = "") {
  if (inFlight.has(key)) {
    return null;
  }
  const token = `op-${nextToken++}`;
  inFlight.set(key, { token, label: String(label || "") });
  if (typeof key === "string" && key.startsWith(ENV_BUSY_PREFIX)) {
    publishBusyEnvs();
  }
  return token;
}

/**
 * @param {string} token
 */
export function endOperation(token) {
  for (const [key, held] of inFlight) {
    if (held.token === token) {
      inFlight.delete(key);
      if (typeof key === "string" && key.startsWith(ENV_BUSY_PREFIX)) {
        publishBusyEnvs();
      }
      return;
    }
  }
}

/**
 * @param {string} key
 * @returns {boolean}
 */
export function isOperationInFlight(key) {
  return inFlight.has(key);
}

/**
 * @param {string} key
 * @returns {string} the label holding the key, or "" when it is free.
 */
export function operationLabel(key) {
  return inFlight.get(key)?.label || "";
}

/**
 * @param {string} project
 * @returns {boolean} whether the project currently holds its env lock.
 */
export function isEnvBusy(project) {
  const key = envBusyKey(project);
  if (!String(project || "").trim()) {
    return false;
  }
  return inFlight.has(key);
}

/**
 * @param {string} project
 * @returns {string} the label holding the project's env lock, or "".
 */
export function envBusyLabel(project) {
  if (!String(project || "").trim()) {
    return "";
  }
  return operationLabel(envBusyKey(project));
}

/**
 * Releases the token when the promise settles (either way). Used when a
 * timeout backstop wins the race but the backend call is still running: the
 * key stays held until the real operation finishes instead of admitting a
 * duplicate.
 *
 * @param {string} token
 * @param {Promise<unknown>} promise
 */
export function releaseWhenSettled(token, promise) {
  void Promise.resolve(promise).then(
    () => endOperation(token),
    () => endOperation(token),
  );
}

/**
 * Test seam: the store copy of the busy map. The registry stays the source of
 * truth; this only reads what the hero and sidebar render.
 */
export function busyEnvsSnapshot() {
  return { ...(getState().busyEnvs || {}) };
}
