// Dev-only entry point for desktop/frontend/preview.html. It installs the
// preview seam before importing the app, so the real main.js runs against
// fixtures instead of a Go backend. Never referenced from index.html, and never
// part of the production build (a Go test pins that).
import { installPreviewSeam } from "./seam.js";
import { installPreviewControl } from "./control.js";

// The app gates every event subscription on hasEventRuntime(), which probes the
// three transports the Wails runtime package itself selects on. In plain Chrome none of
// them exists, so the app would subscribe to nothing and
// window.__govardPreview.pushEvent would have no listener to reach. The WebKitGTK
// probe is the one the Linux desktop build really uses, so installing it makes
// the preview take the same branch production takes on this platform - and it is
// what lets the CDP harness drive typed events at all.
// It stays inert: the runtime resolves its own invoke probe to this no-op (only
// System.invoke uses it, once, for "wails:runtime:ready"), and the seam below
// answers every binding call before the default HTTP transport is consulted.
window.webkit = window.webkit || {};
window.webkit.messageHandlers = window.webkit.messageHandlers || {};
window.webkit.messageHandlers.external = window.webkit.messageHandlers.external || {};
window.webkit.messageHandlers.external.postMessage = () => {};

await installPreviewSeam();
installPreviewControl();
// main.js mounts the metrics island with this polling interval instead of the
// production 15 s, and exposes the island handle as window.__govardMetricsIsland,
// so a behaviour test can watch the polling run and then stop on unmount. One
// second is far longer than a click plus its render, which keeps a per-click
// call count exact.
window.__govardPreviewMetricsIntervalMs = 1000;
// main.js mounts the logs island with this live-poll interval instead of the
// production 2 s, and exposes the island handle as window.__govardLogsIsland, so
// a behaviour test can watch the poll run and then prove unmounting stops it.
window.__govardPreviewLogsPollMs = 1000;
// main.js then exposes the update prompt model as window.__govardUpdatePromptModel,
// so a behaviour test can stop the 12 s startup schedule and run each background
// check itself instead of waiting on (and racing) the production timer.
window.__govardPreviewExposeUpdatePrompt = true;
await import("../main.js");

// After the app has booted, so the island's store reads see the same state
// main.js has already set. Preview-only: nothing imports this from index.html.
await import("./demo-island-mount.js").then((m) => m.mountDemoIsland());
