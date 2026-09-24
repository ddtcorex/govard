// @ts-check

/**
 * The only module that subscribes to backend events. Plan B (Wails 3)
 * replaces this body with @wailsio/runtime; callers do not change.
 */

/** @returns {boolean} */
export function hasEventRuntime() {
  return typeof window !== "undefined" && typeof window.runtime?.EventsOn === "function";
}

/**
 * @param {string} name
 * @param {(data: any) => void} handler
 * @returns {() => void} unsubscribe
 */
export function onEvent(name, handler) {
  const runtime = typeof window === "undefined" ? undefined : window.runtime;
  if (!runtime || typeof runtime.EventsOn !== "function") {
    return () => {};
  }
  const off = runtime.EventsOn(name, handler);
  return typeof off === "function" ? off : () => {};
}
