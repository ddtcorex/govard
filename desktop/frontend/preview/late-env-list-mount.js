// Dev-only, imported by a behaviour scenario after the app has booted. It mounts
// a second sidebar list through the production helper once main.js has already
// published the dashboard, which is the order a production boot can take: the
// fetch returns before React has run the island's effects.
import { createElement } from "react";
import { EnvironmentList } from "../islands/EnvironmentList.tsx";
import { mountIsland } from "../islands/mount.js";

export function mountLateEnvList(containerId) {
  const container = document.createElement("div");
  container.id = containerId;
  document.body.appendChild(container);
  return mountIsland(
    containerId,
    createElement(EnvironmentList, {
      onSelect: () => {},
      onToggle: () => {},
      onSwitchSidebarMode: () => {},
    }),
  );
}
