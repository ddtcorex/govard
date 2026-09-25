// Dev-only, injected into index.html by preview/vite-plugin-preview.js when
// GOVARD_PREVIEW_RECORD=1 reaches the Vite dev server. It installs the recording
// loader BEFORE main.js runs, so the running app keeps talking to the real Go
// backend while every call is copied to preview/fixtures/<module>.json.
//
// Record mode always uses the BINDINGS LOADER seam, whichever candidate
// PREVIEW_SEAM selects for playback. In the real desktop app there is no inner
// transport to chain: the Wails runtime's customTransport is null there, so its
// call path is the built-in HTTP transport, which is not reachable as an object
// and cannot be wrapped without re-implementing its chunking and error-kind
// switch. The loader seam has no such problem: it wraps the real generated
// module, which is exactly spec D2's "same seam, wrapping the real one".
//
// The real module comes from bridge.js's loadGeneratedModules, not from a direct
// import: preview files must stay clear of the runtime and the bindings path,
// which tests/desktop_frontend_bridge_guard_test.go enforces.
import { __setBindingsLoaderForTest, loadGeneratedModules } from "../services/bridge.js";
import { getState } from "../state/store.js";
import { createRecordingLoader } from "./record.js";

// Vite's HMR channel, not an HTTP POST: the app window's origin is the Wails
// asset server, whose dev-server proxy drops POST bodies (measured). Vite only
// defines import.meta.hot while it is serving, so a missing channel is reported
// rather than swallowed: a recorder that fails silently looks exactly like an app
// that made no calls.
const post = (entry) => {
  if (!import.meta.hot) {
    console.warn("[preview] no Vite HMR channel; record mode cannot capture calls");
    return Promise.resolve();
  }
  import.meta.hot.send("govard:record", entry);
  return Promise.resolve();
};

__setBindingsLoaderForTest(
  createRecordingLoader({
    loadReal: loadGeneratedModules,
    post,
    getModule: () => getState().sidebarMode,
  }),
);
