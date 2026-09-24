// @ts-check

/**
 * The only module that subscribes to backend events. @wailsio/runtime is
 * loaded lazily so node tests never import the browser runtime.
 */

/** @type {() => Promise<{ Events: { On(name: string, cb: (event: { data: any }) => void): () => void } }>} */
let loadRuntime = () => import("@wailsio/runtime");

/**
 * Test seam: replace the runtime loader, returns a restore function.
 * @param {typeof loadRuntime} fn
 */
export function __setRuntimeLoaderForTest(fn) {
  const previous = loadRuntime;
  loadRuntime = fn;
  return () => {
    loadRuntime = previous;
  };
}

/**
 * True when the page runs inside the Wails webview, which is the only place the
 * backend can be reached. The transports probed here are the ones
 * @wailsio/runtime itself selects on (WebKitGTK and macOS, Windows WebView2,
 * Android): the runtime module loads in a plain browser too, so a DOM by itself
 * says nothing about a backend being there.
 * @returns {boolean}
 */
export function hasEventRuntime() {
  if (typeof window === "undefined") {
    return false;
  }
  const host = /** @type {any} */ (window);
  return Boolean(
    host.webkit?.messageHandlers?.external?.postMessage ||
      host.chrome?.webview?.postMessage ||
      host.wails?.invoke,
  );
}

/**
 * @param {string} name
 * @param {(data: any) => void} handler
 * @returns {() => void} unsubscribe
 */
export function onEvent(name, handler) {
  /** @type {null | (() => void)} */
  let off = null;
  let cancelled = false;
  loadRuntime()
    .then(({ Events }) => {
      if (!cancelled) {
        off = Events.On(name, (event) => handler(event.data));
      }
    })
    .catch(() => {});
  return () => {
    cancelled = true;
    if (off) {
      off();
    }
  };
}
