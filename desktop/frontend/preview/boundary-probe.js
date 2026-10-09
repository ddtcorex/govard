// Dev-only probe for the error-boundary behaviour test. It mounts a throwing
// island through the production mountIsland helper; nothing in index.html or
// main.js knows it exists, and nothing under preview/ reaches prod builds.
import { createElement } from "react";
import { mountIsland } from "../islands/mount.js";

export function mountBoundaryProbe(containerId) {
  const Thrower = () => {
    if (window.__boundaryThrows) {
      throw new Error("boundary probe");
    }
    return createElement(
      "p",
      { "data-testid": "boundary-recovered" },
      "recovered",
    );
  };
  return mountIsland(containerId, createElement(Thrower));
}
