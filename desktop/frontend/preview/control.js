// @ts-check
import { setFixtures, getFixtures, resetFixtures, getCalls, resetCalls } from "./fixture-store.js";

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
    // Adds scenario-specific entries after the installed ones. Lookup prefers an
    // exact-args match and otherwise falls back to the first entry of the route,
    // so an added entry only answers the calls whose args it names, and a
    // `delayMs` field holds that one response back (or `error` makes it fail).
    appendFixtures(list) {
      setFixtures([...getFixtures(), ...list]);
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
