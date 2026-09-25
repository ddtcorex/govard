import { useSyncExternalStore } from "react";
import { getState, getVersion, subscribe } from "./store.js";

/**
 * Reads the shared store from a React island (spec D6).
 *
 * The snapshot React watches is the store's revision number, not the state
 * object: the store mutates in place, so `getState()` returns the same identity
 * forever and useSyncExternalStore would bail out of every update - an island
 * that renders once and then ignores every later setState. Watching the number
 * and reading the object is the pair that works with a mutable store.
 *
 * Measured 2026-09-25: with `getState` as the snapshot the demo island mounted,
 * incremented its own state, and never re-rendered when a setState from outside
 * React changed sidebarMode; with the version it re-renders.
 *
 * The returned object is the store itself, and its identity never changes, so do
 * not memoize on it: `useMemo(() => derive(state), [state])` and `React.memo` on
 * a `state` prop both keep their first value forever. Read the fields you need in
 * the component body, or memoize on those fields (`[state.selectedProject]`).
 */
export const useStore = () => {
  useSyncExternalStore(subscribe, getVersion);
  return getState();
};
