// @ts-check
import { __setBindingsLoaderForTest } from "../services/bridge.js";
import { getFixtures, recordCall } from "./fixture-store.js";
import { resolveFixtureResponse } from "./fixture-lookup.js";
import { materializeResponse } from "./response-materializer.js";

/**
 * A Proxy-backed fake bindings module: every Service.Method call resolves
 * through the shared fixture lookup + materializer, so every route answers from
 * day one with no fixture needed (spec Architecture: "every bridge route answers
 * from day one").
 */
function makeService(serviceName) {
  return new Proxy(
    {},
    {
      get(_target, method) {
        if (typeof method !== "string") return undefined;
        return async (...args) => {
          recordCall({ service: serviceName, method, args });
          const resolved = resolveFixtureResponse(getFixtures(), serviceName, method, args);
          if (resolved.found && "error" in resolved) {
            throw new Error(String(resolved.error));
          }
          const raw = resolved.found ? resolved.result : undefined;
          return materializeResponse(serviceName, method, raw);
        };
      },
    },
  );
}

const SERVICE_NAMES = [
  "EnvironmentService", "GlobalServiceService", "LogService", "OnboardingService",
  "RemoteService", "SettingsService", "SystemService", "UpdateService",
];

export function installFakeBindingsLoader() {
  const fakeModule = Object.fromEntries(SERVICE_NAMES.map((name) => [name, makeService(name)]));
  return __setBindingsLoaderForTest(async () => fakeModule);
}
