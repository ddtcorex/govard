// @ts-check
import { setFixtures, resetFixtures, getCalls, resetCalls } from "./fixture-store.js";

/**
 * window.__govardPreview: the only control surface a CDP scenario needs.
 * The only module that touches window in the preview build.
 */
export function installPreviewControl() {
  window.__govardPreview = {
    async installFixtures(name) {
      const res = await fetch(`/preview/fixtures/${name}.json`);
      if (!res.ok) {
        throw new Error(`installFixtures: no fixture file for "${name}" (HTTP ${res.status})`);
      }
      setFixtures(await res.json());
    },
    pushEvent(name, data) {
      window._wails = window._wails || {};
      window._wails.dispatchWailsEvent?.({ name, data });
    },
    reset() {
      resetFixtures();
      resetCalls();
    },
    getCalls,
  };
}
