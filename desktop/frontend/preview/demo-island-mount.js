// Dev-only, imported by preview/bootstrap.js after the app has booted. It mounts
// the demo island into preview.html's own container through the production
// helper; nothing in index.html or main.js knows the island exists.
import { createElement } from "react";
import { DemoIsland } from "../islands/DemoIsland.tsx";
import { mountIsland } from "../islands/mount.js";

export function mountDemoIsland() {
  const handle = mountIsland("react-demo-island-root", createElement(DemoIsland));
  if (handle) window.__govardDemoIsland = handle;
}
