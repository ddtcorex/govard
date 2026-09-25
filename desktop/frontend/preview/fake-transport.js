// @ts-check
import {
  __setTransportForTest,
  CALL_OBJECT_ID,
  CANCEL_CALL_OBJECT_ID,
} from "../services/bridge.js";
import { ID_MAP } from "./id-map.generated.js";
import { getFixtures, recordCall } from "./fixture-store.js";
import { resolveFixtureResponse } from "./fixture-lookup.js";
import { materializeResponse } from "./response-materializer.js";

/**
 * Candidate A (the Phase 0 gate's choice): keep the real generated bindings and
 * the real `$Call.ByID` path, and answer at the Wails runtime transport instead.
 * The Wails runtime package is reached through bridge.js's lazy seam, so this file - and
 * every other preview file - stays clear of the runtime and of the bindings path,
 * which is what tests/desktop_frontend_bridge_guard_test.go enforces.
 */
export function installFakeTransport() {
  return __setTransportForTest({
    async call(objectID, _method, _windowName, args) {
      if (objectID === CANCEL_CALL_OBJECT_ID) {
        return true; // no-op success; CancellablePromise's oncancelled path
      }
      // The runtime makes non-binding calls of its own (System, Window, Events)
      // and a future runtime version may add more. Nothing in this app routes
      // through them - the frontend guard allows exactly two modules at the
      // runtime boundary, and this app uses only $Call.ByID plus the local event
      // listener registry - so answer them with a no-op instead of throwing. A
      // throw here would surface as an unhandled rejection inside the runtime.
      if (objectID !== CALL_OBJECT_ID) {
        return undefined;
      }
      const route = ID_MAP[String(args.methodID)];
      if (!route) {
        throw new Error(
          `preview fake transport: unknown methodID ${args.methodID}; regenerate id-map.generated.js`,
        );
      }
      const callArgs = args.args ?? [];
      recordCall({ service: route.service, method: route.method, args: callArgs });
      const resolved = resolveFixtureResponse(getFixtures(), route.service, route.method, callArgs);
      if (resolved.found && "error" in resolved) {
        throw new Error(String(resolved.error));
      }
      const raw = resolved.found ? resolved.result : undefined;
      return materializeResponse(route.service, route.method, raw);
    },
  });
}
