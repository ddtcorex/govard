/**
 * One in-flight registry for long desktop operations (`env-start:bebe9`,
 * `sync:bebe9:staging:db`, ...). A second begin for the same key is refused
 * so double-clicks and Start-then-Stop races run exactly one operation; the
 * refused caller reports through the status line instead.
 */

const inFlight = new Map();
let nextToken = 1;

/**
 * @param {string} key
 * @returns {string | null} a token to pass to endOperation, or null when the
 * key already has an operation in flight.
 */
export function beginOperation(key) {
  if (inFlight.has(key)) {
    return null;
  }
  const token = `op-${nextToken++}`;
  inFlight.set(key, token);
  return token;
}

/**
 * @param {string} token
 */
export function endOperation(token) {
  for (const [key, held] of inFlight) {
    if (held === token) {
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
