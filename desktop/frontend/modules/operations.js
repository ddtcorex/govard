/**
 * One in-flight registry for long desktop operations. Keys are scoped so
 * mutually exclusive work cannot overlap: mutating environment actions share
 * one key per project (`env:bebe9`), so a Start-then-Stop race is refused
 * just like a double-clicked Start. The refused caller reports through the
 * status line instead.
 */

const inFlight = new Map();
let nextToken = 1;

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
  return token;
}

/**
 * @param {string} token
 */
export function endOperation(token) {
  for (const [key, held] of inFlight) {
    if (held.token === token) {
      inFlight.delete(key);
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
