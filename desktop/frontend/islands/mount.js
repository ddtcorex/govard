// @ts-check
import { createRoot } from "react-dom/client";

/**
 * Mounts one React island into an existing container (spec: island contract).
 * The island owns the container's whole subtree from here on; nothing else may
 * write into it, and its markup carries no data-action attributes (D5).
 *
 * @param {string} containerId
 * @param {import("react").ReactNode} element
 * @returns {{ unmount(): void } | null} null when the container is missing
 */
export function mountIsland(containerId, element) {
  if (typeof document === "undefined") return null;
  const container = document.getElementById(containerId);
  if (!container) return null;
  const root = createRoot(container);
  root.render(element);
  return { unmount: () => root.unmount() };
}
