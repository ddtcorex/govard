// Dev-only, imported by preview/bootstrap.js after the app has booted. It mounts
// the demo island into preview.html's own container; nothing in index.html or
// main.js knows the island exists.
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { DemoIsland } from "../islands/DemoIsland.tsx";

export function mountDemoIsland() {
  const container = document.getElementById("react-demo-island-root");
  if (!container) return;
  const root = createRoot(container);
  root.render(createElement(DemoIsland));
  window.__govardDemoIsland = { unmount: () => root.unmount() };
}
